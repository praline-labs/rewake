package cli

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The repeated end: an adapter whose answer to a hold was lost confirms the
// same end again. It is answered from the record — the same reason, nothing
// published, nothing kept again — and never taken for its own continuation
// (docs/delivery-turn-end.md#a-held-end-confirmed-again).

// heldEnd is the end the repeated cases hold, with the boundary it was
// captured with, which a retry carries unchanged.
func (lab interimLab) heldEnd(t *testing.T) adapter.TurnEnd {
	t.Helper()
	end := lab.end("turn-2", 1, mail.Finished, "the held answer")
	end.Boundary = boundaryNow(t, lab.dir, lab.self)
	return end
}

// again confirms an end as it is, its boundary included.
func (lab interimLab) again(t *testing.T, end adapter.TurnEnd) string {
	t.Helper()
	outcome, err := ConfirmCompletion(context.Background(), lab.dir, lab.self, end)
	if err != nil {
		t.Fatal(err)
	}
	return outcome.Held
}

func (lab interimLab) clockPath() string {
	return filepath.Join(state.AwaitingPath(lab.dir, "api"), lab.self.Epoch(), ".read-clock")
}

// clockWord reads the run's committed read word from its file.
func (lab interimLab) clockWord(t *testing.T) uint64 {
	t.Helper()
	raw, err := os.ReadFile(lab.clockPath())
	if err != nil || len(raw) != 8 {
		t.Fatalf("the read clock: %d bytes, %v", len(raw), err)
	}
	return binary.NativeEndian.Uint64(raw)
}

func (lab interimLab) keptRaw(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(state.InboxPath(lab.dir, "api"), "pending", "kept.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// continued confirms the continuation of the held end, with no read between,
// and checks that its report carries the held answer exactly once.
func (lab interimLab) continued(t *testing.T) {
	t.Helper()
	if reason := lab.confirm(t, lab.end("turn-2b", 2, mail.Finished, "the continuation")); reason != "" {
		t.Fatalf("the continuation was held: %s", reason)
	}
	got := lab.reportsOf(t, mail.Finished)
	if len(got) != 1 || got[0] != "the held answer\n\nthe continuation" || lab.kept() {
		t.Fatalf("after the continuation web holds %q, kept %v", got, lab.kept())
	}
}

func TestTheSameEndConfirmedAgainGetsTheSameAnswer(t *testing.T) {
	lab := newInterimLab(t)
	end := lab.heldEnd(t)
	reason := lab.again(t, end)
	kept, word := lab.keptRaw(t), lab.clockWord(t)
	if reason == "" {
		t.Fatal("the end was not held")
	}
	if again := lab.again(t, end); again != reason {
		t.Fatalf("confirmed again, the end got %q, want %q", again, reason)
	}
	if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 1 || lab.keptRaw(t) != kept || lab.clockWord(t) != word {
		t.Fatalf("the repeat published %v, kept %v (was %v), moved the clock to %d from %d", got, lab.keptRaw(t), kept, lab.clockWord(t), word)
	}
	lab.continued(t)
	if reason := lab.again(t, end); reason != "" || len(lab.reportsOf(t, mail.Finished)) != 1 {
		t.Fatalf("the held end retried after publication: reason %q, reports %v", reason, lab.reportsOf(t, mail.Finished))
	}
}

func TestTwoEqualConfirmationsAtOnceHoldOnce(t *testing.T) {
	lab := newInterimLab(t)
	end := lab.heldEnd(t)
	var wg sync.WaitGroup
	reasons := make([]string, 2)
	errs := make([]error, 2)
	for i := range reasons {
		wg.Go(func() {
			var outcome mail.Outcome
			outcome, errs[i] = ConfirmCompletion(context.Background(), lab.dir, lab.self, end)
			reasons[i] = outcome.Held
		})
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || reasons[0] == "" || reasons[0] != reasons[1] {
		t.Fatalf("the two confirmations answered %q, %v", reasons, errs)
	}
	if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 1 || !lab.kept() {
		t.Fatalf("the two published %v, kept %v", got, lab.kept())
	}
	lab.continued(t)
}
