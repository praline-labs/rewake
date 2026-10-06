package inbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// keptClock is a run of api with its read clock open, as its wrapper holds
// it, and a hook on the writes of the kept answer.
type keptClock struct {
	dir, epoch string
	clock      *ReadClock
}

func newKeptClock(t *testing.T) keptClock {
	t.Helper()
	dir := stateDir(t)
	clock, err := OpenReadClock(context.Background(), dir, "api", "1.1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(clock.Close)
	return keptClock{dir: dir, epoch: "1.1", clock: clock}
}

// atKeptWrite runs fn as the kept answer is about to be written, and fails
// that write when fn answers an error.
func (k keptClock) atKeptWrite(t *testing.T, fn func() error) {
	t.Helper()
	kept := keptPath(k.dir, "api")
	state.Fault = func(op, path string) error {
		if op == state.OpWrite && path == kept {
			return fn()
		}
		return nil
	}
	t.Cleanup(func() { state.Fault = nil })
}

func (k keptClock) high(t *testing.T) uint64 {
	t.Helper()
	path, _ := awaitingPath(k.dir, "api", k.epoch)
	raw, err := readHigh(filepath.Join(path, ".read-high"))
	if err != nil {
		t.Fatal(err)
	}
	high, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return high
}

func (k keptClock) keep(text string) error {
	return withLock(k.dir, "api", func() error { return KeepAnswer(k.dir, "api", k.epoch, text) })
}

func (k keptClock) read(id string) error {
	return withLock(k.dir, "api", func() error {
		return markScopedAwaiting(k.dir, "api", k.epoch, Message{ID: id, From: "web", FromEpoch: "2.2"}, 0)
	})
}

// A hold takes its place on the read clock as a read does: the position is
// reserved in the high-water record before the kept answer is written, and
// the word a boundary is captured from is committed only after it. A
// boundary captured while the hold writes does not cover the hold; one
// captured after does.
func TestAHoldReservesItsPositionBeforeItCommits(t *testing.T) {
	k := newKeptClock(t)
	if err := k.read("before"); err != nil {
		t.Fatal(err)
	}
	var reserved uint64
	var during *ReadBoundary
	k.atKeptWrite(t, func() error {
		reserved, during = k.high(t), k.clock.Snapshot()
		return nil
	})
	if err := k.keep("held"); err != nil {
		t.Fatal(err)
	}
	state.Fault = nil
	record, ok, err := readKept(k.dir, "api", k.epoch)
	if err != nil || !ok || record.Seq != reserved || during.Through >= reserved {
		t.Fatalf("kept at %d, reserved %d, the word %d while it was written: %v", record.Seq, reserved, during.Through, err)
	}
	if _, _, ok, err := KeptAnswerThrough(k.dir, "api", k.epoch, &during.Through); ok || err != nil {
		t.Fatalf("a boundary captured while the hold wrote covers it: %v", err)
	}
	after := k.clock.Snapshot()
	if _, _, ok, err := KeptAnswerThrough(k.dir, "api", k.epoch, &after.Through); !ok || err != nil {
		t.Fatalf("a boundary captured after the hold does not cover it: %v", err)
	}
}

// A hold whose write fails leaves a gap on the clock, not a position to issue
// again: the next read and the next hold each take a position of their own,
// above it, and no two are the same.
func TestAFailedHoldLeavesAGapAndNoPositionIsIssuedTwice(t *testing.T) {
	k := newKeptClock(t)
	var failed uint64
	k.atKeptWrite(t, func() error {
		failed = k.high(t)
		return fmt.Errorf("write failed: %w", syscall.EIO)
	})
	if err := k.keep("lost"); !errors.Is(err, syscall.EIO) {
		t.Fatalf("a failed hold answers %v", err)
	}
	if k.clock.Snapshot().Through >= failed {
		t.Fatalf("a failed hold committed its position %d", failed)
	}
	state.Fault = nil
	seen := map[uint64]bool{failed: true}
	for i, step := range []string{"read", "hold", "read", "hold"} {
		var err error
		if step == "read" {
			err = k.read(NewID())
		} else {
			err = k.keep("held " + strconv.Itoa(i))
		}
		if err != nil {
			t.Fatal(err)
		}
		position := k.clock.Snapshot().Through
		if seen[position] || position <= failed {
			t.Fatalf("%s %d took position %d, issued already or below the gap at %d", step, i, position, failed)
		}
		seen[position] = true
	}
	record, _, err := readKept(k.dir, "api", k.epoch)
	if err != nil || record.Seq != k.clock.Snapshot().Through {
		t.Fatalf("the last hold is kept at %d, the clock at %d: %v", record.Seq, k.clock.Snapshot().Through, err)
	}
}
