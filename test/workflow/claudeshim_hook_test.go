package workflow

// What this column's session does with a delivery, and how the end of its turn
// reaches the wrapper.
//
// There is no protocol event here. The wrapper configures a Stop hook in the
// settings it layers over the session's, and the harness runs that command
// with a JSON object on stdin when a turn ends. So the fixture reads the hook
// out of the settings it was launched with and runs it the same way: the
// report then travels the product's own path — `rewake turn-ended` reading
// stdin — rather than a path invented for the test.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// claudeSettings is the part of the settings layer this fixture uses: the
// command each hook runs, and the status line. What the wrapper puts there is
// checked, because a session that never ran the hook would leave every sender
// waiting and the scenario would report it as a delivery that produced no
// answer.
type claudeSettings struct {
	stop        string
	stopFailure string
	// observe is the telemetry hook: one command, registered for every event
	// in telemetryEvents and run in the background.
	observe string
	// statusLine is the status line's command: rewake's tap.
	statusLine string
}

// telemetryEvents are the hooks the adapter registers for telemetry. Spelled
// out rather than imported, like everything else this fixture checks.
var telemetryEvents = []string{"SessionStart", "UserPromptSubmit", "PreCompact", "PostCompact", "Notification", "Stop", "StopFailure", "SessionEnd"}

func parseClaudeSettings(raw string) (claudeSettings, error) {
	var settings claudeSettings
	if strings.TrimSpace(raw) == "" {
		return settings, errors.New("no --settings; without it the end of a turn reaches nobody")
	}
	var layer struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Kind    string `json:"type"`
				Command string `json:"command"`
				Timeout int    `json:"timeout"`
				Async   bool   `json:"async"`
			} `json:"hooks"`
		} `json:"hooks"`
		StatusLine *struct {
			Kind    string `json:"type"`
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal([]byte(raw), &layer); err != nil {
		return settings, fmt.Errorf("unreadable --settings: %w", err)
	}
	observed := map[string]bool{}
	for name, matchers := range layer.Hooks {
		for _, matcher := range matchers {
			for _, hook := range matcher.Hooks {
				if hook.Kind != "command" || hook.Command == "" {
					return settings, fmt.Errorf("the %s hook is not a command", name)
				}
				switch {
				case strings.HasSuffix(hook.Command, "'turn-ended'"):
					// The end of a turn has to be recorded before the session
					// goes idle, so it never runs in the background.
					if hook.Async {
						return settings, fmt.Errorf("the %s turn-ended hook runs in the background", name)
					}
					switch name {
					case "Stop":
						settings.stop = hook.Command
					case "StopFailure":
						settings.stopFailure = hook.Command
					default:
						return settings, fmt.Errorf("turn-ended registered for %s", name)
					}
				case strings.Contains(hook.Command, "'observe' "):
					// In front of every prompt: it must not hold the session.
					if !hook.Async {
						return settings, fmt.Errorf("the %s telemetry hook runs in the foreground", name)
					}
					if settings.observe != "" && settings.observe != hook.Command {
						return settings, fmt.Errorf("the %s telemetry hook names a different command", name)
					}
					if !slices.Contains(telemetryEvents, name) || observed[name] {
						return settings, fmt.Errorf("a telemetry hook this fixture does not serve: %s", name)
					}
					settings.observe = hook.Command
					observed[name] = true
				default:
					return settings, fmt.Errorf("a hook this fixture does not serve: %s runs %s", name, hook.Command)
				}
			}
		}
	}
	if settings.stopFailure == "" {
		// Every role gets this one; only Stop depends on whether the role
		// reports at all.
		return settings, errors.New("no StopFailure hook in --settings")
	}
	if len(observed) != len(telemetryEvents) {
		return settings, fmt.Errorf("telemetry hooks for %d of %d events", len(observed), len(telemetryEvents))
	}
	if layer.StatusLine == nil || layer.StatusLine.Kind != "command" || !strings.Contains(layer.StatusLine.Command, "'status-tap' ") {
		return settings, errors.New("no status-line tap in --settings")
	}
	settings.statusLine = layer.StatusLine.Command
	return settings, nil
}

