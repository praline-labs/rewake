package host

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// adopting is the entry of a run an earlier image of this wrapper handed
// over (reexec.go): its harness runs already, on what its launch prepared, so
// nothing of a start is read, planned, checked or said again. How it fails is
// unadopted.go.
type adopting struct{ h *Handover }

// lease only follows the room's build (build.Follow).
func (adopting) lease(base, room string) (*build.Lease, error) { return build.Follow(base, room) }

// version reads nothing: a gate that refused the running harness now — a
// pinned copy the new build pins at another version, a program replaced since
// the launch — would end a running session rather than keep one from
// starting. The plan uses neither answer (adapter.Harness.Serve).
func (adopting) version(Request, string) (string, adapter.Start, error) {
	return "", adapter.Start{}, nil
}

// claim takes the record an earlier image of this process claimed: same pid,
// same start, same epoch.
func (a adopting) claim(request Request, self int, selfStart uint64, _, _ string) (registry.Session, error) {
	session, err := registry.Load(request.Dir, a.h.Name)
	if err != nil {
		return registry.Session{}, err
	}
	if session.ServicePID != self || session.ServiceStart != selfStart || session.Epoch() != a.h.Epoch {
		return registry.Session{}, errors.New("the record of the run handed over is not this process's any more")
	}
	return session, nil
}

// plan is the service alone (adapter.Harness.Serve).
func (adopting) plan(request Request, launch adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	return adapter.LaunchPlan{ServicePlan: request.Harness.Serve(launch)}, nil
}

// announce says nothing: the image's stderr is the harness's terminal, where
// a line drawn again lands inside the harness's own screen — pi's input box
// after a switch (found in review, November 2, 2026).
func (adopting) announce(registry.Session, adapter.LaunchPlan) {}

func (a adopting) startService(ctx context.Context, service adapter.Service, note func(string)) error {
	adopter, ok := service.(adapter.Handover)
	if !ok {
		return errors.New("the run was handed over to an adapter that cannot adopt it")
	}
	return adopter.Adopt(ctx, note, adapter.HandedOver{State: a.h.Service, Files: a.h.files})
}

func (a adopting) startHarness(Request, adapter.LaunchPlan) (*os.Process, func(int), error) {
	return a.h.adoptHarness()
}

// model is the one the last image handed over, which a face reconnects to;
// one that cannot be read is replaced by a model that says what it lost.
func (a adopting) model() *mirror.Model {
	adopted, err := adoptModel(a.h)
	if err == nil {
		return adopted
	}
	model := mirror.New()
	model.Apply(mirror.Event{Kind: mirror.EventNotice, ID: fmt.Sprintf("rewake-handover-%d", a.h.Count), Text: "rewake was updated while the session ran, and what came before could not be kept (" + err.Error() + "); the native face has it."})
	return model
}

func (a adopting) carried() carried {
	return carried{signals: a.h.Signals, notices: a.h.Notices, delivery: a.h.Delivery, count: a.h.Count}
}

// served marks the outcome for the sender of the exec.
func (a adopting) served(runDir string) {
	(&reexec{runDir: runDir}).mark(fmt.Sprintf("adopted %d", a.h.Count))
}

// adoptHarness takes the harness an earlier image started, which is this
// process's child still.
func (h *Handover) adoptHarness() (*os.Process, func(int), error) {
	if start, err := proc.StartTime(h.HarnessPID); err != nil || start != h.HarnessStart {
		return nil, nil, errors.New("the harness handed over no longer runs")
	}
	process, err := os.FindProcess(h.HarnessPID)
	if h.Shell {
		// Its suspends are answered again: the earlier image's catch went
		// with it (startHarness).
		go continueSuspends(catchSuspends(), h.HarnessPID)
		return process, continueHarness, err
	}
	return process, followJob, err
}
