package mail

import (
	"context"
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The listing takes as settled exactly what the stop's one reader does
// (stopState): the occurrences a durable decision names. An occurrence about
// a decided report that the decision does not name — recorded after it — or
// any occurrence while the decision is not durable yet stops the mailbox, and
// the listing names it beside the decided subject, which it keeps; it offers
// no subject to decide again, since a decision is never replaced.
func TestDecisionListingNamesEveryOccurrenceTheDecisionDoesNotDispose(t *testing.T) {
	cases := map[string]struct {
		// after is what happens once R is decided; it answers the occurrence
		// still open.
		after   func(t *testing.T, s *matrixScene, decided openStop) openStop
		durable bool
	}{
		"an occurrence recorded after the decision": {func(t *testing.T, s *matrixScene, decided openStop) openStop {
			stop, err := live(s.dir).recordOccurrence("api", decided.record.stopCause)
			if err != nil {
				t.Fatal(err)
			}
			return stop
		}, true},
		"the decision not durable yet": {func(t *testing.T, s *matrixScene, decided openStop) openStop {
			if err := os.Remove(DecisionPath(s.dir, "api", s.report.ID) + durableSuffix); err != nil {
				t.Fatal(err)
			}
			return decided
		}, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, r := newContractScene(t)
			before := stopsOnRecord(t, s.dir, "api")
			if len(before) != 1 {
				t.Fatalf("setup: %v", before)
			}
			answer, err := s.decide(r, ChoiceArrived, false)
			if err != nil {
				t.Fatal(err)
			}
			if listing, err := ListDecisions(s.dir, "api"); err != nil || len(listing.Undecidable) != 0 || len(listing.Subjects) != 1 {
				t.Fatalf("the decision disposes its occurrence, yet the listing is %+v, %v", listing, err)
			}
			want := c.after(t, s, before[0])
			open := stopsOnRecord(t, s.dir, "api")
			if len(open) != 1 || open[0].id != want.id {
				t.Fatalf("setup: the stop's reader holds %v, want %s", open, want.id)
			}
			if err := state.WithMailboxLock(context.Background(), s.dir, "api", func() error { return MailboxStopped(s.dir, "api") }); err == nil {
				t.Fatal("setup: the mailbox is not stopped")
			}
			listing, err := ListDecisions(s.dir, "api")
			if err != nil {
				t.Fatal(err)
			}
			if len(listing.Subjects) != 1 || listing.Subjects[0].Decided == nil || listing.Subjects[0].Decided.Decision.ID != answer.Decision.ID || listing.Subjects[0].Durable != c.durable {
				t.Errorf("the decided subject: %+v", listing.Subjects)
			}
			if len(listing.Undecidable) != 1 || listing.Undecidable[0].Path != want.path(s.dir, "api") {
				t.Fatalf("the occurrence still open is not named: %+v", listing.Undecidable)
			}
		})
	}
}
