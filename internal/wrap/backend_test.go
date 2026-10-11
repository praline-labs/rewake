package wrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
)

// backendFixture is a run's process service that also wakes, reports turns
// and follows its conversation, as the fixture's backend does.
type backendFixture struct {
	marker  string
	die     bool
	done    chan struct{}
	closed  bool
	handler adapter.TurnHandler
}

func (b *backendFixture) ReportTurns(handler adapter.TurnHandler) { b.handler = handler }

func (b *backendFixture) Start(context.Context, func(string)) error {
	if err := os.WriteFile(b.marker, nil, 0o600); err != nil {
		return err
	}
	if err := b.handler.Publish(context.Background(), adapter.TurnEnd{Event: "event", Reason: adapter.Answered, Answer: "result"}); err != nil {
		return err
	}
	if b.die {
		go func() { time.Sleep(50 * time.Millisecond); close(b.done) }()
	}
	return nil
}
func (b *backendFixture) ProcessID() int          { return 0 }
func (b *backendFixture) Done() <-chan struct{}   { return b.done }
func (b *backendFixture) Close()                  { b.closed = true }
func (b *backendFixture) Thread() (string, error) { return "root", nil }
func (b *backendFixture) Deliver(context.Context, inbox.Message) inbox.Result {
	return inbox.Result{State: inbox.Delivered}
}

type backendHarness struct {
	fakeHarness
	backend *backendFixture
}

func (h *backendHarness) Launch(r adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(r)
	plan.Service, plan.Wake, plan.TurnBoundary, plan.Thread = h.backend, h.backend, h.backend, h.backend
	if h.backend.die {
		plan.Command = "/bin/sleep"
		plan.Args = []string{"2"}
	}
	return plan, err
}

func TestBackendStartsBeforeTUIAndSharesItsLifetime(t *testing.T) {
	for _, die := range []bool{false, true} {
		t.Run(fmt.Sprint(die), func(t *testing.T) {
			dir := stateDir(t)
			marker := filepath.Join(t.TempDir(), "started")
			backend := &backendFixture{marker: marker, die: die, done: make(chan struct{})}
			fake := &backendHarness{fakeHarness: fakeHarness{script: fmt.Sprintf("test -f %q", marker)}, backend: backend}
			observed := false
			started := time.Now()
			code, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api", OnTurn: func(_ context.Context, self registry.Session, result adapter.TurnEnd) error {
				observed = self.Name == "api-fake" && result.Answer == "result"
				return nil
			}})
			if err != nil || !observed || !backend.closed {
				t.Fatalf("lifecycle: code=%d err=%v observed=%v closed=%v", code, err, observed, backend.closed)
			}
			if die && time.Since(started) > time.Second {
				t.Fatal("server death left the TUI running")
			}
			if !die && code != 0 {
				t.Fatal("TUI started before backend readiness")
			}
		})
	}
}
