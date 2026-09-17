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
	"os/signal"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Request is one launch.
type Request struct {
	// Harness is what to run.
	Harness harness.Harness
	// Dir is the room-scoped state directory.
	Dir string
	// Name is the requested session name; empty picks a free one.
	Name string
	// Args are the caller's arguments for the harness.
	Args []string
	// Intro asks for the briefing that tells the agent it runs under rewake.
	Intro    bool
	Greeting bool
	// Role is explicit when its ID is set; empty chooses a role for the room.
	Role role.Role
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

	plan, err := request.Harness.Launch(harness.LaunchRequest{
		Name:       name,
		Dir:        state.RootForRoom(request.Dir),
		Room:       session.Room,
		RoleReason: session.RoleReason,
		Args:       request.Args,
		Intro:      request.Intro,
		Greeting:   request.Greeting,
		Socket:     registry.SocketFor(request.Dir, name, epoch),
		Epoch:      epoch,
		Role:       role.Of(session.Role),
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

	if plan.Greeting {
		if err := inbox.MarkGreeting(request.Dir, name, epoch); err != nil {
			return 0, err
		}
	}

	// Signals are caught before the child exists. In the gap between starting it
	// and installing the handlers, a SIGTERM meant for the wrapper would kill it
	// outright: the harness would keep running with nobody serving its mailbox
	// and its record left behind.
	signals, restore := catchSignals()
	defer restore()

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
	harnessStart := session.HarnessStart
	go forward(signals, command.Process, func() bool {
		return proc.Alive(command.Process.Pid, harnessStart)
	})

	serveCtx, stopServing := context.WithCancel(ctx)
	defer stopServing()
	served := make(chan struct{})
	go func() {
		defer close(served)
		var thread func() (string, error)
		if tracker, ok := request.Harness.(harness.ThreadTracker); ok {
			thread = func() (string, error) { return tracker.Thread(current(request.Dir, name, session)) }
		}
		server := &inbox.Server{
			Dir:    request.Dir,
			Thread: thread,
			Name:   name,
			Epoch:  epoch,
			// Asked before the mailbox is touched at all: a serving goroutine
			// that starts late, after its harness is gone and the name has
			// changed hands, has no business in there.
			Owns: func() bool { return registry.OwnsName(request.Dir, name, epoch) },
			Deliver: func(ctx context.Context, message inbox.Message) inbox.Result {
				return request.Harness.Deliver(ctx, current(request.Dir, name, session), message)
			},
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

// followStop stops the wrapper with a harness that stopped on its own, and
// brings the harness back when the wrapper is continued.
//
// The report of a stop can be old news. Ctrl+Z stops the whole job, wrapper
// included, and the report is read only after "fg" has continued both — stopping
// again then handed the shell a stopped job while the harness ran on its own.
// So the wrapper follows only a harness that is stopped right now.
func followStop(pid int, stopped func(int) bool, stop func()) {
	if !stopped(pid) {
		return
	}
	stop()
	_ = syscall.Kill(pid, syscall.SIGCONT)
}

// stillStopped reports whether a process is in a job-control stop now.
func stillStopped(pid int) bool {
	state, err := proc.State(pid)
	return err == nil && state == "T"
}

// stopSelf stops the wrapper the way a job is stopped.
func stopSelf() { _ = syscall.Kill(os.Getpid(), syscall.SIGSTOP) }

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

// catchSignals starts listening before there is a child to forward to.
func catchSignals() (chan os.Signal, func()) {
	incoming := make(chan os.Signal, 8)
	signal.Notify(incoming, syscall.SIGTERM, syscall.SIGHUP)
	// The keyboard signals are caught and dropped rather than left to their
	// default. The wrapper shares the harness's group, so Ctrl+C reaches it too,
	// and dying from it left the agent running with nobody serving its mailbox:
	// interrupting a turn must not end the session.
	//
	// Caught, not ignored. An ignored signal stays ignored across exec, so the
	// harness and every command it ran would have lost Ctrl+C too; a caught one
	// is reset to its default in the child.
	keyboard := make(chan os.Signal, 8)
	signal.Notify(keyboard, syscall.SIGINT, syscall.SIGQUIT)
	go func() {
		for {
			// Keyboard signals belong to the harness; consume until the channel closes.
			if _, open := <-keyboard; !open {
				return
			}
		}
	}()
	return incoming, func() {
		signal.Stop(incoming)
		// Stop guarantees nothing more is sent, so closing ends the drain.
		signal.Stop(keyboard)
		close(keyboard)
	}
}

// forwardGrace is how long the harness is given to act on a signal it may have
// received directly before the wrapper repeats it.
const forwardGrace = 300 * time.Millisecond

// forward passes a termination request on to the harness — once, and only if it
// has not already acted on one.
//
// Both processes are in the same group, so a signal sent to the group reaches
// the harness by itself; repeating it turns one request into two, and for many
// programs the second one means "stop cleaning up and die". A signal sent to the
// wrapper alone reaches nobody else, and that is the case this covers. The
// difference between them is not visible in the signal, so the answer comes from
// the harness: if it is still running a moment later, it did not get one.
func forward(incoming chan os.Signal, process *os.Process, alive func() bool) {
	for received := range incoming {
		signalValue, ok := received.(syscall.Signal)
		if !ok {
			continue
		}
		time.Sleep(forwardGrace)
		if !alive() {
			continue
		}
		_ = process.Signal(signalValue)
	}
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