// workTurn is a turn of this column's session: read the mail the notice was
// about, then end the turn through the hook.
func (s *claudeSession) workTurn(turn string, notice claudeNotice) {
	s.recordDelivery(turn, notice)
	// A delivered message starts a turn through UserPromptSubmit, as a typed
	// one does (seen live on 2.1.280); its hook is how rewake learns the turn
	// began.
	if payload, err := json.Marshal(map[string]any{
		"hook_event_name": "UserPromptSubmit", "session_id": os.Getenv(sessionNameEnv),
		"cwd": workingDirectory(), "prompt": notice.Summary,
	}); err == nil {
		s.runHook("UserPromptSubmit", s.launch.settings.observe, payload)
	}

	var text string
	var err error
	if os.Getenv(shimReadEach) != "" {
		text, err = readEachAnnounced(notice.Count)
	} else {
		text, err = (&shimSession{}).readMailbox()
	}
	if err != nil {
		text = "could not read the mailbox: " + err.Error()
	}
	markPendingOnce()
	s.endTurn(text)
	// Recorded after the hook, not before: a session told to leave once it
	// has worked a turn leaves when this line appears, and one that left
	// before its hook ran would take its report with it — which is a
	// different control from leaving early, and not the one asked for.
	(&shimSession{}).recordTurn(turn + " " + firstLine(text))
}

// endTurn runs the hook the wrapper configured, with the payload the real
// harness sends on stdin.
//
// Which hook depends on how the turn went, the way the harness chooses: Stop
// for a turn that answered, StopFailure for one that broke. A silent role gets
// no Stop hook at all, and then a turn that answered reports nothing — which
// is the product's rule, not a gap in the fixture.
func (s *claudeSession) endTurn(text string) {
	failed := os.Getenv(shimLateFailure) != "" || os.Getenv(shimFailHeldTurn) != ""
	command, event := s.launch.settings.stop, "Stop"
	if failed {
		command, event = s.launch.settings.stopFailure, "StopFailure"
	}
	payload, err := json.Marshal(hookInput(event, text))
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-shim: the hook payload could not be written: %v\n", err)
		return
	}
	// The harness runs every hook of the event; the telemetry one runs in the
	// background and cannot hold the turn, so its result is not waited on here
	// either beyond its own exit.
	s.runHook(event, s.launch.settings.observe, payload)
	s.runHook(event, command, payload)
}

// runHook runs one hook command with its payload on stdin. The command is a
// quoted argv, as the settings layer spells it, so it runs through a shell the
// way the harness runs it.
func (s *claudeSession) runHook(event, command string, payload []byte) {
	if command == "" {
		return
	}
	hook := exec.Command("/bin/sh", "-c", command)
	hook.Env = os.Environ()
	hook.Stdin = strings.NewReader(string(payload))
	if out, err := hook.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "claude-shim: the %s hook failed: %v: %s\n", event, err, out)
	}
}

// hookInput is what the harness puts on the hook's stdin.
//
// What rewake reads from it is more than the message alone
// (internal/cli/turn_result.go): agent_id, which decides whether the end
// counts at all — an inherited hook in a child carries one and is ignored —
// then type, hook_event_name, turn-id or turn_id, thread-id, the last message
// under three spellings, and error with error_details for a failure. This
// fixture sends the ones a root session's Stop and StopFailure carry, and no
// agent_id: this is the session itself, never a child.
//
// Verified is less than sent. Live, on 2.1.270, the Stop hook was seen to
// receive last_assistant_message; the rest — the event name, session_id, cwd,
// transcript_path, and the failure fields — follow the hook reference and are
// assumed, which docs/research.md records.
//
// For a failure the reference puts the error text in last_assistant_message,
// with error_details and error beside it, and that is what this sends — not an
// ordinary answer next to an error string, which is a payload the harness does
// not produce.
func hookInput(event, text string) map[string]any {
	input := map[string]any{
		"hook_event_name":        event,
		"last_assistant_message": text,
		"session_id":             os.Getenv(sessionNameEnv),
		"cwd":                    workingDirectory(),
		"transcript_path":        "",
	}
	if event == "StopFailure" {
		failure := "the turn failed after its answer was sent"
		input["last_assistant_message"] = failure
		input["error_details"] = failure
		input["error"] = "api_error"
	}
	return input
}

func workingDirectory() string {
	directory, err := os.Getwd()
	if err != nil {
		return ""
	}
	return directory
}
