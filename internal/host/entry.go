package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/delivery"
	"github.com/praline-labs/rewake/internal/infra/build"
)

// entry is how a run comes to its harness: a launch starts one, an adoption
// takes the one an earlier image of this wrapper started (reexec.go). Every
// step where the two differ is a method here, so the run's phases are one
// path for both, and what the handover carries stays with the adoption.
type entry interface {
	// lease takes the room's build lease when the caller handed none.
	lease(base, room string) (*build.Lease, error)
	// version reads the harness's version before the claim, so a refusal
	// leaves nothing behind.
	version(request Request, cwd string) (string, adapter.Start, error)
	// claim claims the run's name, or takes the record claimed before.
	claim(request Request, self int, selfStart uint64, boot, cwd string) (registry.Session, error)
	// plan is what the run serves, and for a launch what it starts.
	plan(request Request, launch adapter.LaunchRequest) (adapter.LaunchPlan, error)
	// announce says on stderr what the launch decided, before the harness
	// takes the screen.
	announce(request Request, session registry.Session, plan adapter.LaunchPlan)
	// startService starts the plan's service, or adopts what was handed over.
	startService(ctx context.Context, service adapter.Service, note func(string)) error
	// startHarness starts the plan's program, or takes the one running, and
	// says how its stops are followed.
	startHarness(request Request, plan adapter.LaunchPlan) (*os.Process, func(int), error)
	// model is the face's model the run serves (startMirror).
	model() *mirror.Model
	// carried is what an earlier image handed the run's parts.
	carried() carried
	// served tells whoever handed the run over that this image serves it.
	served(runDir string)
	// fail ends the run on err and answers what Run returns.
	fail(r *run, err error) (int, error)
}

// carried is what an earlier image handed the run's parts; a launch carries
// nothing.
type carried struct {
	// signals are those a parked image caught and did not pass on.
	signals []int
	// notices are main's notice state (notices_handover.go).
	notices *Notices
	// delivery is the delivery server's reminder state.
	delivery *delivery.Memory
	// count is how many execs the run has been through.
	count int
}

// entryOf is the entry of request: an adoption when it carries a handover.
func entryOf(request Request) entry {
	if request.Handover != nil {
		return adopting{request.Handover}
	}
	return launching{}
}

// launching is the entry of a run that starts its harness.
type launching struct{}

// lease may switch the room's build.
func (launching) lease(base, room string) (*build.Lease, error) { return build.Launch(base, room) }

func (launching) claim(request Request, self int, selfStart uint64, boot, cwd string) (registry.Session, error) {
	return claimName(request, self, selfStart, boot, cwd)
}

func (launching) plan(request Request, launch adapter.LaunchRequest) (adapter.LaunchPlan, error) {
	return request.Harness.Launch(launch)
}

func (launching) announce(request Request, session registry.Session, plan adapter.LaunchPlan) {
	_, _ = fmt.Fprintf(os.Stderr, "rewake: room %s, role %s: %s\n", session.Room, session.Role, session.RoleReason)
	if session.Role == role.Main.ID {
		_, _ = fmt.Fprintln(os.Stderr, "rewake: "+mainPolicyNote(request.MainImplements, request.MainPolicyExplicit))
	}
	for _, note := range plan.Notes {
		// Said once, on stderr, before the harness takes over the screen:
		// these are things rewake decided not to do, and silence about them
		// would look like it had done them.
		_, _ = fmt.Fprintln(os.Stderr, "rewake: "+note)
	}
}

func (launching) startService(ctx context.Context, service adapter.Service, note func(string)) error {
	return service.Start(ctx, note)
}

func (launching) startHarness(request Request, plan adapter.LaunchPlan) (*os.Process, func(int), error) {
	command := exec.Command(plan.Command, plan.Args...)
	command.Env = plan.Env
	onStop, err := startHarness(request, command)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return nil, nil, fmt.Errorf("%s is not installed or not in PATH", plan.Command)
		}
		return nil, nil, err
	}
	return command.Process, onStop, nil
}

func (launching) model() *mirror.Model { return mirror.New() }
func (launching) carried() carried     { return carried{} }
func (launching) served(string)        {}

// fail returns err at once: a launch that fails has started nothing.
func (launching) fail(_ *run, err error) (int, error) { return 0, err }

// mainPolicyNote says once, at a main launch, what authorship policy this run
// froze and where it came from: the project's own answer, or the default when
// nobody set one. It names the source, never a setting's value from elsewhere
// (docs/rules/launch.md, L2).
func mainPolicyNote(mainImplements, explicit bool) string {
	switch {
	case mainImplements:
		return "main may implement product changes itself: the project's [main] implements = true, frozen for this run"
	case explicit:
		return "main delegates implementation: the project's [main] implements = false, frozen for this run"
	default:
		return "main delegates implementation: no project sets [main] implements, the default"
	}
}
