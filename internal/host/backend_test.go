package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// backendFixture is a run's service that also wakes, reports turns and
// follows its conversation, as the fixture's backend does.
type backendFixture struct {
	marker  string
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
	return nil
}
func (b *backendFixture) Close()                  { b.closed = true }
func (b *backendFixture) Thread() (string, error) { return "root", nil }
func (b *backendFixture) Deliver(context.Context, mail.Message) mail.Result {
	return mail.Result{State: mail.Delivered}
}

type backendHarness struct {
	fakeHarness
	backend *backendFixture
}

func (h *backendHarness) Launch(r adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	plan, err := h.fakeHarness.Launch(r)
	plan.Service, plan.Wake, plan.TurnBoundary, plan.Thread = h.backend, h.backend, h.backend, h.backend
	return plan, err
}

// The service starts before the harness, its turns reach the run's
// publication, and it is closed with the run.
func TestBackendStartsBeforeTheHarnessAndClosesWithIt(t *testing.T) {
	dir := stateDir(t)
	marker := filepath.Join(t.TempDir(), "started")
	backend := &backendFixture{marker: marker}
	fake := &backendHarness{fakeHarness: fakeHarness{script: fmt.Sprintf("test -f %q", marker)}, backend: backend}
	observed := false
	code, err := Run(context.Background(), Request{Harness: fake, Dir: dir, Name: "api", OnTurn: func(_ context.Context, self registry.Session, result adapter.TurnEnd) (mail.Outcome, error) {
		observed = self.Name == "api-fake" && result.Answer == "result"
		return mail.Outcome{}, nil
	}})
	if err != nil || !observed || !backend.closed {
		t.Fatalf("lifecycle: code=%d err=%v observed=%v closed=%v", code, err, observed, backend.closed)
	}
	if code != 0 {
		t.Fatal("the harness started before the service was ready")
	}
}
