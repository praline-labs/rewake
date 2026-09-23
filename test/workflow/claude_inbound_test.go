package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestClaudeInbound is the Claude Code inbound gate end to end: what a sender
// is told when the receiving session holds its notice, and when the first
// notice to a new session goes out. Five workers, each with a sender of its
// own: one that has only just started, one whose gate holds the task and then
// releases it, one whose gate holds it until it expires, one whose gate
// refuses it, and one whose gate says it holds the task only after rewake has
// counted it delivered, and then lets it expire.
//
// Only this column: Codex has no such gate, and its column is unchanged.
//
// What it does not prove: that the real harness holds, releases and reports
// as its fixture does. The fixture plays what was read in the binary and seen
// live (docs/research-launch.md, docs/research.md).
func TestClaudeInbound(t *testing.T) {
	binary := enterScenario(t, "claude-inbound")
	c := Start(t, Spec{
		Name:         "claude-inbound",
		Harness:      claudeColumn.harness,
		Observations: inboundObservations,
		Deadline:     90 * time.Second,
	})
	for _, finding := range playClaudeInbound(t, c, Isolate(t, c, binary)) {
		if finding.held {
			c.Observed(finding.observation, finding.detail)
		} else {
			c.Contradicted(finding.observation, "%s", finding.detail)
		}
	}
}

const (
	obsStartAccepted   = "the first notice to a new session goes out once it is up, and is not held"
	obsReleaseReported = "a task held and then released is reported delivered after the hold"
	obsReleaseWorked   = "the released task is worked and reported"
	obsHeldReported    = "a task still held when send stops waiting is reported held, exit 3"
	obsExpiryTold      = "the sender of a held task that expired is told it was not delivered"
	obsExpiryUnsettled = "a held task that expired is failed, never read and not owed"
	obsRefusalReported = "a refused task is reported failed, exit 1, and never worked"
	obsLateHoldTold    = "a task held after it was counted delivered ends failed, never worked, and its sender is told"
	inboundStartText   = "inbound-start-probe"
	inboundReleaseText = "inbound-release-probe"
	inboundExpireText  = "inbound-expire-probe"
	inboundRefuseText  = "inbound-refuse-probe"
	inboundLateText    = "inbound-late-probe"
)

var inboundObservations = []string{
	obsStartAccepted, obsReleaseReported, obsReleaseWorked, obsHeldReported, obsExpiryTold, obsExpiryUnsettled,
	obsRefusalReported, obsLateHoldTold,
}

// inboundPair is a worker whose gate does what its mode says, and the session
// that sends it one task.
type inboundPair struct {
	worker, sender *codexSession
	inbound        string
}

func startInboundPair(t *testing.T, c *Case, iso *Isolation, label, mode string, waitForMount bool) inboundPair {
	t.Helper()
	pair := inboundPair{inbound: filepath.Join(iso.Home, label+".inbound")}
	controls := []string{shimInboundFile + "=" + pair.inbound}
	if mode != "" {
		controls = append(controls, shimInbound+"="+mode)
	}
	pair.worker = startHarnessSession(t, c, iso, claudeColumn.harness, label, "--general", controls...)
	// A sender that waits only for the socket sends into the startup; one
	// that waits for the session to be up tests the gate it has then.
	ready := pair.worker.ready
	if waitForMount {
		ready += ".mounted"
	}
	text := map[string]string{
		"starting": inboundStartText, "releasing": inboundReleaseText, "expiring": inboundExpireText,
		"refusing": inboundRefuseText, "late": inboundLateText,
	}[label]
	pair.sender = startHarnessSession(t, c, iso, claudeColumn.harness, "to-"+label, "--general",
		shimSendTo+"="+pair.worker.name, shimSendText+"="+text, shimInboxJSON+"=1", shimWaitForFile+"="+ready)
	return pair
}

// events is what the worker's gate recorded, in order.
func (p inboundPair) events() []string {
	raw, err := os.ReadFile(p.inbound)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if event, _, _ := strings.Cut(line, "\t"); event != "" {
			out = append(out, event)
		}
	}
	return out
}

