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
)

// Request is one launch.
type Request struct {
	// Harness is what to run.
	Harness harness.Harness
	// Dir is the state directory.
	Dir string
	// Name is the requested session name; empty picks a free one.
	Name string
	// Args are the caller's arguments for the harness.
	Args []string
	// Intro asks for the briefing that tells the agent it runs under rewake.
	Intro bool
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
		ours := registry.OwnsName(request.Dir, name, epoch)
		_ = registry.RemoveOwned(request.Dir, name, epoch)
		if ours && session.OwnsSocket {
			removeSocket(session.Socket)
		}
	}()

	plan, err := request.Harness.Launch(harness.LaunchRequest{
		Name:   name,
		Dir:    request.Dir,
		Args:   request.Args,
		Intro:  request.Intro,
		Socket: registry.SocketFor(request.Dir, name),
	})
	if err != nil {
		return 0, err
	}
	for _, note := range plan.Notes {
		// Said once, on stderr, before the harness takes over the screen: these
		// are things rewake decided not to do, and silence about them would look
		// like it had done them.
		fmt.Fprintln(os.Stderr, "rewake: "+note)
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
		server := &inbox.Server{
			Dir:   request.Dir,
			Name:  name,
			Epoch: epoch,
			Deliver: func(ctx context.Context, message inbox.Message) inbox.Result {
				return request.Harness.Deliver(ctx, current(request.Dir, name, session), message)
			},
		}
		server.Serve(serveCtx)
	}()

	waitErr := command.Wait()

	// Stop serving before the record goes away, and let the server refuse what
	// is still waiting: a sender blocked on a status should be told the session
	// ended rather than wait out its timeout.
	stopServing()
	<-served

	return exitCode(waitErr), nil
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

// claimName publishes the session, retrying under an automatic name: two plain
// "rewake claude" starting together would otherwise pick the same free name and
// one of them would refuse instead of becoming claude-2. An explicit name is
// never changed — the caller is about to hand that address to somebody else.
func claimName(request Request, self int, selfStart uint64, cwd string) (registry.Session, error) {
	for attempt := 0; attempt < 16; attempt++ {
		name, err := registry.ChooseName(request.Dir, request.Name, request.Harness.ID())
		if err != nil {
			return registry.Session{}, err
		}

		session := registry.Session{
			Name:         name,
			Harness:      request.Harness.ID(),
			ServicePID:   self,
			ServiceStart: selfStart,
			PIDNamespace: proc.Namespace(),
			CWD:          cwd,
			StartedAt:    time.Now(),
		}
		err = registry.Publish(request.Dir, session)
		if err == nil {
			return session, nil
		}

		var taken *registry.NameTakenError
		if request.Name == "" && errors.As(err, &taken) {
			continue
		}
		return registry.Session{}, err
	}
	return registry.Session{}, fmt.Errorf("could not claim a name for this session: every candidate was taken while starting")
}

// catchSignals starts listening before there is a child to forward to.
func catchSignals() (chan os.Signal, func()) {
	incoming := make(chan os.Signal, 8)
	signal.Notify(incoming, syscall.SIGTERM, syscall.SIGHUP)
	return incoming, func() { signal.Stop(incoming) }
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

// exitCode turns the result of Wait into the code the harness exited with. A
// harness killed by a signal has no exit code of its own, so it gets the one a
// shell would report: 128 plus the signal number, rather than the -1 that would
// otherwise reach os.Exit and arrive as 255.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		if code := exit.ExitCode(); code >= 0 {
			return code
		}
	}
	return 1
}
