/*
Package wrap runs a harness as a rewake session.

The wrapper is the parent of the harness and nothing more: the program keeps
this terminal, its own flags and its own login. What the wrapper adds is a name
in the registry and a process that serves this session's mailbox for exactly as
long as the session exists. There is no daemon, so there is no state where the
sessions are listed but nothing is left to deliver to them.
*/
package wrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/iiiokojiadbi/rewake/internal/control"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Request is one launch.
type Request struct {
	// Harness is what to run.
	Harness harness.Harness
	// Dir is the room-scoped state directory.
	Dir string
	// Name is the requested prefix; empty uses the role and picks a free address.
	Name string
	// Args are the caller's arguments for the harness.
	Args []string
	// Intro asks for the briefing that tells the agent it runs under rewake.
	Intro bool
	// Command is the program to start in place of the harness's own.
	Command string

	// Role is explicit when its ID is set; empty always uses general.
	Role   role.Role
	OnTurn func(context.Context, registry.Session, harness.Completion) error
	// OnClaimed hears the session once its name is claimed, before the
	// harness is planned: whatever was prepared for the launch learns whose
	// it is. An error ends the launch.
	OnClaimed func(registry.Session) error
}

// Run starts the harness, serves its mailbox until it exits, and returns the
// exit code of the harness itself: rewake is in the middle, and a caller
// scripting around it should see what the program said.
func Run(ctx context.Context, request Request) (int, error) {
	self := os.Getpid()
	selfStart, err := proc.StartTime(self)
	if err != nil {
		return 0, fmt.Errorf("could not read the start time of this process: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}

	// The name is claimed before anything is prepared. Preparing first means a
	// launch that loses the race has already touched what belongs to the session
	// that won — its socket file, for one.
	session, err := claimName(request, self, selfStart, cwd)
	if err != nil {
		return 0, err
	}
	name := session.Name
	epoch := session.Epoch()
	defer func() {
		// Ownership is settled once, before anything is removed. Asking again
		// afterwards always answered no — the record had just been deleted — so
		// the socket of a session that ended was left behind for good.
		// The record goes only while it is still ours. The socket is ours
		// whatever happened to the name: its path names this run, so the next
		// holder of the name has a socket of its own that this cannot reach.
		_, _ = registry.RemoveOwned(request.Dir, name, epoch)
		if session.OwnsSocket {
			removeSocket(session.Socket)
		}
	}()
	if request.OnClaimed != nil {
		if err := request.OnClaimed(session); err != nil {
			return 0, err
		}
	}

	// The control directory is the run's, like its sockets: made before the
	// harness can be asked anything, removed with the session. Only for a
	// harness that serves it: a directory nobody serves would answer "not
	// answering" where "no control directory" names the real next step.
	controlDir := ""
	if _, steerable := request.Harness.(harness.Steerable); steerable {
		made := registry.ControlFor(request.Dir, name, epoch)
		if control.Prepare(made) == nil {
			controlDir = made
			defer func() { _ = os.RemoveAll(made) }()
		}
	}

	plan, err := request.Harness.Launch(harness.LaunchRequest{
		Name:       name,
		Dir:        state.RootForRoom(request.Dir),
		Room:       session.Room,
		RoleReason: session.RoleReason,
		Args:       request.Args,
		Intro:      request.Intro,
		Command:    request.Command,
		Socket:     registry.SocketFor(request.Dir, name, epoch),
		Epoch:      epoch,
		Role:       role.Of(session.Role),

		ObservationSocket: registry.ObservationFor(request.Dir, name, epoch),
		ControlDir:        controlDir,
	})
	if err != nil {
		return 0, err
	}
	_, _ = fmt.Fprintf(os.Stderr, "rewake: room %s, role %s: %s\n", session.Room, session.Role, session.RoleReason)
	for _, note := range plan.Notes {
		// Said once, on stderr, before the harness takes over the screen: these
		// are things rewake decided not to do, and silence about them would look
		// like it had done them.
		_, _ = fmt.Fprintln(os.Stderr, "rewake: "+note)
	}

	session.Socket = plan.Socket
	session.OwnsSocket = plan.OwnsSocket
	session.CodexHome = plan.CodexHome
	if err := registry.Update(request.Dir, session); err != nil {
		return 0, err
	}

	// Signals are caught before the child exists. In the gap between starting it
	// and installing the handlers, a SIGTERM meant for the wrapper would kill it
	// outright: the harness would keep running with nobody serving its mailbox
	// and its record left behind.
	signals, restore := catchSignals()
	defer restore()

	if plan.Backend != nil {
		reportSession := session
		clock, err := inbox.OpenReadClock(ctx, request.Dir, name, epoch)
		if err != nil {
			return 0, err
		}
		defer clock.Close()
		if err := plan.Backend.Start(ctx, harness.CompletionHandler{Capture: clock.Snapshot, Publish: func(ctx context.Context, result harness.Completion) error {
			if request.OnTurn != nil {
				return request.OnTurn(ctx, reportSession, result)
			}
			return nil
		}}, func(note string) { _, _ = fmt.Fprintln(os.Stderr, "rewake: "+note) }); err != nil {
			return 0, err
		}
		defer plan.Backend.Close()
		if observer, ok := plan.Backend.(harness.ObservedBackend); ok {
			defer sessionstate.Start(ctx, request.Dir, name, epoch, observer.SessionState)()
		}
	}

	if plan.Backend == nil && plan.Observer != nil {
		if reporter, ok := plan.Observer.(harness.TurnReporter); ok && request.OnTurn != nil {
			// The outcomes the observer hears are published the way a
			// backend's are, bounded by what was read when each was heard.
			// Without the clock they are not published at all, and the
			// session reports as it did before.
			reportSession := session
			if clock, err := inbox.OpenReadClock(ctx, request.Dir, name, epoch); err != nil {
				_, _ = fmt.Fprintln(os.Stderr, "rewake: not reporting interrupted turns: "+err.Error())
			} else {
				defer clock.Close()
				reporter.ReportTurns(harness.CompletionHandler{Capture: clock.Snapshot, Publish: func(ctx context.Context, result harness.Completion) error {
					return request.OnTurn(ctx, reportSession, result)
				}})
			}
		}
		// Started before the harness, so its first hook finds the socket.
		// Telemetry that cannot start costs telemetry, never the session.
		// Closed either way: what the launch wrote beside the socket goes.
		defer plan.Observer.Close()
		if err := plan.Observer.Start(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "rewake: not collecting telemetry: "+err.Error())
		} else {
			defer sessionstate.Start(ctx, request.Dir, name, epoch, plan.Observer.SessionState)()
		}
	}

	if plan.Backend == nil && plan.Lane != nil {
		// Before the harness, like the telemetry socket: its answer to the
		// first notice goes to an address that has to exist by then.
		if err := plan.Lane.Start(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "rewake: "+err.Error())
		}
		defer plan.Lane.Close()
	}

	command := exec.Command(plan.Command, plan.Args...)
	command.Env = plan.Env
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	// The harness stays in the wrapper's process group, which is what makes the
	// terminal treat it as the program it is: Ctrl+C, Ctrl+Z, window size and
	// job control all work as they would without rewake in between. Giving it a
	// group of its own and handing over the terminal was tried, and it broke
	// exactly that — Ctrl+Z left the terminal with a stopped harness, and
	// starting rewake in the background took the terminal away from the shell.
	if err := command.Start(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return 0, fmt.Errorf("%s is not installed or not in PATH", plan.Command)
		}
		return 0, err
	}

	if start, err := proc.StartTime(command.Process.Pid); err == nil {
		session.HarnessPID = command.Process.Pid
		session.HarnessStart = start
		_ = registry.Update(request.Dir, session)
	}
	if plan.Backend != nil {
		watchCtx, stopWatching := context.WithCancel(ctx)
		defer stopWatching()
		go stopWithBackend(watchCtx, plan.Backend, command.Process, session.HarnessStart)
	}
	harnessStart := session.HarnessStart
	go forward(signals, command.Process, func() bool {
		return proc.Alive(command.Process.Pid, harnessStart)
	})

	serveCtx, stopServing := context.WithCancel(ctx)
	defer stopServing()
	served := make(chan struct{})
	serviceReady := make(chan struct{})
	stopAvailability := startAvailability(serveCtx, request.Dir, session, serviceReady, func() bool {
		if !proc.Alive(command.Process.Pid, harnessStart) {
			return false
		}
		if plan.Backend != nil {
			thread, err := plan.Backend.Thread()
			return err == nil && thread != ""
		}
		if plan.Socket != "" {
			info, err := os.Stat(plan.Socket)
			return err == nil && info.Mode()&os.ModeSocket != 0
		}
		return true
	})
	defer stopAvailability()
	go func() {
		defer close(served)
		var thread func() (string, error)
		if tracker, ok := request.Harness.(harness.ThreadTracker); ok {
			thread = func() (string, error) { return tracker.Thread(current(request.Dir, name, session)) }
		}
		if source, ok := plan.Observer.(harness.ThreadSource); ok && plan.Backend == nil {
			thread = source.Thread
		}
		if plan.Backend != nil {
			thread = plan.Backend.Thread
		}
		server := &inbox.Server{
			Ready:  func() { close(serviceReady) },
			Dir:    request.Dir,
			Thread: thread,
			Name:   name,
			Epoch:  epoch,
			// Asked before the mailbox is touched at all: a serving goroutine
			// that starts late, after its harness is gone and the name has
			// changed hands, has no business in there.
			Owns: func() bool { return registry.OwnsName(request.Dir, name, epoch) },
			// A burst of letters that ask for nothing wakes the session once.
			Window: inbox.Coalescing,
			Deliver: func(ctx context.Context, message inbox.Message) inbox.Result {
				if plan.Backend != nil {
					return plan.Backend.Deliver(ctx, message)
				}
				if plan.Lane != nil {
					return plan.Lane.Deliver(ctx, current(request.Dir, name, session), message)
				}
				return request.Harness.Deliver(ctx, current(request.Dir, name, session), message)
			},
		}
		if backend, ok := plan.Backend.(harness.ReservingBackend); ok {
			server.Reserve = backend.Reserve
		}
		if plan.Backend == nil && plan.Lane != nil {
			server.Receipts, server.Opened = plan.Lane.Receipts(), plan.Lane.Opened()
		}
		server.Serve(serveCtx)
	}()

	code := waitForHarness(command.Process.Pid)

	// Stop serving before the record goes away, and let the server refuse what
	// is still waiting: a sender blocked on a status should be told the session
	// ended rather than wait out its timeout.
	stopServing()
	<-served

	return code, nil
}

