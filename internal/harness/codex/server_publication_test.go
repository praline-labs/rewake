package codex

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
)

func TestBlockedPublicationDoesNotBlockNativeReader(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s, ui, _ := runtimeFixture(t, func(harness.Completion) error { once.Do(func() { close(started) }); <-release; return nil })
	defer close(release)
	if result := s.Deliver(context.Background(), inbox.Message{ID: "task"}); result.State != inbox.Delivered {
		t.Fatal(result)
	}
	emitFixture(t, ui, "turn/started", map[string]any{"threadId": fixtureRoot, "turn": map[string]string{"id": "active-turn"}})
	emitFixture(t, ui, "turn/completed", map[string]any{"threadId": fixtureRoot, "turn": map[string]string{"id": "active-turn", "status": "failed"}})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("publication did not start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := ui.call(ctx, "thread/list", map[string]any{}, nil); err != nil {
		t.Fatalf("mailbox publisher blocked native traffic: %v", err)
	}
	raw, err := os.ReadFile(s.path + ".outcomes.json")
	if err != nil {
		t.Fatal(err)
	}
	var queued []harness.Completion
	if json.Unmarshal(raw, &queued) != nil || len(queued) != 1 || queued[0].ID != fixtureRoot+"/active-turn" {
		t.Fatal("completion not journaled before blocked publication")
	}
}

func TestPublicationRetryAndJournalRecoveryKeepOriginalIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.sock")
	s := newServer(path, nil, nil, "")
	s.note = func(string) {}
	original := harness.Completion{ID: "A/turn", Thread: "A", Kind: inbox.Finished, Text: "fixed result"}
	s.queueCompletion(original)
	if err := s.persistOutcomes(); err != nil {
		t.Fatal(err)
	}
	recovered := newServer(path, nil, nil, "")
	recovered.note = func(string) {}
	if err := recovered.restoreOutcomes(); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	published := make(chan harness.Completion, 1)
	recovered.emit = func(_ context.Context, result harness.Completion) error {
		if result != original {
			t.Errorf("retry changed identity: %+v", result)
		}
		if attempts.Add(1) == 1 {
			return errors.New("mailbox busy")
		}
		published <- result
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go recovered.report(ctx)
	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("publication was not retried")
	}
	cancel()
	<-recovered.stopped
	raw, err := os.ReadFile(path + ".outcomes.json")
	if err != nil {
		t.Fatal(err)
	}
	var pending []harness.Completion
	if json.Unmarshal(raw, &pending) != nil || len(pending) != 0 || attempts.Load() != 2 {
		t.Fatal("successful durable handoff left a pending callback")
	}
}

func TestAdapterPreservesAnonymousGapScope(t *testing.T) {
	s, ui, outcomes := runtimeFixture(t)
	var ids []string
	for i := 0; i < 2; i++ {
		if i > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := ui.call(ctx, "thread/start", map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{"/work"}}, nil)
			cancel()
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, status := range []string{"active", "idle"} {
			emitFixture(t, ui, "thread/status/changed", map[string]any{"threadId": fixtureRoot, "status": map[string]string{"type": status}})
		}
		select {
		case result := <-outcomes:
			b := s.gateway.Binding()
			want := fixtureRoot + "/gap-1/" + strconv.FormatUint(b.Connection, 10) + "/" + strconv.FormatUint(b.Generation, 10)
			if result.ID != want {
				t.Fatalf("adapter discarded gap identity: got %s want %s", result.ID, want)
			}
			ids = append(ids, result.ID)
		case <-time.After(2 * time.Second):
			t.Fatal("gap lost")
		}
	}
	if ids[0] == ids[1] {
		t.Fatal("gap generations collide")
	}
}
