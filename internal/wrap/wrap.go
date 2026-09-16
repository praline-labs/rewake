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
	name, err := registry.ChooseName(request.Dir, request.Name, request.Harness.ID())
	if err != nil {
		return 0, err
	}

	self := os.Getpid()
	selfStart, err := proc.StartTime(self)
	if err != nil {
		return 0, fmt.Errorf("could not read the start time of this process: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}

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

	session := registry.Session{
		Name:         name,
		Harness:      request.Harness.ID(),
		ServicePID:   self,
		ServiceStart: selfStart,
		CWD:          cwd,
		StartedAt:    time.Now(),
		Socket:       plan.Socket,
		CodexHome:    plan.CodexHome,
	}
	if err := registry.Publish(request.Dir, session); err != nil {
		return 0, err
	}
	defer func() {
		_ = registry.Remove(request.Dir, name)
		removeSocket(plan.Socket)
	}()

	command := exec.Command(plan.Command, plan.Args...)
	command.Env = plan.Env
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	// The harness stays in this process group so the terminal keeps treating it
	// as the foreground program: Ctrl+C, window size and job control all reach
	// it the way they would without rewake in between.
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

	serveCtx, stopServing := context.WithCancel(ctx)
	defer stopServing()
	served := make(chan struct{})
	go func() {
		defer close(served)
		server := &inbox.Server{
			Dir:  request.Dir,
			Name: name,
			Deliver: func(ctx context.Context, message inbox.Message) inbox.Result {
				return request.Harness.Deliver(ctx, current(request.Dir, name, session), message)
			},
		}
		server.Serve(serveCtx)
	}()

	restore := forwardSignals(command.Process)
	defer restore()

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
// record is where that shows up.
func current(dir, name string, fallback registry.Session) registry.Session {
	if session, err := registry.Load(dir, name); err == nil {
		return session
	}
	return fallback
}

// forwardSignals keeps the terminal behaving as if the harness were alone.
// Ctrl+C reaches the harness directly through the process group, and the
// wrapper must not die from it and leave the harness without a mailbox; a
// termination request from outside, though, is passed on.
func forwardSignals(process *os.Process) func() {
	incoming := make(chan os.Signal, 4)
	signal.Notify(incoming, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGHUP)

	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case received := <-incoming:
				switch received {
				case syscall.SIGINT, syscall.SIGQUIT:
					// The harness has it already: same process group.
				default:
					_ = process.Signal(received)
				}
			}
		}
	}()

	return func() {
		signal.Stop(incoming)
		close(done)
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

// exitCode turns the result of Wait into the code the harness exited with.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return 1
}
