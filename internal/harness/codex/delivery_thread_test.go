package codex

import (
	"context"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

func TestDeliveryUsesTheRecordedThread(t *testing.T) {
	home := t.TempDir()
	pid := threadFixture(t, home, map[string]time.Time{"current": time.Now()})
	previous := queue
	t.Cleanup(func() { queue = previous })
	queued := ""
	queue = func(_ context.Context, _, thread, _ string) (string, error) { queued = thread; return "", nil }
	result := New().Deliver(context.Background(), registry.Session{HarnessPID: pid, CodexHome: home}, inbox.Message{ID: "message", DeliveryThread: "recorded"})
	if queued != "recorded" || result.State != inbox.Failed {
		t.Fatalf("target changed silently: queued=%q result=%+v", queued, result)
	}
}
