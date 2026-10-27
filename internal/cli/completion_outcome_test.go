package cli

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
)

// report publishes an end with the boundary the run's clock gives now, as an
// adapter that cannot hold one does, and answers what the end said.
func (lab interimLab) report(t *testing.T, end adapter.TurnEnd) mail.Outcome {
	t.Helper()
	end.Boundary = boundaryNow(t, lab.dir, lab.self)
	outcome, err := ReportCompletion(context.Background(), lab.dir, lab.self, end)
	if err != nil {
		t.Fatal(err)
	}
	return outcome
}

// The completion says which end finished, decided where the end is published:
// an answered turn under a pending mark of its own is interim and is not
// looked at for what it left unread, an unmarked answered one is, under its
// end; a failed, refused, stopped or empty one is neither, and the same end
// reported again says nothing again.
func TestTheCompletionSaysWhichEndFinished(t *testing.T) {
	lab := newInterimLab(t)
	if err := markPending(lab.dir, "api", lab.self.Epoch(), "waiting on CI", markAt-2); err != nil {
		t.Fatal(err)
	}
	if got := lab.report(t, lab.end("marked", 1, mail.Finished, "CI is running")); !got.Interim || got.Finished != 0 || got.Held != "" {
		t.Fatalf("an answered end under a pending mark said %+v, want interim", got)
	}
	unmarked := lab.end("unmarked", 2, mail.Finished, "CI is green")
	if got := lab.report(t, unmarked); got.Interim || got.Finished != unmarked.Ended {
		t.Fatalf("an unmarked answered end said %+v, want finished at %d", got, unmarked.Ended)
	}
	if got := lab.report(t, unmarked); got != (mail.Outcome{}) {
		t.Fatalf("the same end reported again said %+v, want nothing", got)
	}

	for at, end := range []adapter.TurnEnd{
		{Reason: adapter.Failed, Answer: "broke"},
		{Reason: adapter.Refused, Answer: "no"},
		{Reason: adapter.Aborted, Answer: "stopped"},
		{Reason: adapter.Answered, Answer: " "},
	} {
		end.Event, end.Started, end.Ended = string(end.Reason), lab.start, markAt+3+int64(at)
		if got := lab.report(t, end); got != (mail.Outcome{}) {
			t.Fatalf("a %s end said %+v, want nothing", end.Reason, got)
		}
	}
}

// A held end says only its reason; the continuation that publishes its answer
// is the end that finished.
func TestAHeldEndFinishesWithItsContinuation(t *testing.T) {
	lab := newInterimLab(t)
	end := lab.end("turn-2", 1, mail.Finished, "the suite is green")
	end.Boundary = boundaryNow(t, lab.dir, lab.self)
	held, err := ConfirmCompletion(context.Background(), lab.dir, lab.self, end)
	if err != nil || held.Held == "" || held.Finished != 0 {
		t.Fatalf("the held end said %+v, %v", held, err)
	}
	continued := lab.end("turn-2b", 2, mail.Finished, "")
	continued.Boundary = boundaryNow(t, lab.dir, lab.self)
	got, err := ConfirmCompletion(context.Background(), lab.dir, lab.self, continued)
	if err != nil || got.Held != "" || got.Finished != continued.Ended {
		t.Fatalf("the continuation said %+v, %v; want finished at %d", got, err, continued.Ended)
	}
}