// sent is what the sender recorded about its one letter.
func (p inboundPair) sent(c *Case) (sendRecord, bool) {
	var record sendRecord
	found := waitFor(c, 20*time.Second, func() bool {
		records, err := p.sender.sendRecords()
		if err != nil {
			return false
		}
		var ok bool
		record, ok = sendOf(records, "letter-1")
		return ok
	})
	return record, found
}

func playClaudeInbound(t *testing.T, c *Case, iso *Isolation) []telemetryFinding {
	t.Helper()
	starting := startInboundPair(t, c, iso, "starting", "", false)
	releasing := startInboundPair(t, c, iso, "releasing", "hold-release", true)
	expiring := startInboundPair(t, c, iso, "expiring", "hold-expire", true)
	refusing := startInboundPair(t, c, iso, "refusing", "refuse", true)
	late := startInboundPair(t, c, iso, "late", "hold-late", true)
	for _, pair := range []inboundPair{starting, releasing, expiring, refusing, late} {
		defer stopSession(t, c, pair.sender)
		defer stopSession(t, c, pair.worker)
	}
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	unjudged := func(observation, detail string) telemetryFinding {
		return telemetryFinding{observation: observation, detail: detail}
	}
	var out []telemetryFinding

	// The session that had only just started.
	if record, ok := starting.sent(c); !ok || !waitFor(c, 10*time.Second, func() bool { return len(starting.events()) > 0 }) {
		out = append(out, unjudged(obsStartAccepted, fmt.Sprintf("no send record or gate record: %+v, %v", record, starting.events())))
	} else {
		events := starting.events()
		out = append(out, finding(obsStartAccepted, events[0] == "accepted" && record.Outcome == "exit=0" && !strings.Contains(record.Detail, "released"),
			"gate %v; the sender was told %s %q", events, record.Outcome, record.Detail))
	}

	// Held, then released.
	if record, ok := releasing.sent(c); !ok {
		out = append(out, unjudged(obsReleaseReported, "the sender recorded no send"))
	} else {
		waitFor(c, 5*time.Second, func() bool { return len(releasing.events()) >= 2 })
		out = append(out, finding(obsReleaseReported,
			record.Outcome == "exit=0" && strings.Contains(record.Detail, "released after being held") && slices.Equal(releasing.events(), []string{"held", "released"}),
			"gate %v; the sender was told %s %q", releasing.events(), record.Outcome, record.Detail))
	}
	worked := waitFor(c, 20*time.Second, func() bool { return slices.Contains(kindsFrom(releasing.sender, releasing.worker), "finished") })
	out = append(out, finding(obsReleaseWorked, worked, "the sender read %v from the worker", kindsFrom(releasing.sender, releasing.worker)))

	// Held until it expired.
	if record, ok := expiring.sent(c); !ok {
		out = append(out, unjudged(obsHeldReported, "the sender recorded no send"))
	} else {
		out = append(out, finding(obsHeldReported, record.Outcome == "exit=3" && strings.HasPrefix(record.Detail, "held for "+expiring.worker.name+": "),
			"the sender was told %s %q", record.Outcome, record.Detail))
	}
	// The note and the status follow the fixture's last word within a moment,
	// so the wait for them starts there: a mutant that sends no note is then
	// judged in seconds rather than at the end of a long wait.
	if !expiring.finalWord(c) {
		out = append(out, unjudged(obsExpiryTold, fmt.Sprintf("the fixture never let the hold expire: gate %v", expiring.events())),
			unjudged(obsExpiryUnsettled, "the hold never expired"))
		return append(out, playRefusalAndLate(c, iso, refusing, late)...)
	}
	var note reportView
	told := waitFor(c, afterFinalWord, func() bool {
		for _, message := range readMessages(expiring.sender) {
			if message.From == expiring.worker.name && message.Undelivered != nil {
				note = message
				return true
			}
		}
		return false
	})
	// Judged on the worker's own mailbox, whatever the sender was told.
	var state string
	waitFor(c, afterFinalWord, func() bool {
		state = taskState(iso, expiring.worker)
		return state != "" && state != "held" && state != "pending"
	})
	out = append(out, finding(obsExpiryTold, told && note.Kind == "notify" && note.Undelivered.Kind == "task" && strings.Contains(note.Text, inboundExpireText),
		"a note from the worker naming the task: %v; kinds the sender read from it: %v", told, kindsFrom(expiring.sender, expiring.worker)))
	owed, _ := filepath.Glob(filepath.Join(iso.StateDir, "rooms", "default", "inbox", expiring.worker.name, "awaiting", "*", "*"))
	out = append(out, finding(obsExpiryUnsettled, state == "failed" && expiring.worker.acceptedTurns() == "" && len(owed) == 0,
		"the task's status %q; turns worked %q; owed %v; gate %v", state, expiring.worker.acceptedTurns(), owed, expiring.events()))
	return append(out, playRefusalAndLate(c, iso, refusing, late)...)
}

