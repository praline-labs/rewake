package cli

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// mainRun makes a live main named boss in dir.
func mainRun(t *testing.T, dir string) registry.Session {
	t.Helper()
	session := otherSession(t, dir, "boss")
	session.Role = role.Main.ID
	if err := registry.Publish(dir, session); err != nil {
		t.Fatal(err)
	}
	return session
}

func endNow(t *testing.T, dir string, self registry.Session, id, answer string) adapter.TurnEnd {
	t.Helper()
	return adapter.TurnEnd{Event: id, Reason: adapter.Answered, Answer: answer, Boundary: boundaryNow(t, dir, self), Ended: boottime.Now()}
}

func notifiesTo(t *testing.T, dir, name string) []mail.Message {
	t.Helper()
	var out []mail.Message
	for _, message := range reportsTo(t, dir, name) {
		if mail.KindOf(message) == mail.Note {
			out = append(out, message)
		}
	}
	return out
}

// forgetDone removes the done records of name, as an end that failed before
// its last step leaves them.
func forgetDone(t *testing.T, dir, name string) {
	t.Helper()
	found, _ := filepath.Glob(filepath.Join(mail.JournalPath(dir, name), "*.done"))
	for _, path := range found {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
}

// The live case of brief 99: an unmarked acknowledgment settles what was
// waiting, and the real result, a later end, then reaches main once.
func TestAnAnswerNoWaiterTakesReachesMainOnce(t *testing.T) {
	dir, self, ack := ownerLab(t)
	mainRun(t, dir)
	ack.Answer = "Noted, still waiting on the checks"
	if _, err := ReportCompletion(context.Background(), dir, self, ack); err != nil {
		t.Fatal(err)
	}
	if got := notifiesTo(t, dir, "boss"); len(got) != 0 {
		t.Fatalf("an end that reported to its waiter sent main %d notifies", len(got))
	}
	final := endNow(t, dir, self, "harness/end-2", "brief 97: all green")
	for attempt := range 2 {
		if _, err := ReportCompletion(context.Background(), dir, self, final); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		forgetDone(t, dir, "api")
	}
	got := notifiesTo(t, dir, "boss")
	if len(got) != 1 || got[0].From != "api" || len(got[0].InReplyTo) != 0 ||
		!strings.HasPrefix(got[0].Text, "Unclaimed answer: api's turn ended") || !strings.HasSuffix(got[0].Text, "brief 97: all green") {
		t.Fatalf("main holds %+v, want one unclaimed answer", got)
	}
	if got := reportsTo(t, dir, "web"); len(got) != 1 {
		t.Fatalf("web holds %d reports, want the acknowledgment alone", len(got))
	}
}

// An end retried after it reported and cleared its wait finds no waiter, and
// the receipt keeps it from sending its answer to main as well.
func TestARetriedOrdinaryEndSendsNoUnclaimedAnswer(t *testing.T) {
	dir, self, end := ownerLab(t)
	mainRun(t, dir)
	for attempt := range 2 {
		if _, err := ReportCompletion(context.Background(), dir, self, end); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
		forgetDone(t, dir, "api")
	}
	if got := notifiesTo(t, dir, "boss"); len(got) != 0 {
		t.Fatalf("main holds %d notifies for an answer web took", len(got))
	}
	if got := reportsTo(t, dir, "web"); len(got) != 1 {
		t.Fatalf("web holds %d reports, want one", len(got))
	}
}

// Empty and stopped ends, and main's own, add nothing to the net.
func TestTheNetTakesOnlyAWorkersNonemptyFinish(t *testing.T) {
	dir, self, _ := ownerLab(t)
	mainRun(t, dir)
	stopped := endNow(t, dir, self, "harness/stop", "x")
	stopped.Reason = adapter.Aborted
	if _, err := ReportCompletion(context.Background(), dir, self, stopped); err != nil {
		t.Fatal(err)
	}
	if got := notifiesTo(t, dir, "boss"); len(got) != 0 {
		t.Fatalf("main holds %d notifies from a stopped end", len(got))
	}
	boss, err := registry.Lookup(dir, "boss")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReportCompletion(context.Background(), dir, boss, endNow(t, dir, boss, "harness/main", "main talking")); err != nil {
		t.Fatal(err)
	}
	if got := reportsTo(t, dir, "boss"); len(got) != 0 {
		t.Fatalf("main's own end left %d letters", len(got))
	}
}

// An end whose answer is empty after trimming is not an answer to carry: the
// worker read nothing, so there is no error either, and main hears nothing.
func TestTheNetTakesNoEmptyAnswer(t *testing.T) {
	dir := liveSession(t, "api")
	mainRun(t, dir)
	self, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReportCompletion(context.Background(), dir, self, endNow(t, dir, self, "harness/blank", " \n ")); err != nil {
		t.Fatal(err)
	}
	if got := reportsTo(t, dir, "boss"); len(got) != 0 {
		t.Fatalf("an empty answer sent main %d letters", len(got))
	}
}

// The whole incident of brief 99: a pending end, a second task read, an
// unmarked acknowledgment that settles both, then the real result.
func TestTheIncidentOfBrief97ReachesMainOnce(t *testing.T) {
	dir := liveSession(t, "api")
	boss := mainRun(t, dir)
	self, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, "api")
	t.Setenv(epochEnv, self.Epoch())
	read := func(text string) string {
		id := rawUnread(t, dir, "api", map[string]any{"from": boss.Name, "fromEpoch": boss.Epoch(), "toEpoch": self.Epoch(), "text": text})
		if code, _, why := run("inbox"); code != ExitOK {
			t.Fatalf("read: %d %s", code, why)
		}
		return id
	}
	first := read("brief 97: run the checks")
	turnStarted(t, dir, self, markAt-1)
	if code, _, why := run("pending", "waiting on the background checks"); code != ExitOK {
		t.Fatalf("pending: %d %s", code, why)
	}
	interim := endNow(t, dir, self, "incident/pending", "checks still running")
	interim.Started = markAt - 1
	if outcome, err := ReportCompletion(context.Background(), dir, self, interim); err != nil || !outcome.Interim {
		t.Fatalf("interim end: %+v %v", outcome, err)
	}
	second := read("unrelated second task")
	if _, err := ReportCompletion(context.Background(), dir, self, endNow(t, dir, self, "incident/ack", "Noted; still waiting on the checks")); err != nil {
		t.Fatal(err)
	}
	var settled []mail.Message
	for _, message := range reportsTo(t, dir, boss.Name) {
		if mail.KindOf(message) == mail.Finished {
			settled = append(settled, message)
		}
	}
	if len(settled) != 1 || !slices.Equal(settled[0].InReplyTo, []string{first, second}) || len(mail.Open(dir, self.Name, self.Epoch()).Waiters()) != 0 {
		t.Fatalf("the acknowledgment did not settle both tasks: %+v", settled)
	}
	final := endNow(t, dir, self, "incident/final", "brief 97: checks and results")
	for attempt := range 2 {
		if _, err := ReportCompletion(context.Background(), dir, self, final); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	notices := notifiesTo(t, dir, boss.Name)
	if len(notices) != 1 || len(notices[0].InReplyTo) != 0 || notices[0].ToEpoch != boss.Epoch() || !strings.HasSuffix(notices[0].Text, final.Answer) {
		t.Fatalf("the final answer did not reach main exactly once: %+v", notices)
	}
	if got := reportsTo(t, dir, boss.Name); len(got) != 3 {
		t.Fatalf("main holds %d letters, want the pending, the settling report and the notify", len(got))
	}
}

// A receipt that cannot be written fails the end before any wait is cleared;
// the retry reports nothing twice and sends no unclaimed answer.
func TestAFailedReceiptKeepsTheWaitAndTheRetrySettlesOnce(t *testing.T) {
	dir, self, end := ownerLab(t)
	mainRun(t, dir)
	hit := false
	state.Fault = func(op, path string) error {
		if op == state.OpWrite && strings.HasSuffix(path, ".reported") && !hit {
			hit = true
			return os.ErrPermission
		}
		return nil
	}
	t.Cleanup(func() { state.Fault = nil })
	if _, err := ReportCompletion(context.Background(), dir, self, end); err == nil || !hit {
		t.Fatalf("the receipt did not fail: hit=%v err=%v", hit, err)
	}
	if got := reportsTo(t, dir, "web"); len(got) != 1 || len(mail.Open(dir, self.Name, self.Epoch()).Waiters()) != 1 {
		t.Fatalf("web holds %d reports, or the wait was cleared", len(got))
	}
	state.Fault = nil
	for attempt := range 2 {
		if _, err := ReportCompletion(context.Background(), dir, self, end); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if len(reportsTo(t, dir, "web")) != 1 || len(notifiesTo(t, dir, "boss")) != 0 || len(mail.Open(dir, self.Name, self.Epoch()).Waiters()) != 0 {
		t.Fatal("the retry duplicated a report, sent a notify, or left the wait")
	}
}
