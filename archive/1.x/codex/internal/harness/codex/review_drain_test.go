package codex

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
)

func TestReviewShutdownDrainHonorsDeadlineWhilePublicationSucceeds(t *testing.T) {
	s := newServer(filepath.Join(t.TempDir(), "run.sock"), nil, nil, "")
	s.note = func(message string) { t.Log(message) }
	for i := range 8 {
		s.queueCompletion(harness.Completion{ID: fmt.Sprintf("A/turn-%d", i), Thread: "A", Kind: inbox.Finished, Text: "result"})
	}
	s.emit = func(context.Context, harness.Completion) error {
		time.Sleep(900 * time.Millisecond)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	s.report(ctx)
	elapsed := time.Since(started)
	t.Logf("drain elapsed=%v remaining=%d", elapsed, len(s.outcomes))
	if elapsed > 6*time.Second {
		t.Fatal("successful publication bypassed the five-second shutdown drain deadline")
	}
}

func TestDrainDeadlineIncludesInFlightPublishAndRetainsJournal(t *testing.T) {
	s := newServer(filepath.Join(t.TempDir(), "run.sock"), nil, nil, "")
	s.note = func(string) {}
	s.queueCompletion(harness.Completion{ID: "A/blocked", Thread: "A", Kind: inbox.Finished, Text: "result"})
	entered, release := make(chan struct{}), make(chan struct{})
	s.emit = func(context.Context, harness.Completion) error { close(entered); <-release; return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.report(ctx)
	<-entered
	started := time.Now()
	cancel()
	select {
	case <-s.stopped:
	case <-time.After(6 * time.Second):
		close(release)
		t.Fatal("in-flight publish bypassed drain deadline")
	}
	close(release)
	if time.Since(started) > 6*time.Second || len(s.outcomes) != 1 {
		t.Fatal("pending in-flight result was discarded")
	}
	restored := newServer(s.path, nil, nil, "")
	if err := restored.restoreOutcomes(); err != nil || len(restored.outcomes) != 1 {
		t.Fatalf("pending journal lost: %v", err)
	}
}
