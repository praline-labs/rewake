package workflow

// The half of the shim a scenario about mid-turn arrival needs: a turn the
// session holds open, the status a working thread reports, and the record of
// what was steered into that turn.
//
// A fixture that answered every turn/start as a fresh turn could not express
// the question at all. The real server decides between starting and steering
// itself — start_or_steer_turn tries to steer an active turn first — so what
// differs between an idle delivery and a mid-turn one is the server's answer,
// not the request. The shim plays that fork; see docs/research-protocol.md for
// where each part of it was read.

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// holdWindow bounds the wait for a second delivery to be steered in. It is a
// ceiling on a wait that a healthy run ends in milliseconds — the sender
// watches the recipient's telemetry and sends as soon as it says working — and
// a run that reaches it has already failed: nothing arrived mid-turn. Ten
// seconds rather than more because the controls that steer nothing wait it out
// in full, and the suite pays that wait once per control.
const holdWindow = 10 * time.Second

// holdOpen keeps the turn in progress until a delivery is steered into it, and
// answers what happened for the turn's own text. It returns "" when this
// session is not holding turns at all.
//
// The pending operation is the session's, not the test's: the wait is recorded
// by the session before it begins and after it ends, so a scenario can see
// that the arrival fell between the two rather than inferring it from a clock.
func (s *shimSession) holdOpen(id string) string {
	s.turn.mu.Lock()
	holding, steered, abort := s.turn.open == id, s.turn.steered, s.turn.abort
	s.turn.mu.Unlock()
	if !holding {
		return ""
	}
	s.recordTurnEvent("operation-open", id, "")
	arrived := false
	select {
	case <-steered:
		arrived = true
	case <-abort:
		// Interrupted: the turn ends here, with nothing more read.
		s.turn.mu.Lock()
		s.turn.open, s.turn.steered = "", nil
		s.turn.mu.Unlock()
		s.recordTurnEvent("operation-aborted", id, "")
		return ""
	case <-time.After(holdWindow):
	}
	// Whatever arrived mid-turn is read inside this same turn, the way an
	// agent reads the mail a steer brought it before answering. A read that
	// fails is recorded and the turn still ends: a session that says nothing
	// leaves its sender waiting for ever.
	second, err := s.readMailbox()
	if err != nil {
		second = "second read failed: " + err.Error()
	}
	s.turn.mu.Lock()
	s.turn.open, s.turn.steered, s.turn.abort = "", nil, nil
	s.turn.mu.Unlock()
	s.recordTurnEvent("operation-closed", id, fmt.Sprintf("steered=%v", arrived))
	return second
}

// recordSteer writes down that a delivery was steered into an open turn, with
// the message it named. This line exists only on the steer branch, so its
// presence is what separates "a delivery reached a working session" from "a
// delivery started a turn of its own".
func (s *shimSession) recordSteer(turn, messageID string) {
	s.recordTurnEvent("steered", turn, messageID)
}

// turnEventMark opens every structured line of the turn log. The older
// free-text lines stay beside these — other scenarios read them — so the two
// have to be told apart, and the mark is what does it. Separating them by the
// presence of a tab would have worked until a message body carried one: the
// free text is the first line of what rewake inbox printed, so its content is
// somebody else's to choose, and a record whose grammar depends on the mail it
// reports is a record that fails on the wrong day.
const turnEventMark = "event"

// recordTurnEvent appends one structured line to the session's turn log.
func (s *shimSession) recordTurnEvent(kind, turn, detail string) {
	s.recordTurn(fmt.Sprintf("%s\t%s\t%s\t%s", turnEventMark, kind, turn, detail))
}

// The status a thread reports while it works, and the notification that
// carries it. Both go through constructors the shape check reads, because a
// message built somewhere else is a message the check never sees.

// activeStatus is the working status. `activeFlags` is required beside the
// type — the schema's active variant demands it — and an empty list is a
// thread working on nothing but its own turn.
func activeStatus() map[string]any {
	return map[string]any{"type": "active", "activeFlags": []string{}}
}

