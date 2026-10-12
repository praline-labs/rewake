//go:build rewakefault

package delivery

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// TestReportFailureRecovery's recovery when the removal of the report's
// waiting copy fails: the report stays readable, and once the removal works
// its durable failed status keeps it from being announced again.
func TestReportFailureRecoveryAfterAFailedRemoval(t *testing.T) {
	s, m, calls := reportFailureFixture(t)
	attempts, unblock := mail.FailWaitingRemoval(filepath.Join(state.InboxPath(s.Dir, s.Name), m.ID+".json"))
	t.Cleanup(unblock)
	s.drain(context.Background())
	if unread, err := mail.PeekUnread(s.Dir, s.Name, s.Epoch); err != nil || len(unread) != 1 {
		t.Fatalf("fault lost report: %v %v", unread, err)
	}
	if attempts() == 0 {
		t.Fatal("the removal of the waiting copy was never tried")
	}
	unblock()
	// Recreate volatile state: durable failed status must prevent another RPC.
	s.attempts = map[string]time.Time{}
	s.outcomes = map[string]mail.Result{}
	s.drain(context.Background())
	requireReportState(t, s, m, mail.Failed, true)
	if *calls != 1 {
		t.Fatalf("failure retried %d times", *calls)
	}
	if queued, _ := mail.List(s.Dir, s.Name); len(queued) != 0 {
		t.Fatal("report still queued")
	}
}