// waitForHarness waits for the harness and follows it into a stop.
//
// A harness that stops itself — Ctrl+Z reaches it through the group, but it can
// also do it on its own — would leave the shell waiting on a wrapper that is
// still running, with no prompt and no way to bring the job back. So the wrapper
// stops with it and continues with it, which is what a shell expects of a job.
func waitForHarness(pid int) int {
	for {
		var status syscall.WaitStatus
		_, err := syscall.Wait4(pid, &status, syscall.WUNTRACED|syscall.WCONTINUED, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil {
			return 1
		}

		switch {
		case status.Stopped():
			followStop(pid, stillStopped, stopSelf)
		case status.Continued():
			// Both are running again; nothing to do.
		case status.Signaled():
			// No exit code of its own: report what a shell would.
			return 128 + int(status.Signal())
		case status.Exited():
			return status.ExitStatus()
		}
	}
}

// current re-reads the session record so delivery sees the latest one. The
// harness may have moved on — a new Codex thread, a recreated socket — and the
// record is where that shows up. A record that is no longer ours is ignored:
// once the name has changed hands it describes a different session.
func current(dir, name string, fallback registry.Session) registry.Session {
	session, err := registry.Load(dir, name)
	if err != nil || session.Epoch() != fallback.Epoch() {
		return fallback
	}
	return session
}

// removeSocket clears the socket file of a session that has ended. The harness
// removes its own on a clean exit; this covers the rest.
func removeSocket(path string) {
	if path == "" {
		return
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return
	}
	_ = os.Remove(path)
}
