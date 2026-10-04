package state

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
)

// The shell's evidence is dated when the state was reached, not when the
// command that reached it returns: a send may wait for delivery long after
// its write, and a failure interval that opened meanwhile must not take that
// write for a recovery (docs/mail-bridge-channel.md#the-shell-observation).
func TestTheReachKeepsTheTimeItHappened(t *testing.T) {
	ResetReach()
	t.Cleanup(ResetReach)
	dir := t.TempDir()
	before := boottime.Now()
	if err := WriteAtomic(filepath.Join(dir, "first"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	firstDone := boottime.Now()
	time.Sleep(5 * time.Millisecond)
	got := Reached()
	if !got.Wrote || got.WroteAt.Boot < before || got.WroteAt.Boot > firstDone || got.WroteAt.Wall.IsZero() {
		t.Fatalf("write between %d and %d dated %+v", before, firstDone, got.WroteAt)
	}

	// A later write is later evidence; a failure keeps its own, first time.
	failedFrom := boottime.Now()
	_ = noteReach(&os.PathError{Op: "open", Path: dir, Err: syscall.EROFS}, true)
	failedBy := boottime.Now()
	time.Sleep(5 * time.Millisecond)
	_ = noteReach(&os.PathError{Op: "open", Path: dir, Err: syscall.EACCES}, true)
	secondFrom := boottime.Now()
	if err := WriteAtomic(filepath.Join(dir, "second"), []byte("x")); err != nil {
		t.Fatal(err)
	}
	got = Reached()
	if got.WroteAt.Boot < secondFrom {
		t.Fatalf("the last write dated %d, before it began at %d", got.WroteAt.Boot, secondFrom)
	}
	if got.FailedAt.Boot < failedFrom || got.FailedAt.Boot > failedBy {
		t.Fatalf("the first failure, between %d and %d, dated %d", failedFrom, failedBy, got.FailedAt.Boot)
	}
	ResetReach()
	if got := Reached(); got.Wrote || got.Failed != nil || got.WroteAt.Boot != 0 || got.FailedAt.Boot != 0 {
		t.Fatalf("a reset kept %+v", got)
	}
}
