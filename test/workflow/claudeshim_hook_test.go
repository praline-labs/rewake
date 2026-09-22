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
	"strings"
)

// claudeSettings is the part of the settings layer this fixture uses: the
// command each hook runs. What the wrapper puts there is checked, because a
// session that never ran the hook would leave every sender waiting and the
// scenario would report it as a delivery that produced no answer.
type claudeSettings struct {
	stop        string
	stopFailure string
}

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
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(raw), &layer); err != nil {
		return settings, fmt.Errorf("unreadable --settings: %w", err)
	}
	for name, matchers := range layer.Hooks {
		for _, matcher := range matchers {
			for _, hook := range matcher.Hooks {
				if hook.Kind != "command" || hook.Command == "" {
					return settings, fmt.Errorf("the %s hook is not a command", name)
				}
				switch name {
				case "Stop":
					settings.stop = hook.Command
				case "StopFailure":
					settings.stopFailure = hook.Command
				default:
					return settings, fmt.Errorf("a hook this fixture does not serve: %s", name)
				}
			}
		}
	}
	if settings.stopFailure == "" {
		// Every role gets this one; only Stop depends on whether the role
		// reports at all.
		return settings, errors.New("no StopFailure hook in --settings")
	}
	return settings, nil
}

// workTurn is a turn of this column's session: read the mail the notice was
// about, then end the turn through the hook.
func (s *claudeSession) workTurn(turn string, notice claudeNotice) {
	s.recordDelivery(turn, notice)

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
	if command == "" {
		return
	}
	payload, err := json.Marshal(hookInput(event, text))
	if err != nil {
		fmt.Fprintf(os.Stderr, "claude-shim: the hook payload could not be written: %v\n", err)
		return
	}
	// The hook command is a quoted argv, as the settings layer spells it, so
	// it runs through a shell the way the harness runs it.
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
