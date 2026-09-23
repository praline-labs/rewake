package workflow

// What this column's session says about itself, through the two channels the
// adapter configures: the telemetry hook on each event, and the status line.
//
// The fixture plays a short session — a start, a turn with a compaction in
// it, the end of the turn — and runs each command exactly as the harness
// runs it: through /bin/sh -c, the payload on stdin. The payloads carry the
// conversation fields a real one carries, so a product that kept them would
// have them to keep. Then it measures what those commands cost, since the
// hooks sit in front of every prompt, and writes both down for the case.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"
)

const (
	// shimTelemetryFile switches the telemetry script on and names where the
	// session writes what it saw: the status line's output and the timings.
	shimTelemetryFile = "RW_SHIM_TELEMETRY_FILE"
	// shimOwnerStatus is the person's own status-line command, run directly
	// to time what the tap adds to it.
	shimOwnerStatus = "RW_SHIM_OWNER_STATUS"
	// shimTelemetryNotify names the session told, by a notify, that the
	// script has finished: its header then shows this session's final state.
	shimTelemetryNotify = "RW_SHIM_TELEMETRY_NOTIFY"
	// shimReportListing makes a session write what its own `rewake list` says,
	// until it is asked to stop; listings that time out are recorded beside it.
	shimReportListing = "RW_SHIM_REPORT_LISTING"
	// telemetryConversation is the conversation the script's events name.
	telemetryConversation = "conv-telemetry-1"
	// conversationText stands for the words of a conversation. It rides in
	// every field a real payload carries text in.
	conversationText = "CONVERSATION-TEXT-NOT-FOR-REWAKE"
	// latencyRuns is how many times each command is timed.
	latencyRuns = 40
)

// telemetryResult is what the script writes for the case.
type telemetryResult struct {
	// StatusOutputs are what each run of the status line printed.
	StatusOutputs []string `json:"statusOutputs"`
	// Hook, Tap and Owner are timings in milliseconds: one telemetry hook, the
	// tap running the person's command, and that command run directly.
	Hook  []float64 `json:"hookMs"`
	Tap   []float64 `json:"tapMs"`
	Owner []float64 `json:"ownerMs"`
	Error string    `json:"error,omitempty"`
}

func hookPayload(event string, extra map[string]any) []byte {
	payload := map[string]any{
		"session_id":      telemetryConversation,
		"transcript_path": "/home/u/.claude/projects/p/" + telemetryConversation + ".jsonl",
		"cwd":             workingDirectory(),
		"hook_event_name": event,
	}
	for key, value := range extra {
		payload[key] = value
	}
	encoded, _ := json.Marshal(payload)
	return encoded
}

// statusPayload is what the harness hands its status line, in the shape seen
// on 2.1.280 (docs/research.md). used < 0 is a context not measured yet.
func statusPayload(used int64) []byte {
	window := map[string]any{
		"total_input_tokens": 0, "total_output_tokens": 0, "context_window_size": 200000,
		"current_usage": nil, "used_percentage": nil, "remaining_percentage": nil,
	}
	if used >= 0 {
		percent := used * 100 / 200000
		window = map[string]any{
			"total_input_tokens": used, "total_output_tokens": 72, "context_window_size": 200000,
			"current_usage":   map[string]any{"input_tokens": 10, "output_tokens": 72, "cache_creation_input_tokens": used - 10, "cache_read_input_tokens": 0},
			"used_percentage": percent, "remaining_percentage": 100 - percent,
		}
	}
	encoded, _ := json.Marshal(map[string]any{
		"session_id": telemetryConversation, "transcript_path": "/home/u/t.jsonl", "cwd": workingDirectory(),
		"model":  map[string]any{"id": "model-telemetry", "display_name": conversationText},
		"effort": map[string]any{"level": "high"}, "context_window": window,
		"workspace": map[string]any{"current_dir": workingDirectory()}, "version": "2.1.280",
	})
	return append(encoded, '\n')
}

// runShellTimed runs one command line the way the harness does and times it.
func runShellTimed(command string, stdin []byte) (string, float64, error) {
	started := time.Now()
	shell := exec.Command("/bin/sh", "-c", command)
	shell.Env = os.Environ()
	shell.Stdin = strings.NewReader(string(stdin))
	out, err := shell.Output()
	return string(out), float64(time.Since(started).Microseconds()) / 1000, err
}

