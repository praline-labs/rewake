package mail

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// O1: every read and write of the check failing falls to publishing, the
// behavior without the hold, and keeps nothing. The check's own access is the
// nth of its path: before the check, the barrier reads the interim record and
// the lookup of an end on record reads the kept answer, and a failure there
// stops the end instead, as an unreadable record does.
func TestEveryFailureOfTheConfirmationCheckFallsToPublishing(t *testing.T) {
	for _, c := range []struct {
		name, op, suffix string
		nth              int
	}{
		{"the interim record", state.OpRead, filepath.Join("pending", "interim.json"), 2},
		{"the kept answer", state.OpRead, filepath.Join("pending", "kept.json"), 2},
		{"the kept answer before keeping", state.OpRead, filepath.Join("pending", "kept.json"), 3},
		{"the sender's record", state.OpRead, filepath.Join("sessions", "web.json"), 1},
		{"the clock's reservation", state.OpWrite, ".read-high", 1},
		{"keeping the answer", state.OpWrite, filepath.Join("pending", "kept.json"), 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newInterimLab(t)
			clock, err := OpenReadClock(t.Context(), lab.dir, "api", lab.self.Epoch())
			if err != nil {
				t.Fatal(err)
			}
			end := TurnEnd{ID: "turn-2", Text: "done", Started: lab.start, Ended: markAt + 1, Boundary: clock.Snapshot()}
			clock.Close()
			seen, failed := 0, false
			state.Fault = func(op, path string) error {
				if op != c.op || !strings.HasSuffix(path, c.suffix) {
					return nil
				}
				if seen++; seen == c.nth {
					failed = true
					return os.ErrPermission
				}
				return nil
			}
			t.Cleanup(func() { state.Fault = nil })
			if reason, err := EndTurn(t.Context(), lab.dir, lab.self, end, true, "", TurnEndOptions{}); err != nil || reason != "" {
				t.Fatalf("the failed hold check: reason %q, error %v", reason, err)
			}
			state.Fault = nil
			if !failed {
				t.Fatalf("nothing asked to %s %s", c.op, c.suffix)
			}
			got := slices.DeleteFunc(turnReports(t, lab.dir), func(m Message) bool { return KindOf(m) != Finished })
			_, kept, err := KeptAnswer(lab.dir, "api", lab.self.Epoch())
			if err != nil || len(got) != 1 || got[0].Text != "done" || lab.owed() != 0 || kept {
				t.Fatalf("after the failure web holds %+v, %d owed, kept %v: %v", got, lab.owed(), kept, err)
			}
		})
	}
}