// afterFinalWord is how long the note and the status get once the fixture has
// said its last word about a line.
const afterFinalWord = 3 * time.Second

// finalWord waits for the fixture to record that the pair's hold expired.
func (p inboundPair) finalWord(c *Case) bool {
	return waitFor(c, 20*time.Second, func() bool { return slices.Contains(p.events(), "expired") })
}

func playRefusalAndLate(c *Case, iso *Isolation, refusing, late inboundPair) []telemetryFinding {
	finding := func(observation string, held bool, detail string, args ...any) telemetryFinding {
		return telemetryFinding{observation: observation, held: held, judged: true, detail: fmt.Sprintf(detail, args...)}
	}
	var out []telemetryFinding

	// Refused outright.
	if record, ok := refusing.sent(c); !ok {
		out = append(out, telemetryFinding{observation: obsRefusalReported, detail: "the sender recorded no send"})
	} else {
		out = append(out, finding(obsRefusalReported,
			record.Outcome == "exit=1" && strings.Contains(record.Detail, "refuses messages from other sessions") &&
				taskState(iso, refusing.worker) == "failed" && refusing.worker.acceptedTurns() == "",
			"the sender was told %s %q; the task's status %q; turns worked %q; gate %v",
			record.Outcome, record.Detail, taskState(iso, refusing.worker), refusing.worker.acceptedTurns(), refusing.events()))
	}

	// Held only after the delivery was counted. What send said depends on how
	// soon it looked; what counts is how the task ends.
	record, _ := late.sent(c)
	if !late.finalWord(c) {
		return append(out, telemetryFinding{observation: obsLateHoldTold, detail: fmt.Sprintf("the fixture never let the late hold expire: gate %v", late.events())})
	}
	lateTold := waitFor(c, afterFinalWord, func() bool { return undeliveredFrom(late) })
	var lateState string
	waitFor(c, afterFinalWord, func() bool {
		lateState = taskState(iso, late.worker)
		return lateState == "failed"
	})
	out = append(out, finding(obsLateHoldTold, lateTold && lateState == "failed" && late.worker.acceptedTurns() == "",
		"a note naming the task: %v; the task's status %q; turns worked %q; the sender was told %s %q; gate %v",
		lateTold, lateState, late.worker.acceptedTurns(), record.Outcome, record.Detail, late.events()))
	return out
}

// undeliveredFrom reports whether the pair's sender has read a note from its
// worker saying its task was not delivered.
func undeliveredFrom(p inboundPair) bool {
	for _, message := range readMessages(p.sender) {
		if message.From == p.worker.name && message.Undelivered != nil && message.Undelivered.Kind == "task" && strings.Contains(message.Text, inboundLateText) {
			return true
		}
	}
	return false
}

// taskState is the status of the one message in a worker's mailbox.
func taskState(iso *Isolation, worker *codexSession) string {
	statuses, _ := filepath.Glob(filepath.Join(iso.StateDir, "rooms", "default", "inbox", worker.name, "*.status"))
	if len(statuses) != 1 {
		return ""
	}
	raw, err := os.ReadFile(statuses[0])
	if err != nil {
		return ""
	}
	var status struct {
		State string `json:"state"`
	}
	if unmarshalJSON(raw, &status) != nil {
		return ""
	}
	return status.State
}
