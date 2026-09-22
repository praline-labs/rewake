package workflow

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMidTurn is the mid-turn scenario: a letter delivered to a session that
// is already working reaches it without waiting for the turn to end, and the
// work that was already running still reports its own result.
//
// What makes the question askable at all is the fork the real server owns. The
// product sends the same request either way — a turn/start — and the server
// steers it into the running turn or starts a new one. So the fixture plays
// that fork, and what the scenario reads is the recipient's own record of
// which branch it took.
//
// The pending operation is the recipient's, not a sleep in the test: the
// session holds its turn open, writes that down before it waits and after it
// stops, and the sender only sends the second letter once the room's telemetry
// says the recipient is working.
//
// What this does not prove: that a model read the steered letter mid-turn.
// This is the fixture tier; the only observation of that kind is the live one
// of September 21, 2026 in claude-parity-2026-09-21.md.
func TestMidTurn(t *testing.T) {
	binary := enterScenario(t, "mid-turn")
	c := Start(t, Spec{
		Name:    "mid-turn",
		Harness: "codex",
		Observations: []string{
			obsMidReady, obsPending, obsSteered, obsOriginalOutcome, obsNoSecondOutcome, obsMidAlive,
		},
		Deadline: 120 * time.Second,
	})
	if !offers(c, "codex", capabilityMidTurn) {
		return
	}
	iso := Isolate(t, c, binary)
	worker, sender := startMidTurnSessions(t, c, iso, midTurnLive)
	defer stopSession(t, c, worker)
	defer stopSession(t, c, sender)

	if !awaitMidTurnReady(c, worker, sender) {
		return
	}
	c.Observed(obsMidReady, "selection ready")

	// Anchored on the recipient's own record of having closed the operation:
	// until then the turn is still running and there is nothing to judge.
	c.Await("the recipient to finish the turn it held open", func() bool {
		events, err := worker.turnEvents()
		if err != nil {
			c.t.Fatalf("workflow: %v", err)
		}
		return eventOf(events, "completed") != nil
	})

	events, err := worker.turnEvents()
	if err != nil {
		c.t.Fatalf("workflow: %v", err)
	}
	open, steered, closed := eventOf(events, "operation-open"), eventOf(events, "steered"), eventOf(events, "operation-closed")
	switch {
	case open == nil || closed == nil:
		c.Contradicted(obsPending, "the recipient never recorded an operation it held open: %v", events)
	case steered == nil:
		c.Contradicted(obsPending, "nothing was steered into the open turn, so it was not pending at a delivery")
	case open.At >= steered.At || steered.At >= closed.At:
		c.Contradicted(obsPending, "the order was open=%d steered=%d closed=%d", open.At, steered.At, closed.At)
	default:
		c.Observed(obsPending, fmt.Sprintf("%s was open when the delivery arrived and closed after it", open.Turn))
	}

	// The steer line exists only on the branch the server takes when a turn is
	// in progress, so its presence is what separates a delivery that reached a
	// working session from one that started a turn of its own. The message it
	// names has to be the second letter, and rewake has to have recorded that
	// same delivery — the fixture alone could claim anything.
	second, known := messageCarrying(worker, midTurnSecond)
	switch {
	case steered == nil:
		c.Contradicted(obsSteered, "no delivery was steered into an open turn")
	case !known:
		c.Contradicted(obsSteered, "the recipient's own read never returned the second letter")
	case steered.Detail != second.ID:
		c.Contradicted(obsSteered, "the steer named %q, the second letter read here was %s", steered.Detail, second.ID)
	case !slices.Contains(worker.deliveredIDs(), second.ID):
		c.Contradicted(obsSteered, "the delivery of %s was not among those rewake named: %v", second.ID, worker.deliveredIDs())
	default:
		c.Observed(obsSteered, "the second letter was steered into "+steered.Turn)
	}

	// The sender reads its own mailbox when the report reaches it, which is a
	// turn of its own and does not happen the instant the recipient finishes.
	// Anchored on the sender's record rather than on a pause after it.
	c.Await("the sender to read an outcome from "+worker.name, func() bool {
		return outcomesRead(sender, worker) > 0
	})

	// The work that was already running still answers: one report, and it
	// settles the letter that started the turn.
	first, firstKnown := messageCarrying(worker, midTurnFirst)
	report, reported := reportOfKind(sender, worker, "finished")
	completions := allOf(events, "completed")
	switch {
	case !firstKnown:
		c.Contradicted(obsOriginalOutcome, "the recipient's own read never returned the first letter")
	case !reported:
		c.Contradicted(obsOriginalOutcome, "no finished report from %s; completions recorded: %v", worker.name, details(completions))
	case !slices.Contains(report.InReplyTo, first.ID):
		c.Contradicted(obsOriginalOutcome, "the report settles %v, which does not include the letter that started the turn, %s", report.InReplyTo, first.ID)
	default:
		c.Observed(obsOriginalOutcome, "one finished report settling "+strings.Join(report.InReplyTo, ", "))
	}

	// And the arrival did not become work of its own: the turn it joined ended
	// once, and the sender was told once.
	reports := outcomesRead(sender, worker)
	switch {
	case steered == nil:
		c.Contradicted(obsNoSecondOutcome, "nothing was steered, so there is no joined turn to count outcomes for")
	case len(completions) != 1:
		c.Contradicted(obsNoSecondOutcome, "the recipient recorded %d turn completions: %v", len(completions), details(completions))
	case reports != 1:
		c.Contradicted(obsNoSecondOutcome, "the sender read %d outcomes from %s, not one", reports, worker.name)
	default:
		c.Observed(obsNoSecondOutcome, "one completion of "+steered.Turn+", one outcome at the sender")
	}

	if !worker.alive() || !sender.alive() {
		c.Contradicted(obsMidAlive, "worker alive: %v, sender alive: %v", worker.alive(), sender.alive())
		return
	}
	c.Observed(obsMidAlive, "neither session left before the verdict")
}

