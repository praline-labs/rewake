package adapter

import (
	"context"
	"encoding/json"
	"os"
)

// Handover is a run's service that an exec of its wrapper can carry: the
// wrapper replaces its own image and keeps its pid, its children and the files
// the service hands over, and the new image adopts what this one ran. It is
// the probe of step 1 of "Updating without losing the sessions"
// (.scratch/reexec-probe.md), not yet a contract: one run, the same build.
type Handover interface {
	// Busy answers why the run cannot be handed over now — an obligation that
	// lives only in this image's memory — and "" when it can.
	Busy() string
	// HandOver answers the service's state and the files the next image
	// inherits, in an order Adopt reads them in. It changes nothing: a caller
	// whose exec fails closes the files and the service runs on.
	HandOver() (HandedOver, error)
	// Adopt runs the service from what an earlier image of this run handed
	// over, in place of Start: no program is started and nothing is told
	// twice.
	Adopt(ctx context.Context, note func(string), handed HandedOver) error
}

// WorkersOnly is a Handover that carries a worker's run and not main's: main's
// wrapper holds what a worker's does not — its notices of the workers, and
// turns that come often enough to want the switch deferred rather than
// refused — and a real harness's main waits for the stage that hands those
// over, answering cannot.
type WorkersOnly interface {
	WorkersOnly()
}

// HandedOver is what a service hands over across an exec.
type HandedOver struct {
	State json.RawMessage `json:"state"`
	Files []*os.File      `json:"-"`
	// Resume, when set, serves on after an exec that failed: HandOver
	// stopped what would change the state it answered.
	Resume func() `json:"-"`
}

// Contracts is a harness whose running sessions hold contracts that a switch
// of the room's build must keep (docs/rules/state.md#switching-a-running-room):
// a loaded mod, an extension, the state its service hands over. rewake
// --version --json speaks each, by name, and a switch admits only a build
// that speaks the same versions.
type Contracts interface {
	Contracts() map[string]string
}
