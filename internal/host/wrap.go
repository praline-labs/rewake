/*
Package host runs a harness as a rewake session.

The wrapper is the parent of the harness and nothing more: the program keeps
this terminal, its own flags and its own login. What the wrapper adds is a name
in the registry and a process that serves this session's mailbox for exactly as
long as the session exists. There is no daemon, so there is no state where the
sessions are listed but nothing is left to deliver to them.
*/
package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/core/work"
	"github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Run starts the harness, serves its mailbox until it exits, and returns the
// exit code of the harness itself: rewake is in the middle, and a caller
// scripting around it should see what the program said.
func Run(ctx context.Context, request Request) (int, error) {
	r := &run{request: request, entry: entryOf(request)}
	// Whatever a phase took is given back once Run returns, latest first; a
	// failure is the entry's to answer before that (entry.fail).
	defer r.undo()
	for _, phase := range []func(context.Context) error{r.claim, r.prepare, r.serve, r.start} {
		if err := phase(ctx); err != nil {
			return r.entry.fail(r, err)
		}
	}
	return r.deliver(ctx), nil
}

// run is one run of the wrapper as its phases build it: what each phase
// took, which the next ones use and a failure reads.
type run struct {
	request Request
	entry   entry

	// From the claim: the room's base and lease, the run's lifetime, the
	// harness's version read, the run's record, its live set and its
	// directory.
	base    string
	lease   *build.Lease
	life    *lifetime
	version string
	started adapter.Start
	session registry.Session
	epoch   string
	live    *liveSet
	runDir  string

	// From preparing: the face's model, the plan and the close of its
	// service, once.
	model        *mirror.Model
	plan         adapter.LaunchPlan
	closeService func()

	// From serving: the run's catch of signals, and the finished turns'
	// ends the delivery server reminds by.
	signals chan os.Signal
	ends    chan int64

	// From the start: the harness and how its stops are followed.
	harness *os.Process
	onStop  func(int)

	undos []func()
}

// atUndo adds what Run gives back once it returns.
func (r *run) atUndo(undo func()) { r.undos = append(r.undos, undo) }

func (r *run) undo() {
	for i := len(r.undos) - 1; i >= 0; i-- {
		r.undos[i]()
	}
}