// The observations, named once: the scenario records them and the controls
// name which of them their breakage must take down.
const (
	obsMidReady        = "the recipient reaches an accepted conversation"
	obsPending         = "the operation is pending when the delivery arrives"
	obsSteered         = "the delivery is accepted into the open turn"
	obsOriginalOutcome = "the original turn completes with its own outcome"
	obsNoSecondOutcome = "the arrival created no second outcome"
	obsMidAlive        = "both sessions are still running when the case is judged"
)

// The two letters, told apart by text: the first starts the turn, the second
// is the one that has to reach a session already working.
const (
	midTurnFirst  = "mid-turn-first-please-hold"
	midTurnSecond = "mid-turn-second-arrives-mid-work"
)

// capabilityMidTurn is what a harness must offer for this scenario to mean
// anything: its fixture has to show a delivery reaching a session that is
// already working. A harness that does not offer it is unsupported, which is
// not a pass — see classification_test.go.
const capabilityMidTurn = "observes-mid-turn-arrival"

// capabilities is what each harness fixture declares. Claude Code has no
// fixture at all yet, so it declares nothing and every scenario that needs a
// capability reports unsupported for it rather than quietly skipping.
var capabilities = map[string]map[string]bool{
	"codex": {capabilityMidTurn: true},
}

// offers records every observation as unsupported when the harness lacks the
// capability, and answers whether the case may go on. Saying so by name is the
// point: a missing mechanism that stayed silent would read as a defect that
// swallowed the evidence.
func offers(c *Case, harness, capability string) bool {
	if capabilities[harness][capability] {
		return true
	}
	for _, observation := range c.spec.Observations {
		c.Unsupported(observation, harness+" declares no "+capability)
	}
	return false
}

// midTurnTiming says when the second letter leaves: as soon as the recipient
// reports working, or only once it is idle again — which is the control that
// must take the mid-turn observation down.
type midTurnTiming int

const (
	midTurnLive midTurnTiming = iota
	midTurnLate
)

