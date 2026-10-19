package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host"
	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// launchIdentity is who a launch is and where: what the person's launch
// never names (the owner's decision, October 19, 2026 —
// docs/v2/worker-profile.md).
type launchIdentity struct {
	// name is the address prefix; empty takes the role's.
	name string
	room string
	// directory is the resolved launch directory the room is named for,
	// empty for a room named otherwise.
	directory string
	part      role.Role
	reason    string
	// template is the alias of the worker's space, empty for --harness.
	template string
}

// identifyLaunch is identify; a test of the launch's other steps replaces it.
var identifyLaunch = identify

// identify takes a worker's identity from the space that started it, and
// otherwise makes the launch main in the room of its directory.
func identify(call Call) (launchIdentity, error) {
	if value, ok := os.LookupEnv(spacehost.WorkerSpaceEnv); ok {
		return spaceIdentity(value)
	}
	return directIdentity(call)
}

// ownRoom is main in the room of the launch directory.
func ownRoom() (launchIdentity, error) {
	working, err := os.Getwd()
	if err != nil {
		return launchIdentity{}, &UsageError{Message: "the room is named by the launch directory, which cannot be read: " + err.Error()}
	}
	room, resolved, err := state.RoomOf(working)
	if err != nil {
		return launchIdentity{}, &UsageError{Message: err.Error()}
	}
	return launchIdentity{room: room, directory: resolved, part: role.Main, reason: "rewake <harness> always starts main, one per directory"}, nil
}

// spaceIdentity is the worker of the space the variable names. It is
// believed only from that space's host, the launch's parent, while the space
// is opening, or open on this very run: anyone else naming a space would
// start a worker past main.
func spaceIdentity(value string) (launchIdentity, error) {
	_ = os.Unsetenv(spacehost.WorkerSpaceEnv)
	template := os.Getenv(spacehost.WorkerTemplateEnv)
	_ = os.Unsetenv(spacehost.WorkerTemplateEnv)
	refuse := func(why string) (launchIdentity, error) {
		return launchIdentity{}, &FailedError{Message: fmt.Sprintf(
			"%s=%s names the space this worker belongs to, but %s; a person starts main, and main starts workers with rewake space open.",
			spacehost.WorkerSpaceEnv, value, why)}
	}
	room, id, ok := strings.Cut(value, "/")
	if !ok || !state.ValidName(room) {
		return refuse("it is not <room>/<space id>")
	}
	root, err := state.Root()
	if err != nil {
		return launchIdentity{}, &UsageError{Message: err.Error()}
	}
	dir, err := state.RoomDir(root, room)
	if err != nil {
		return refuse(err.Error())
	}
	record, err := space.Load(dir, id)
	if err != nil {
		return refuse("its record cannot be read: " + err.Error())
	}
	// The host records the worker's run as soon as it started it, so the
	// launch may already find the space open, naming this very process.
	if record.State != space.Opening && (record.State != space.Open || record.Worker.Run != thisRun()) {
		return refuse("the space is " + string(record.State) + ", not opening")
	}
	pid, start, ok := registry.ParseEpoch(record.Host)
	parent := os.Getppid()
	if started, err := proc.StartTime(parent); !ok || pid != parent || err != nil || started != start {
		return refuse("its host is not the process that started this launch")
	}
	part, found := role.Find(record.Worker.Role)
	if !found || part.Spaces {
		return refuse("its record names no worker's role")
	}
	return launchIdentity{name: record.Worker.Name, room: room, part: part, reason: "raised by main with rewake space open as " + part.ID, template: template}, nil
}

// thisRun is this process's run epoch, as its space host records it.
func thisRun() string {
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		return ""
	}
	boot, err := registry.CurrentBoot()
	if err != nil {
		return ""
	}
	return registry.RunEpoch(os.Getpid(), start, boot)
}

// mainTaken says a launch found its room's main running.
func mainTaken(id launchIdentity, taken *host.MainTakenError) error {
	if id.directory == "" {
		return &FailedError{Message: taken.Error() + "."}
	}
	return &FailedError{Message: fmt.Sprintf(
		"%s already has main %s: one main per directory. Use that one, or start rewake in another directory, which is another room.",
		id.directory, taken.Name)}
}