// claim takes the room's lease and the run's name, its live set and its
// directory. The build lease comes first and goes last: every lock, callback,
// worker and cleanup of the run is inside it. A caller hands over the one it
// took rather than holding it shared while this asks for it exclusive.
func (r *run) claim(context.Context) error {
	request := r.request
	// The base the room belongs to, checked: it is what the children are
	// told, and a directory that is no room of a base is refused rather than
	// read two levels up.
	base, err := state.BaseForRoom(request.Dir)
	if err != nil {
		return err
	}
	r.base = base
	lease := request.Lease
	if lease == nil {
		acquired, err := r.entry.lease(base, filepath.Base(request.Dir))
		if err != nil {
			return err
		}
		r.atUndo(acquired.Release)
		if note := acquired.Note(); note != "" {
			_, _ = fmt.Fprintln(os.Stderr, note)
		}
		lease = acquired
	}
	r.lease = lease
	// The run's own hold outlasts the caller's: it goes once every worker
	// the run started has stopped, whenever that is.
	r.life = &lifetime{keep: lease.Keep()}
	r.atUndo(r.life.end)
	self := os.Getpid()
	selfStart, err := proc.StartTime(self)
	if err != nil {
		return fmt.Errorf("could not read the start time of this process: %w", err)
	}
	// A run is named by the boot too: after a restart of the machine the same
	// pid and start can name another process.
	boot, err := registry.CurrentBoot()
	if err != nil {
		return fmt.Errorf("could not read the machine's boot id, which names this run: %w", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// The version is read before the run is claimed, so a refusal leaves no
	// record, no session and no socket behind.
	if r.version, r.started, err = r.entry.version(request, cwd); err != nil {
		return err
	}
	// The name is claimed before anything is prepared. Preparing first means a
	// launch that loses the race has already touched what belongs to the session
	// that won — its socket file, for one.
	session, err := r.entry.claim(request, self, selfStart, boot, cwd)
	if err != nil {
		return err
	}
	// The record is the run's one authorship snapshot: a launch wrote what it
	// resolved when it claimed the name, and an adoption takes that same value
	// back rather than rereading configuration that may have changed since.
	r.takeAuthorship(session)
	r.session, r.epoch = session, session.Epoch()
	r.atUndo(func() {
		// Ownership is settled once, before anything is removed. Asking again
		// afterwards always answered no — the record had just been deleted — so
		// the socket of a session that ended was left behind for good.
		// The record goes only while it is still ours. The socket is ours
		// whatever happened to the name: its path names this run, so the next
		// holder of the name has a socket of its own that this cannot reach.
		_, _ = registry.RemoveOwned(request.Dir, r.session.Name, r.epoch)
		if r.session.OwnsSocket {
			removeSocket(r.session.Socket)
		}
	})
	// The live set is the run's from its claim: a held run withdraws it
	// whatever failed after (unadopted.go).
	r.live = newLiveSet(request.Dir, session.Name, r.epoch)
	r.live.workers = &r.life.workers
	r.atUndo(r.live.stop)
	// The run's directory comes before anything is prepared and goes at the
	// run's end, once every worker of the run has stopped and before its hold
	// on the lease goes (docs/rules/launch.md), on every path out.
	reclaimRuns(request.Dir)
	runDir, err := makeRunDir(request.Dir, session.Name, r.epoch)
	if err != nil {
		return err
	}
	r.life.atEnd(func() { _ = state.RemoveAll(runDir) })
	r.runDir = runDir
	if request.OnClaimed != nil {
		return request.OnClaimed(session)
	}
	return nil
}

// takeAuthorship makes the claimed or adopted record the run's policy. It is
// the only place a run reads the value: nothing below it opens the alias file,
// so a configuration edit cannot change a live main's instructions.
func (r *run) takeAuthorship(session registry.Session) {
	r.request.MainImplements = session.MainImplements
}

// prepare makes what the harness is told of, plans the run and records where
// it is served.
func (r *run) prepare(context.Context) error {
	request, name := r.request, r.session.Name
	// The control directory is the run's, like its sockets: made before the
	// harness can be asked anything, removed with the session. Only for a
	// harness that serves it: a directory nobody serves would answer "not
	// answering" where "no control directory" names the real next step.
	controlDir := ""
	if request.Harness.Offers().Control.Any() {
		made := registry.ControlFor(request.Dir, name, r.epoch)
		if control.Prepare(made) == nil {
			controlDir = made
			r.atUndo(func() { _ = state.RemoveAll(made) })
		}
	}
	model, stopMirror, err := startMirror(request, name, r.epoch, r.entry.model)
	if err != nil {
		return err
	}
	r.model = model
	r.atUndo(stopMirror)
	plan, err := r.entry.plan(request, r.launchRequest(controlDir))
	if err != nil {
		return err
	}
	// The plan is the run's from here: whatever fails next, its service is
	// closed once, before the run's directory goes.
	r.plan = plan
	r.closeService = closeOnce(plan.Service)
	r.atUndo(r.closeService)
	r.live.planned = plannedLive(request.Harness.Offers(), plan, controlDir)
	r.entry.announce(r.request, r.session, plan)
	r.session.Socket = plan.Socket
	r.session.OwnsSocket = plan.OwnsSocket
	return registry.Update(request.Dir, r.session)
}

// launchRequest is what the adapter plans the run from.
func (r *run) launchRequest(controlDir string) adapter.LaunchRequest {
	request, session, name, epoch := r.request, r.session, r.session.Name, r.epoch
	launchRequest := adapter.LaunchRequest{
		Name:       name,
		Dir:        r.base,
		RoomDir:    request.Dir,
		Room:       session.Room,
		RoleReason: session.RoleReason,
		Args:       request.Args,
		Intro:      request.Intro,
		Command:    request.Command,
		Executable: r.lease.Executable(),
		Socket:     registry.SocketFor(request.Dir, name, epoch),
		Epoch:      epoch,
		Role:       role.Of(session.Role),

		MainImplements:     request.MainImplements,
		MainPolicyExplicit: request.MainPolicyExplicit,

		ControlDir: controlDir,
		RunDir:     r.runDir,
		Version:    r.version,
		Start:      r.started,
		Shell:      request.Shell,
		Template:   request.Template,

		WorktreeEntry: request.WorktreeEntry,
		Mode:          request.Mode,
		Modes:         request.Modes,
		Migrations:    request.Migrations,
		Mirror:        r.model,
		Work: func() string {
			return work.TailOf(work.Of(launch.QuietBriefHome(request.Dir), name, epoch), session.Role == role.Main.ID)
		},
	}
	launchRequest.Env = launch.SessionEnv(launchRequest, request.Harness.EnvNotInherited())
	if request.Shell {
		// The shell's pane draws main as a space host's draws a worker, so
		// main gets the same locale (docs/shell.md); its workers inherit it.
		launchRequest.Env = terminal.UTF8Locale(launchRequest.Env)
		// The workers' close starts here too, not at the main's run gone:
		// the process takes about half a second more to exit.
		launchRequest.Closing = func() {
			tellShellClosing()
			_ = space.BeginClosingFor(request.Dir, epoch, space.MainEnding)
		}
	}
	return launchRequest
}
