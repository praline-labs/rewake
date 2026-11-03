package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/boottime"
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