// startMidTurnSessions launches a recipient that holds its first turn open and
// a sender that sends one letter now and one when the recipient reaches the
// state this run is about.
func startMidTurnSessions(t *testing.T, c *Case, iso *Isolation, when midTurnTiming, controls ...string) (*codexSession, *codexSession) {
	t.Helper()
	worker := startCodexSession(t, c, iso, "worker", "--general",
		append([]string{shimInboxJSON + "=1", shimHoldTurn + "=1"}, controls...)...)
	timing := shimSendWhenWorking + "=" + worker.name
	if when == midTurnLate {
		timing = shimSendWhenIdle + "=" + worker.name
	}
	sender := startCodexSession(t, c, iso, "sender", "--main",
		shimInboxJSON+"=1", shimSendTo+"="+worker.name, shimSendWhenReady+"="+worker.name,
		shimSendText+"="+midTurnFirst, timing, shimSendSecondText+"="+midTurnSecond)
	return worker, sender
}

func awaitMidTurnReady(c *Case, worker, sender *codexSession) bool {
	if _, ok := sender.await(c, "a selection for "+worker.name, func(l listing) bool {
		_, selection, _, found := l.find(worker.name)
		return found && selection == "ready"
	}); !ok {
		c.Contradicted(obsMidReady, "%s never became ready", worker.name)
		return false
	}
	return true
}

// turnEvent is one line of the recipient's structured turn log: what happened,
// to which turn, with what detail, and where it fell in the order the session
// wrote them.
type turnEvent struct {
	Kind   string
	Turn   string
	Detail string
	At     int
}

// turnEvents are the structured lines of the log, in order. A line is one of
// this reader's when it opens with the mark the shim writes; the older
// free-text lines are skipped, because they are another scenario's record and
// their content belongs to whoever sent the mail. A marked line that cannot be
// read is an error rather than an absent record: that is a record the scenario
// would otherwise judge by its absence.
func (s *codexSession) turnEvents() ([]turnEvent, error) {
	var events []turnEvent
	for index, line := range strings.Split(strings.TrimRight(s.acceptedTurns(), "\n"), "\n") {
		rest, marked := strings.CutPrefix(line, turnEventMark+"\t")
		if !marked {
			continue
		}
		fields := strings.SplitN(rest, "\t", 3)
		if len(fields) != 3 || fields[0] == "" || fields[1] == "" {
			return nil, errUnreadableRecord(s.turns, line)
		}
		events = append(events, turnEvent{Kind: fields[0], Turn: fields[1], Detail: fields[2], At: index})
	}
	return events, nil
}

// kindOf is the first event of one kind, or nil when there is none. A pointer
// rather than a value and a flag: the callers ask about several kinds at once
// and a zero event that reads as "found" is the mistake this suite keeps
// finding.
func eventOf(events []turnEvent, kind string) *turnEvent {
	for index := range events {
		if events[index].Kind == kind {
			return &events[index]
		}
	}
	return nil
}

func allOf(events []turnEvent, kind string) []turnEvent {
	var found []turnEvent
	for _, event := range events {
		if event.Kind == kind {
			found = append(found, event)
		}
	}
	return found
}

// sendRecord is what a sender wrote down about one letter it sent.
type sendRecord struct {
	Which, Outcome, Detail string
}

// sendRecords are the letters this session recorded sending, in order. A line
// that cannot be read is an error rather than an absent record, for the same
// reason everywhere else in this suite: a reader that cannot look has not
// looked.
func (s *codexSession) sendRecords() ([]sendRecord, error) {
	raw, err := os.ReadFile(s.sends)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var records []sendRecord
	for _, line := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) != 3 || fields[0] == "" || fields[1] == "" {
			return nil, errUnreadableRecord(s.sends, line)
		}
		records = append(records, sendRecord{Which: fields[0], Outcome: fields[1], Detail: fields[2]})
	}
	return records, nil
}

// sendOf is what became of one named letter, and whether the sender recorded
// it at all.
func sendOf(records []sendRecord, which string) (sendRecord, bool) {
	for _, record := range records {
		if record.Which == which {
			return record, true
		}
	}
	return sendRecord{}, false
}

// outcomesRead is how many outcomes of a session's work the reader has read.
// Both kinds count: a turn that failed answered too, and a count that only
// knew about successes would call a failed run "no outcome" and wait for ever.
func outcomesRead(reader, about *codexSession) int {
	return countReports(reader, about, "finished") + countReports(reader, about, "error")
}

func details(events []turnEvent) []string {
	out := make([]string, 0, len(events))
	for _, event := range events {
		out = append(out, event.Turn+"="+event.Detail)
	}
	return out
}
