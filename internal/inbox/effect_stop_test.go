package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// An unknown only an effect met — the reading before it and the one after it
// both clean — is kept as the effect's: a call other than the barrier cannot
// meet it again, so it answers the stop on record until the barrier runs the
// effect again and it goes through.
func TestAnUnknownOnlyAnEffectMetStopsUntilTheBarrier(t *testing.T) {
	lab := newConversionLab(t)
	version := "v1"
	raw, err := json.Marshal(keptRecord{Epoch: earlierRun, Text: "held", Version: version})
	if err != nil {
		t.Fatal(err)
	}
	if err := state.EnsureSubdir(pendingDir(lab.dir, "api")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keptPath(lab.dir, "api"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteJournal(lab.dir, "api", "end", TurnJournal{Epoch: earlierRun, Op: "end", Kept: &version}); err != nil {
		t.Fatal(err)
	}
	kept := keptPath(lab.dir, "api")
	t.Cleanup(func() { afterReading, afterEffect = func() {}, func() {} })
	afterReading = func() { _ = os.Chmod(kept, 0) }
	afterEffect = func() { _ = os.Chmod(kept, 0o600) }
	err = withLock(lab.dir, "api", func() error { return Reconcile(context.Background(), lab.dir, "api") })
	var unknown *UnknownRecordError
	if !errors.As(err, &unknown) {
		t.Fatalf("the effect did not meet the unknown: %v", err)
	}
	afterReading, afterEffect = func() {}, func() {}
	var stopped *RecordedStopError
	if err := MailboxStopped(lab.dir, "api"); !errors.As(err, &stopped) {
		t.Fatalf("a call other than the barrier forgot the stop an effect met: %v", err)
	}
	if err := withLock(lab.dir, "api", func() error { return Reconcile(context.Background(), lab.dir, "api") }); err != nil {
		t.Fatalf("the barrier kept a stop whose effect now goes through: %v", err)
	}
	if err := MailboxStopped(lab.dir, "api"); err != nil {
		t.Fatalf("the stop outlived the barrier: %v", err)
	}
}

// stopWrites fails every write of the stop record and counts them.
type stopWrites struct {
	fileAccess
	path   string
	failed int
}

func (s *stopWrites) WriteFile(path string, raw []byte) error {
	if path == s.path {
		s.failed++
		return &fs.PathError{Op: "write", Path: path, Err: syscall.EIO}
	}
	return s.fileAccess.WriteFile(path, raw)
}

// A stop the barrier could write neither when the effect met its unknown nor
// once the plan after it had looked is answered with both failures beside the
// unknown: the caller learns why no stop is on record, and what to look at.
// The effect's cause is named even when the plan after it found another: the
// answer is the only place left that holds it.
func TestAStopThatCouldNotBeRecordedNamesEveryFailedWrite(t *testing.T) {
	for _, other := range []bool{false, true} {
		t.Run(fmt.Sprintf("another cause after the effect: %v", other), func(t *testing.T) {
			lab := newConversionLab(t)
			version := "v1"
			raw, err := json.Marshal(keptRecord{Epoch: earlierRun, Text: "held", Version: version})
			if err != nil {
				t.Fatal(err)
			}
			kept := keptPath(lab.dir, "api")
			writeRaw(t, kept, string(raw))
			if err := WriteJournal(lab.dir, "api", "end", TurnJournal{Epoch: earlierRun, Op: "end", Kept: &version}); err != nil {
				t.Fatal(err)
			}
			rel, err := filepath.Rel(lab.dir, kept)
			if err != nil {
				t.Fatal(err)
			}
			writes := &stopWrites{fileAccess: osAccess{}, path: stopPath(lab.dir, "api")}
			effects := newProbe(lab.dir, writes, &seamFault{at: seamRead{op: "read", rel: rel}, kind: "no access"})
			testAccess.Store(lab.dir, passAccess{live: effects, plan: osAccess{}})
			t.Cleanup(func() { testAccess.Delete(lab.dir) })
			interim := interimPath(lab.dir, "api")
			if other {
				afterEffect = func() { writeRaw(t, interim, "{") }
				t.Cleanup(func() { afterEffect = func() {} })
			}
			err = lab.reconcile(t)
			var unknown *UnknownRecordError
			if !errors.As(err, &unknown) || writes.failed != 2 || strings.Count(err.Error(), "could not record the stop") != 2 {
				t.Fatalf("after %d failed writes of the stop the barrier answers %v", writes.failed, err)
			}
			if !strings.Contains(err.Error(), kept) || other && !strings.Contains(err.Error(), interim) {
				t.Fatalf("with no stop on record the barrier answers %v, which does not name each cause", err)
			}
		})
	}
}
