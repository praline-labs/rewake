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

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/delivery"
	"github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Request is one launch.
type Request struct {
	// Harness is what to run.
	Harness adapter.Harness
	// Protected is what no task may grant the session to write: the
	// catalog's set, every adapter's and the foreign ones it declares.
	Protected adapter.Protected
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
	Role role.Role
	// OnTurn publishes a turn's end the run's TurnBoundary reported.
	OnTurn func(context.Context, registry.Session, adapter.TurnEnd) error
	// OnConfirm runs an end the adapter may hold open, beside OnTurn
	// (adapter.TurnHandler.Confirm).
	OnConfirm func(context.Context, registry.Session, adapter.TurnEnd) (string, error)
	// MailTool serves the mail tool's endpoint; nil serves none.
	MailTool *MailTool
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

	// A run is named by the boot too: after a restart of the machine the same
	// pid and start can name another process.
	boot, err := registry.CurrentBoot()
	if err != nil {
		return 0, fmt.Errorf("could not read the machine's boot id, which names this run: %w", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	// The version is read before the run is claimed, so a refusal leaves no
	// record, no session and no socket behind.
	version, err := readVersion(request, cwd)
	if err != nil {
		return 0, err
	}
	if request.Role.ID == role.Main.ID {
		if authority := listenAuthority(request.Dir, registry.RunEpoch(self, selfStart, boot), self); authority != nil {
			defer authority.Close()
			go authority.Serve(ctx)
		}
	}

	// The name is claimed before anything is prepared. Preparing first means a
	// launch that loses the race has already touched what belongs to the session
	// that won — its socket file, for one.
	session, err := claimName(request, self, selfStart, boot, cwd)
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
	if request.Harness.Offers().Control.Any() {
		made := registry.ControlFor(request.Dir, name, epoch)
		if control.Prepare(made) == nil {
			controlDir = made
			defer func() { _ = os.RemoveAll(made) }()
		}
	}

	// Before the harness: a resumed conversation's grants are given at launch.
	restored := resumeGrants(request.Dir, name, epoch, request.Harness, request.Args, request.Protected)

	tool := startMailTool(request, name, epoch)
	defer tool.close()
	live := newLiveSet(request.Dir, name, epoch)
	defer live.stop()

	launchRequest := adapter.LaunchRequest{
		Name:       name,
		Dir:        state.RootForRoom(request.Dir),
		RoomDir:    request.Dir,
		Room:       session.Room,
		RoleReason: session.RoleReason,
		Args:       request.Args,
		Intro:      request.Intro,
		Command:    request.Command,
		Socket:     registry.SocketFor(request.Dir, name, epoch),
		Epoch:      epoch,
		Role:       role.Of(session.Role),

		ControlDir: controlDir,
		GrantDirs:  restored.dirs(),
		Version:    version,
	}
	launchRequest.Env = launch.SessionEnv(launchRequest, request.Harness.EnvNotInherited())
	plan, err := request.Harness.Launch(launchRequest)
	if err != nil {
		return 0, err
	}
	plan.Notes = append(plan.Notes, restored.notes...)
	tool.attach(plan)
	live.planned = plannedLive(request.Harness.Offers(), plan, controlDir)
	live.exchanged = tool.exchanged
	_, _ = fmt.Fprintf(os.Stderr, "rewake: room %s, role %s: %s\n", session.Room, session.Role, session.RoleReason)
	for _, note := range plan.Notes {
		// Said once, on stderr, before the harness takes over the screen: these
		// are things rewake decided not to do, and silence about them would look
		// like it had done them.
		_, _ = fmt.Fprintln(os.Stderr, "rewake: "+note)
	}

	session.Socket = plan.Socket
	session.OwnsSocket = plan.OwnsSocket
	if err := registry.Update(request.Dir, session); err != nil {
		return 0, err
	}
	// Signals are caught before the child exists. In the gap between starting it
	// and installing the handlers, a SIGTERM meant for the wrapper would kill it
	// outright: the harness would keep running with nobody serving its mailbox
	// and its record left behind.
	signals, restore := catchSignals()
	defer restore()

	stopServing, err := startServing(ctx, request, session, name, epoch, plan, tool, live)
	if err != nil {
		return 0, err
	}
	defer stopServing()
	if err := awaitBoundary(ctx, session, request.Harness, live); err != nil {
		return 0, err
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

	process, _ := plan.Service.(adapter.ProcessService)
	tool.started(command.Process.Pid, process)
	if start, err := proc.StartTime(command.Process.Pid); err == nil {
		session.HarnessPID = command.Process.Pid
		session.HarnessStart = start
		// The record as it stands: the live set may have moved since the
		// copy here was read.
		_ = registry.Modify(request.Dir, name, epoch, func(record *registry.Session) {
			record.HarnessPID, record.HarnessStart = session.HarnessPID, session.HarnessStart
		})
	}
	if process != nil {
		watchCtx, stopWatching := context.WithCancel(ctx)
		defer stopWatching()
		go stopWithProcess(watchCtx, process, command.Process, session.HarnessStart, tool.keeper.exited)
	}
	harnessStart := session.HarnessStart
	tool.keeper.started(ctx, func() bool { return proc.Alive(command.Process.Pid, harnessStart) })
	go forward(signals, command.Process, func() bool {
		return proc.Alive(command.Process.Pid, harnessStart)
	}, tool.keeper.exited)

	serveCtx, stopServing := context.WithCancel(ctx)
	defer stopServing()
	served := make(chan struct{})
	serviceReady := make(chan struct{})
	stopAvailability := startAvailability(serveCtx, request.Dir, session, serviceReady, func() bool {
		if !proc.Alive(command.Process.Pid, harnessStart) {
			return false
		}
		// A process that speaks for the harness makes it available once it
		// names the conversation.
		if process != nil && plan.Thread != nil {
			thread, err := plan.Thread.Thread()
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
		// No adapter follows its conversation outside a service since S9;
		// wrap/thread_test.go holds the tracker's use until one does.
		var thread func() (string, error)
		if tracker, ok := request.Harness.(adapter.ThreadTracker); ok {
			thread = func() (string, error) {
				record := current(request.Dir, name, session)
				return tracker.Thread(adapter.RunProcess{PID: record.HarnessPID, Start: record.HarnessStart})
			}
		}
		if plan.Thread != nil {
			thread = plan.Thread.Thread
		}
		server := &delivery.Server{
			Ready:  func() { close(serviceReady) },
			Dir:    request.Dir,
			Thread: threadIfLive(thread, live),
			Name:   name,
			Epoch:  epoch,
			// Asked before the mailbox is touched at all: a serving goroutine
			// that starts late, after its harness is gone and the name has
			// changed hands, has no business in there.
			Owns: func() bool { return registry.OwnsName(request.Dir, name, epoch) },
			// A burst of letters that ask for nothing wakes the session once.
			Window:     delivery.Coalescing,
			CheckGrant: checkGrant(request.Dir, name, epoch, request.Protected),
			PinGrant:   pinGrant(request.Dir, name, epoch),
			Deliver:    wakeIfLive(plan.Wake, live),
		}
		if reserver, ok := plan.Wake.(adapter.Reserver); ok {
			server.Reserve = reserveIfLive(reserver, live)
		}
		if receiver, ok := plan.Wake.(adapter.Receiver); ok {
			server.Receipts, server.Opened = receiver.Receipts(), receiver.Opened()
		}
		server.Serve(serveCtx)
	}()

	code := waitForHarness(command.Process.Pid)
	tool.keeper.exited()

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
// harness may have moved on — a new conversation, a recreated socket — and the
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