// idleStatus is the other end of it. The idle variant requires the type alone.
func idleStatus() map[string]any {
	return map[string]any{"type": "idle"}
}

func (s *shimSession) threadStatusChangedEvent(status map[string]any) map[string]any {
	return map[string]any{
		"method": "thread/status/changed",
		"params": map[string]any{"threadId": s.thread, "status": status},
	}
}

// turnFailed reports whether this turn must end in a failure. Two switches
// reach it: one fails a turn after its answer was sent, the other fails the
// held operation a mid-turn scenario is built on.
func turnFailed() bool {
	return os.Getenv(shimLateFailure) != "" || os.Getenv(shimFailHeldTurn) != ""
}

// turnOutcome is what the session records about how its turn ended.
func turnOutcome() string {
	if turnFailed() {
		return "failed"
	}
	return "completed"
}

// sendSecond is the sender's part of a mid-turn scenario: wait for the
// recipient to reach a named state, then send one more letter.
//
// The state is read from the recipient's own telemetry through this session's
// rewake, not from a sleep: "working" is the thread reporting an active status,
// which it does when its turn begins. That is the same evidence the live probe
// of September 21, 2026 used.
func sendSecond() {
	text := os.Getenv(shimSendSecondText)
	target := os.Getenv(shimSendTo)
	if text == "" || target == "" {
		return
	}
	want, state := os.Getenv(shimSendWhenWorking), "working"
	if want == "" {
		want, state = os.Getenv(shimSendWhenIdle), "idle"
	}
	if want == "" {
		return
	}
	seen, reached := awaitActivity(want, state)
	if !reached {
		// Not an exit: the letter still goes, and the scenario reads the
		// recipient's record to see that it did not arrive mid-turn. Leaving
		// here would turn a finding about delivery into a missing sender.
		fmt.Fprintf(os.Stderr, "shim: %s never reported %s; sending anyway\n", want, state)
	}
	// What the sender saw before it sent, recorded whether or not it was what
	// it waited for: a letter that went out at the wrong moment and one that
	// went out at the right one look identical afterwards.
	recordSend("second-wait", state, "last seen "+seen)
	send := rewakeCommand("send", target, text)
	out, err := send.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "shim: sending to %s: %v: %s\n", target, err, out)
		recordSend("second", "refused", firstLine(strings.TrimSpace(string(out))))
		return
	}
	recordSend("second", "accepted", firstLine(strings.TrimSpace(string(out))))
}

// recordSend writes down what became of one letter this session sent. A
// refusal is a result like any other, and a scenario that could not see one
// would read a letter that was never delivered as a letter still on its way.
func recordSend(which, outcome, detail string) {
	if target := os.Getenv(shimSendsFile); target != "" {
		appendLine(target, fmt.Sprintf("%s\t%s\t%s", which, outcome, detail))
	}
}

// awaitActivity polls this session's own view of the room until the named
// session reports the state asked for. Bounded: a recipient that never gets
// there is the scenario's finding, not the sender's to wait on for ever.
//
// "idle" has to be seen *after* working, or the control that must deliver
// after the turn would send while the recipient had not started yet — which
// is not "after the turn ends" but "before it begins", and the two look the
// same in a snapshot.
func awaitActivity(name, state string) (string, bool) {
	deadline := time.Now().Add(holdWindow + 10*time.Second)
	worked := state != "idle"
	seen := "nothing"
	for time.Now().Before(deadline) {
		if current, ok := activityOf(name); ok {
			seen = current
			if current == "working" {
				worked = true
			}
			if worked && current == state {
				return seen, true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return seen, false
}

// activityOf answers the activity this session's rewake reports for another,
// and whether it could be read at all. A listing that cannot be read is not an
// absent activity: the caller keeps waiting rather than concluding.
func activityOf(name string) (string, bool) {
	out, err := rewakeCommand("list", "--json").Output()
	if err != nil {
		return "", false
	}
	var current listing
	if unmarshalJSON(out, &current) != nil {
		return "", false
	}
	activity, _, _, found := current.find(name)
	if !found {
		return "", false
	}
	return activity, true
}
