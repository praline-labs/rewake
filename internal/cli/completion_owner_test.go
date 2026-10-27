package cli

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// ownerLab is api having read web's task, with an end of api's turn captured
// now: what the adapter hands the core, retried under its event.
func ownerLab(t *testing.T) (string, registry.Session, adapter.TurnEnd) {
	t.Helper()
	dir := liveSession(t, "api")
	readFrom(t, dir, otherRun(t, dir, "web"))
	self, err := registry.Lookup(dir, "api")
	if err != nil {
		t.Fatal(err)
	}
	end := adapter.TurnEnd{
		Event: "harness/end-1", Reason: adapter.Answered, Answer: "the suite is green",
		Boundary: boundaryNow(t, dir, self), Ended: boottime.Now(),
	}
	return dir, self, end
}

// An end of a run whose name passed to a later run is no receipt: the core
// takes nothing, and the adapter is told the run is gone, so it drops the end
// rather than send it again (docs/rules/effects.md, E7).
func TestAnEndOfARunWhoseNamePassedOnIsGoneAndTakesNothing(t *testing.T) {
	dir, self, end := ownerLab(t)
	otherRun(t, dir, "api")
	if _, err := ReportCompletion(context.Background(), dir, self, end); !errors.Is(err, adapter.ErrRunGone) {
		t.Fatalf("ReportCompletion = %v, want ErrRunGone", err)
	}
	if _, err := ConfirmCompletion(context.Background(), dir, self, end); !errors.Is(err, adapter.ErrRunGone) {
		t.Fatalf("ConfirmCompletion = %v, want ErrRunGone", err)
	}
	if got := reportsTo(t, dir, "web"); len(got) != 0 {
		t.Fatalf("web got %d reports from a run that no longer owns its name", len(got))
	}
	if _, err := mail.EndTurn(context.Background(), dir, self, mail.TurnEnd{Ended: boottime.Now()}, false, "", mail.TurnEndOptions{}); !errors.Is(err, mail.ErrNameLost) {
		t.Fatalf("the core answers %v, want ErrNameLost", err)
	}
}

// A record that cannot be read proves nothing either way: the end is not
// taken and not dropped, and the same end sent once the record reads again is
// published once.
func TestAnEndWhoseRecordCannotBeReadIsRetriedAndPublishedOnce(t *testing.T) {
	dir, self, end := ownerLab(t)
	path := state.SessionPath(dir, "api")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = ReportCompletion(context.Background(), dir, self, end)
	if err == nil || errors.Is(err, adapter.ErrRunGone) {
		t.Fatalf("an unreadable record answers %v, want an error to retry on", err)
	}
	if got := reportsTo(t, dir, "web"); len(got) != 0 {
		t.Fatalf("web got %d reports while the record could not be read", len(got))
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	for attempt := range 2 {
		if _, err := ReportCompletion(context.Background(), dir, self, end); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	if got := reportsTo(t, dir, "web"); len(got) != 1 {
		t.Fatalf("web got %d reports, want the one end once", len(got))
	}
}

// An end taken whose receipt was lost is answered from the journal while the
// run owns its name, and as gone once it does not.
func TestAnEndTakenWhoseReceiptWasLostIsAnsweredFromTheJournal(t *testing.T) {
	dir, self, end := ownerLab(t)
	if _, err := ReportCompletion(context.Background(), dir, self, end); err != nil {
		t.Fatal(err)
	}
	if _, err := ReportCompletion(context.Background(), dir, self, end); err != nil {
		t.Fatalf("the retry of an end taken answers %v", err)
	}
	if got := reportsTo(t, dir, "web"); len(got) != 1 {
		t.Fatalf("web got %d reports, want one", len(got))
	}
	otherRun(t, dir, "api")
	if _, err := ReportCompletion(context.Background(), dir, self, end); !errors.Is(err, adapter.ErrRunGone) {
		t.Fatalf("a retry after the name passed on answers %v, want ErrRunGone", err)
	}
}