// playTelemetry is the session's life as its hooks and status line tell it.
func (s *claudeSession) playTelemetry() {
	target := os.Getenv(shimTelemetryFile)
	var result telemetryResult
	settings := s.launch.settings
	fire := func(event string, extra map[string]any) {
		if _, _, err := runShellTimed(settings.observe, hookPayload(event, extra)); err != nil && result.Error == "" {
			result.Error = fmt.Sprintf("the %s hook failed: %v", event, err)
		}
	}
	status := func(used int64) {
		out, _, err := runShellTimed(settings.statusLine, statusPayload(used))
		if err != nil && result.Error == "" {
			result.Error = fmt.Sprintf("the status line failed: %v", err)
		}
		result.StatusOutputs = append(result.StatusOutputs, out)
	}

	fire("SessionStart", map[string]any{"source": "startup", "model": "model-telemetry"})
	status(-1)
	fire("UserPromptSubmit", map[string]any{"prompt": conversationText, "permission_mode": "default"})
	status(84000)
	fire("PreCompact", map[string]any{"trigger": "auto", "custom_instructions": conversationText})
	fire("SessionStart", map[string]any{"source": "compact", "model": "model-telemetry"})
	fire("PostCompact", map[string]any{"trigger": "auto", "compact_summary": conversationText})
	status(-1)
	status(50000)
	fire("Stop", map[string]any{"last_assistant_message": conversationText, "stop_hook_active": false})

	// A kind the collector does not know: the whole path runs, and the
	// session's state does not move.
	probe := hookPayload("RewakeLatencyProbe", map[string]any{"prompt": conversationText})
	owner := os.Getenv(shimOwnerStatus)
	for range latencyRuns {
		_, hook, _ := runShellTimed(settings.observe, probe)
		_, tap, _ := runShellTimed(settings.statusLine, statusPayload(50000))
		_, direct, _ := runShellTimed(owner, statusPayload(50000))
		result.Hook = append(result.Hook, hook)
		result.Tap = append(result.Tap, tap)
		result.Owner = append(result.Owner, direct)
	}
	// One more status after the timing, so the last thing the collector saw
	// is the state the case checks.
	status(50000)

	if lead := os.Getenv(shimTelemetryNotify); lead != "" {
		send := exec.Command("rewake", "send", lead, "--notify", "telemetry played")
		send.Env = os.Environ()
		if out, err := send.CombinedOutput(); err != nil && result.Error == "" {
			result.Error = fmt.Sprintf("telling %s: %v: %s", lead, err, out)
		}
	}
	encoded, _ := json.Marshal(result)
	_ = os.WriteFile(target+".tmp", encoded, 0o600)
	_ = os.Rename(target+".tmp", target)
}

// reportState writes what this session's own `rewake list` says, until it is
// asked to stop: the session is main, so it sees the telemetry of the room.
//
// Each listing is bounded and dies with the session. One of them once
// outlived the session in a full run and held its process group for the rest
// of the case, which the case then reported as a session that would not end.
func (s *claudeSession) reportState() {
	target := os.Getenv(shimStateFile)
	for {
		if _, err := os.Stat(os.Getenv(shimExitFile)); err == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		list := exec.CommandContext(ctx, "rewake", "list", "--json")
		list.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
		out, err := list.Output()
		timedOut := ctx.Err() != nil
		cancel()
		if timedOut {
			// Recorded, not skipped: the case reads this and fails what the
			// listing was meant to show.
			if file, err := os.OpenFile(target+".timeouts", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
				_, _ = fmt.Fprintf(file, "%s rewake list --json timed out after 5s\n", time.Now().Format(time.RFC3339Nano))
				_ = file.Close()
			}
			continue
		}
		if err == nil {
			_ = os.WriteFile(target+".tmp", out, 0o600)
			_ = os.Rename(target+".tmp", target)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// quantile is the value at q of the sorted timings.
func quantile(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)
	index := int(q * float64(len(sorted)-1))
	return sorted[index]
}
