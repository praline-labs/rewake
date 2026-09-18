package codex_test

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/cli"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/harness/codex"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func publicationSessions(t *testing.T) (string, registry.Session, registry.Session) {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, root)
	t.Setenv(state.RoomEnv, state.DefaultRoom)
	dir, err := state.Dir()
	if err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	self := registry.Session{Name: "api", Harness: "codex", ServicePID: os.Getpid(), ServiceStart: start, CWD: dir, StartedAt: time.Now()}
	peer := self
	peer.Name = "web"
	for _, s := range []registry.Session{self, peer} {
		if err := registry.Publish(dir, s); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(state.SessionEnv, self.Name)
	t.Setenv(state.EpochEnv, self.Epoch())
	return dir, self, peer
}

func readPublicationWork(t *testing.T, dir string, self, peer registry.Session) string {
	t.Helper()
	m := inbox.Message{ID: inbox.NewID(), From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Task, Text: "work", CreatedAt: time.Now()}
	if err := state.EnsureSubdir(state.UnreadPath(dir, self.Name)); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.WriteAtomic(filepath.Join(state.UnreadPath(dir, self.Name), m.ID+".json"), raw); err != nil {
		t.Fatal(err)
	}
	if code := cli.Run([]string{"inbox"}, io.Discard, io.Discard); code != 0 {
		t.Fatalf("inbox exit=%d", code)
	}
	return m.ID
}

// INT-1: same queue, failed journal persistence, normal inbox reader and durable
// publication as the review reproduction. Only API plumbing changed.
func TestReviewDelayedPublicationDoesNotSettleLaterWork(t *testing.T) {
	dir, self, peer := publicationSessions(t)
	clock, err := inbox.OpenReadClock(context.Background(), dir, self.Name, self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	defer clock.Close()
	first := readPublicationWork(t, dir, self, peer)
	journalDir := filepath.Join(t.TempDir(), "unavailable")
	published := make(chan struct{}, 4)
	queue, closePublisher := codex.PublisherForTest(filepath.Join(journalDir, "run.sock"), harness.CompletionHandler{Capture: clock.Snapshot, Publish: func(ctx context.Context, result harness.Completion) error {
		err := cli.ReportCompletion(ctx, dir, self, result)
		if err == nil {
			published <- struct{}{}
		}
		return err
	}})
	defer closePublisher()
	queue(harness.Completion{ID: "A/first-turn", Thread: "A", Kind: inbox.Finished, Text: "first turn result"})
	select {
	case <-published:
		t.Fatal("publication bypassed unavailable journal")
	case <-time.After(300 * time.Millisecond):
	}
	second := readPublicationWork(t, dir, self, peer)
	if err := os.Mkdir(journalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("publication did not recover")
	}
	waits := inbox.Waiters(dir, self.Name, self.Epoch())
	if len(waits) != 1 || !slices.Equal(waits[0].Messages, []string{second}) {
		t.Fatalf("delayed completion consumed later work: first=%s second=%s remaining=%v", first, second, waits)
	}
	// The next result still owns the second message; retries cannot absorb a third.
	queue(harness.Completion{ID: "A/second-turn", Thread: "A", Kind: inbox.Finished, Text: "second result"})
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("second result did not publish")
	}
	if waits := inbox.Waiters(dir, self.Name, self.Epoch()); len(waits) != 0 {
		t.Fatal(waits)
	}
}
