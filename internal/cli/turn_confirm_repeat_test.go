package cli

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// The repeated end: an adapter whose answer to a hold was lost confirms the
// same end again. It is answered from the record — the same reason, nothing
// published, nothing kept again — and never taken for its own continuation
// (docs/turn-end-recovery.md#a-held-end-confirmed-again).

// heldEnd is the end the repeated cases hold, with the boundary it was
// captured with, which a retry carries unchanged.
func (lab interimLab) heldEnd(t *testing.T) harness.Completion {
	t.Helper()
	end := lab.end("turn-2", 1, inbox.Finished, "the held answer")
	end.Boundary = boundaryNow(t, lab.dir, lab.self)
	return end
}

// again confirms an end as it is, its boundary included.
func (lab interimLab) again(t *testing.T, end harness.Completion) string {
	t.Helper()
	reason, err := ConfirmCompletion(context.Background(), lab.dir, lab.self, end)
	if err != nil {
		t.Fatal(err)
	}
	return reason
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

// setClockWord puts the word back where a crash before its commit leaves it.
func (lab interimLab) setClockWord(t *testing.T, word uint64) {
	t.Helper()
	raw := make([]byte, 8)
	binary.NativeEndian.PutUint64(raw, word)
	if err := os.WriteFile(lab.clockPath(), raw, 0o600); err != nil {
		t.Fatal(err)
	}
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
	if reason := lab.confirm(t, lab.end("turn-2b", 2, inbox.Finished, "the continuation")); reason != "" {
		t.Fatalf("the continuation was held: %s", reason)
	}
	got := lab.reportsOf(t, inbox.Finished)
	if len(got) != 1 || got[0] != "the held answer\n\nthe continuation" || lab.kept() {
		t.Fatalf("after the continuation web holds %q, kept %v", got, lab.kept())
	}
}

type crash struct{}

// crashAt runs fn and ends it, as a killed process ends, at the first file
// operation of the fault seam that match names.
func crashAt(t *testing.T, match func(op, path string) bool, fn func()) {
	t.Helper()
	crashed := false
	state.Fault = func(op, path string) error {
		if !crashed && match(op, path) {
			crashed = true
			panic(crash{})
		}
		return nil
	}
	defer func() {
		state.Fault = nil
		if r := recover(); r != nil {
			if _, ok := r.(crash); !ok {
				panic(r)
			}
		}
		if !crashed {
			t.Fatal("the crash point was never reached")
		}
	}()
	fn()
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
}

func TestTwoEqualConfirmationsAtOnceHoldOnce(t *testing.T) {
	lab := newInterimLab(t)
	end := lab.heldEnd(t)
	var wg sync.WaitGroup
	reasons := make([]string, 2)
	errs := make([]error, 2)
	for i := range reasons {
		wg.Go(func() { reasons[i], errs[i] = ConfirmCompletion(context.Background(), lab.dir, lab.self, end) })
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

// A crash at each write around the hold, then the same end again: held once,
// never published without its continuation.
func TestAHoldCutByACrashIsHeldOnceWhenConfirmedAgain(t *testing.T) {
	for _, c := range []struct{ name, suffix string }{
		{"the clock's reservation", ".read-high"},
		{"the kept answer", filepath.Join("pending", "kept.json")},
	} {
		t.Run(c.name, func(t *testing.T) {
			lab := newInterimLab(t)
			end := lab.heldEnd(t)
			crashAt(t, func(op, path string) bool { return op == state.OpWrite && strings.HasSuffix(path, c.suffix) }, func() { lab.again(t, end) })
			if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 1 {
				t.Fatalf("the crash published %v", got)
			}
			if reason := lab.again(t, end); reason == "" {
				t.Fatal("the end confirmed again after the crash was not held")
			}
			if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 1 || !lab.kept() {
				t.Fatalf("the retry published %v, kept %v", got, lab.kept())
			}
			lab.continued(t)
		})
	}
	// The crash between the kept answer and the clock's commit: the word stays
	// below the hold's position. Confirmed again, the end raises it, so the
	// continuation with no read between takes the kept answer.
	t.Run("the clock's commit", func(t *testing.T) {
		lab := newInterimLab(t)
		end := lab.heldEnd(t)
		before := lab.clockWord(t)
		reason := lab.again(t, end)
		held := lab.clockWord(t)
		if reason == "" || held <= before {
			t.Fatalf("the hold: %q, the clock at %d from %d", reason, held, before)
		}
		lab.setClockWord(t, before)
		if again := lab.again(t, end); again != reason {
			t.Fatalf("confirmed again after the crash: %q", again)
		}
		if got := lab.clockWord(t); got != held {
			t.Fatalf("the clock is at %d, want the hold's position %d", got, held)
		}
		if got := kinds(reportsTo(t, lab.dir, "web")); len(got) != 1 {
			t.Fatalf("the retry published %v", got)
		}
		lab.continued(t)
	})
}

// The held end confirmed again after its continuation published is answered as
// published, whatever form the continuation's journal is in: nothing is
// published again and nothing held again. A continuation that marked pending
// leaves the task owed and the run interim, so only the journal's record of
// the held end tells it from an end that could be held. The sweep that keeps
// the journal while the run lives is inbox's
// TestAJournalThatTookAHeldEndNamesItInEveryForm.
func TestAHeldEndConfirmedAfterItsContinuationIsPublished(t *testing.T) {
	journal := filepath.Join(string(filepath.Separator)+"journal", "")
	for _, c := range []struct {
		name  string
		crash func(op, path string) bool
	}{
		{"its journal open", func(op, path string) bool {
			return op == state.OpWrite && strings.Contains(path, filepath.Join("inbox", "web")+string(filepath.Separator))
		}},
		{"done in place", func(op, path string) bool {
			return op == state.OpRename && strings.Contains(path, journal)
		}},
		{"renamed done", nil},
	} {
		for _, marked := range []bool{false, true} {
			name := c.name
			if marked {
				name += ", marked pending"
			}
			t.Run(name, func(t *testing.T) {
				lab := newInterimLab(t)
				end := lab.heldEnd(t)
				lab.again(t, end)
				if marked {
					if err := markPending(lab.dir, "api", lab.self.Epoch(), "the second half is running", markAt+2); err != nil {
						t.Fatal(err)
					}
				}
				continuation := lab.end("turn-2b", 3, inbox.Finished, "the continuation")
				continuation.Boundary = boundaryNow(t, lab.dir, lab.self)
				if c.crash != nil {
					crashAt(t, c.crash, func() { lab.again(t, continuation) })
				} else {
					lab.again(t, continuation)
				}
				// The barrier of the next end completes a journal a crash left.
				if _, err := ConfirmCompletion(context.Background(), lab.dir, lab.self, continuation); err != nil {
					t.Fatal(err)
				}
				published := reportsTo(t, lab.dir, "web")
				if reason := lab.again(t, end); reason != "" {
					t.Fatalf("the held end was held again: %s", reason)
				}
				if got := reportsTo(t, lab.dir, "web"); len(got) != len(published) || lab.kept() {
					t.Fatalf("the held end published again: web holds %v, was %v, kept %v", kinds(got), kinds(published), lab.kept())
				}
				want, kind := "the held answer\n\nthe continuation", inbox.Finished
				if marked {
					want, kind = "the second half is running\n\n"+want, inbox.Interim
				}
				if got := lab.reportsOf(t, kind); !slices.Contains(got, want) || len(published) != 2 {
					t.Fatalf("web holds %q of %d reports, want %q once", got, len(published), want)
				}
			})
		}
	}
}
