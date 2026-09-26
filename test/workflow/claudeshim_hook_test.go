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
	"sync"
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
	s.clearIfAsked()
	// A delivered message starts a turn through UserPromptSubmit, as a typed
	// one does (seen live on 2.1.280); its hook is how rewake learns the turn
	// began.
	if payload, err := json.Marshal(map[string]any{
		"hook_event_name": "UserPromptSubmit", "session_id": shimConversation(),
		"cwd": workingDirectory(), "prompt": notice.Summary,
	}); err == nil {
		s.runHook("UserPromptSubmit", s.launch.settings.observe, payload)
	}
	s.startTurn(turn)
	s.plugin.turnStarted(turn, notice.Summary)

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
	recordOwedOnce()
	markPendingOnce()
	if s.firstTurn(shimInterruptFirst) {
		// Esc: the harness ends the turn with no Stop hook at all, and only
		// the plugin hears it (docs/research-claude-control.md).
		s.completeTurn(turn, text, "aborted")
		(&shimSession{}).recordTurn(turn + " interrupted " + firstLine(text))
		return
	}
	if s.firstTurn(shimHoldFirstTurn) && s.holdTurn(turn, text) {
		return
	}
	late := ""
	if s.firstTurn(shimInterruptAtStop) {
		// Esc after the Stop hook ran: the turn is reported and still ends
		// as aborted.
		late = "aborted"
	}
	s.endTurn(turn, text, late)
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
//
// The plugin hears the end as well, in the order seen live on 2.1.280: the
// hook event through classic.Stop or classic.StopFailure first, then
// turn.complete — 15 to 30 ms after Stop, about 1 ms after StopFailure. Its
// report of an ordinary end must add nothing to the hooks'. late, when set,
// is the reason turn.complete gives instead: an Esc landing after the hook.
//
// A Stop hook that prints decision "block" holds the turn, as the harness
// lets it: the model is asked again, and the Stop hooks run once more on its
// new answer, up to maxStopHolds times in a row (seen live on 2.1.280).
func (s *claudeSession) endTurn(turn, text, late string) {
	failed := os.Getenv(shimLateFailure) != "" || os.Getenv(shimFailHeldTurn) != ""
	command, event, reason := s.launch.settings.stop, "Stop", "answer"
	if failed {
		command, event, reason = s.launch.settings.stopFailure, "StopFailure", "error"
	}
	if late != "" {
		reason = late
	}
	input := hookInput(event, text)
	payload, err := json.Marshal(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-shim: the hook payload could not be written: %v\n", err)
		return
	}
	s.plugin.event("classic."+event, input)
	// The harness runs every hook of the event; the telemetry one runs in the
	// background and cannot hold the turn, so its result is not waited on here
	// either beyond its own exit.
	s.runHook(event, s.launch.settings.observe, payload)
	out := s.runHook(event, command, payload)
	for holds := 1; event == "Stop"; holds++ {
		held, ok := blockDecision(out)
		if !ok {
			break
		}
		(&shimSession{}).recordTurn(turn + " held: " + held)
		if holds > maxStopHolds {
			// The harness gives up on the blocking hook and ends the turn,
			// calling no Stop hook again.
			break
		}
		// The model is asked again with the reason, and its new answer
		// ends the turn once more: the same hook, called with
		// stop_hook_active true and that answer alone.
		text = s.continueHeld()
		input = hookInput(event, text)
		input["stop_hook_active"] = true
		if payload, err = json.Marshal(input); err != nil {
			break
		}
		s.plugin.event("classic."+event, input)
		s.runHook(event, s.launch.settings.observe, payload)
		out = s.runHook(event, command, payload)
	}
	s.completeTurn(turn, text, reason)
}

// maxStopHolds is how many Stop blocks in a row the harness honors in one turn:
// after the ninth it ends the turn anyway (seen live on 2.1.280).
const maxStopHolds = 8

// holdContinuation is what the fixture's model says when a Stop hook held its
// turn and it has nothing to add.
const holdContinuation = "nothing to add after the hold"

// shimPendingOnHold makes the continuation of the session's first held turn
// run `rewake pending <text>` before it ends: a worker asked by the hold that
// is still waiting on something. Later continuations only end the turn.
const shimPendingOnHold = "RW_SHIM_PENDING_ON_HOLD"

var pendingOnHold sync.Once

// continueHeld is the model's turn after a hold: it may mark the turn, and
// answers its short last message.
func (s *claudeSession) continueHeld() string {
	if text := os.Getenv(shimPendingOnHold); text != "" {
		pendingOnHold.Do(func() {
			out, err := exec.Command("rewake", "pending", text).CombinedOutput()
			line := "pending on hold ok"
			if err != nil {
				line = "pending on hold refused: " + err.Error() + ": " + string(out)
			}
			(&shimSession{}).recordTurn(line)
		})
	}
	return holdContinuation
}

// blockDecision reads a Stop hook's stdout the way the harness does: a JSON
// object with decision "block" holds the turn, and its reason goes to the
// model.
func blockDecision(out []byte) (string, bool) {
	var decision struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if json.Unmarshal(out, &decision) != nil || decision.Decision != "block" {
		return "", false
	}
	return decision.Reason, true
}

// runHook runs one hook command with its payload on stdin. The command is a
// quoted argv, as the settings layer spells it, so it runs through a shell the
// way the harness runs it.
// It answers what the hook printed on stdout, where a Stop hook gives its
// decision.
func (s *claudeSession) runHook(event, command string, payload []byte) []byte {
	if command == "" {
		return nil
	}
	hook := exec.Command("/bin/sh", "-c", command)
	hook.Env = os.Environ()
	hook.Stdin = strings.NewReader(string(payload))
	var stderr strings.Builder
	hook.Stderr = &stderr
	out, err := hook.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-shim: the %s hook failed: %v: %s%s\n", event, err, out, stderr.String())
	}
	return out
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
// receive last_assistant_message, and on 2.1.280 every hook the event name,
// session_id, cwd and transcript_path, and Stop its stop_hook_active: false on
// a turn end's first call, true on the call after a block, whose
// last_assistant_message is only what the model said after it; the failure fields follow the hook
// reference and are assumed, which docs/research.md records. session_id is the
// session's conversation (claudeshim_clear_test.go), which rewake compares
// with the one the task was delivered to.
//
// For a failure the reference puts the error text in last_assistant_message,
// with error_details and error beside it, and that is what this sends — not an
// ordinary answer next to an error string, which is a payload the harness does
// not produce.
func hookInput(event, text string) map[string]any {
	input := map[string]any{
		"hook_event_name":        event,
		"last_assistant_message": text,
		"session_id":             shimConversation(),
		"cwd":                    workingDirectory(),
		"transcript_path":        "",
	}
	if event == "Stop" {
		// The first call of a turn end; the call after a hold says true.
		input["stop_hook_active"] = false
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
