package inbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

func TestReadBoundaryExcludesLaterReadsAndIncludesSameTurnSteering(t *testing.T) {
	dir := stateDir(t)
	clock, err := OpenReadClock(context.Background(), dir, "api", "1.1")
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	read := func(id string) {
		t.Helper()
		if err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
			return markScopedAwaiting(dir, "api", "1.1", Message{ID: id, From: "peer", FromEpoch: "2.2"})
		}); err != nil {
			t.Fatal(err)
		}
	}
	read("first")
	read("steered")
	boundary := clock.Snapshot()
	read("next-turn")
	readAt := Waiters(dir, "api", "1.1")[0].ReadAt
	selected, err := ScopedWaiters(dir, "api", "1.1", boundary)
	if err != nil || len(selected) != 1 || !slices.Equal(selected[0].Messages, []string{"first", "steered"}) {
		t.Fatalf("scope=%v %v", selected, err)
	}
	// Each message keeps its own read time through the scope and the clear:
	// the resume window counts from it.
	if len(readAt) != 3 || !slices.Equal(selected[0].ReadAt, readAt[:2]) {
		t.Fatalf("read times %v, scoped %v", readAt, selected[0].ReadAt)
	}
	ClearAwaiting(dir, "api", "1.1", selected[0])
	remaining := Waiters(dir, "api", "1.1")
	if len(remaining) != 1 || !slices.Equal(remaining[0].Messages, []string{"next-turn"}) || !slices.Equal(remaining[0].ReadAt, readAt[2:]) {
		t.Fatal(remaining)
	}
	if _, err := ScopedWaiters(dir, "api", "other", boundary); err == nil {
		t.Fatal("foreign epoch accepted")
	}
	if _, err := ScopedWaiters(dir, "other", "1.1", boundary); err == nil {
		t.Fatal("foreign mailbox accepted")
	}
}

func TestReadBoundaryCaptureNeverWaitsForMailbox(t *testing.T) {
	dir := stateDir(t)
	clock, err := OpenReadClock(context.Background(), dir, "api", "1.1")
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		_ = state.WithMailboxLock(context.Background(), dir, "api", func() error { close(held); <-release; return nil })
	}()
	<-held
	captured := make(chan *ReadBoundary, 1)
	go func() { captured <- clock.Snapshot() }()
	select {
	case <-captured:
	case <-time.After(100 * time.Millisecond):
		close(release)
		<-done
		t.Fatal("native reader waited for mailbox")
	}
	close(release)
	<-done
}

func TestReadBoundaryHighWatermarkPreventsReuseAfterCounterLoss(t *testing.T) {
	dir := stateDir(t)
	clock, err := OpenReadClock(context.Background(), dir, "api", "1.1")
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	if err := markScopedAwaiting(dir, "api", "1.1", Message{ID: "old", From: "peer", FromEpoch: "2.2"}); err != nil {
		t.Fatal(err)
	}
	boundary := clock.Snapshot()
	for _, waiter := range Waiters(dir, "api", "1.1") {
		ClearAwaiting(dir, "api", "1.1", waiter)
	}
	atomic.StoreUint64(clock.word(), 0)
	if err := markScopedAwaiting(dir, "api", "1.1", Message{ID: "later", From: "peer", FromEpoch: "2.2"}); err != nil {
		t.Fatal(err)
	}
	selected, err := ScopedWaiters(dir, "api", "1.1", boundary)
	if err != nil || len(selected) != 0 {
		t.Fatalf("old boundary absorbed a new read: %v %v", selected, err)
	}
	if clock.Snapshot().Through <= boundary.Through {
		t.Fatal("counter reused durable sequence")
	}
	path, _ := awaitingPath(dir, "api", "1.1")
	if _, err := readHigh(filepath.Join(path, ".read-high")); err != nil {
		t.Fatal(err)
	}
}

func TestReadClockProcessWriter(t *testing.T) {
	dir := os.Getenv("RW_READ_CLOCK_TEST_DIR")
	if dir == "" {
		return
	}
	err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
		return markScopedAwaiting(dir, "api", "1.1", Message{ID: "from-process", From: "peer", FromEpoch: "2.2"})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestReadBoundaryIsSharedWithAnotherReaderProcess(t *testing.T) {
	dir := stateDir(t)
	clock, err := OpenReadClock(context.Background(), dir, "api", "1.1")
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	before := clock.Snapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestReadClockProcessWriter$")
	command.Env = append(os.Environ(), "RW_READ_CLOCK_TEST_DIR="+dir)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("reader process: %v %s", err, output)
	}
	after := clock.Snapshot()
	if before.Through != 0 || after.Through == 0 {
		t.Fatalf("mapping did not observe another process: before=%v after=%v", before, after)
	}
	selected, err := ScopedWaiters(dir, "api", "1.1", after)
	if err != nil || len(selected) != 1 || !slices.Equal(selected[0].Messages, []string{"from-process"}) {
		t.Fatalf("reader scope: %v %v", selected, err)
	}
}
