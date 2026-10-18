package spacehost

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// detachBound is how long the space host waits to see its launcher gone,
// which exits right after starting it.
const detachBound = 5 * time.Second

// host is one space host at work.
type host struct {
	spec   Spec
	record space.Record
	run    string
}

// serve is the space host's stage: it holds the room's lease for as long as
// it runs, since every step it records is a write of the room.
func serve(spec Spec) int {
	lease, err := build.Acquire(spec.Base, spec.Room)
	if err != nil {
		return 1
	}
	defer lease.Release()
	h := &host{spec: spec}
	if err := h.start(); err != nil {
		return 1
	}
	return 0
}

// start records the space host, shows it is outside main's run and starts the
// worker; anything that stops it first closes the record with the worker
// never started.
func (h *host) start() error {
	self, err := selfRun()
	if err != nil {
		return err
	}
	h.run = self
	if err := waitDetached(h.spec.Launcher, detachBound); err != nil {
		return h.never(err.Error())
	}
	record, err := space.SetHost(h.spec.Dir, h.spec.ID, self)
	if err != nil {
		// The record is not this host's to close: the open gave up on it, or
		// it names another host.
		return err
	}
	h.record = record
	pid, _, ok := registry.ParseEpoch(record.Main.Run)
	if !ok {
		return h.never("the record names no main run")
	}
	if err := proc.Default.Outside(os.Getpid(), pid); err != nil {
		return h.never("the space host could not be shown outside main's run: " + err.Error())
	}
	if space.MainRuns(record) != proc.IdentityAlive {
		return h.never("main's run had ended, or could not be told from here, before the worker started")
	}
	// The terminal is served before the worker starts: an open answered with
	// no way for the person to reach a prompt would leave the worker stuck.
	listener, err := state.Listen(state.SpaceSocketPath(h.spec.Dir, h.spec.ID))
	if err != nil {
		return h.never("the worker's terminal could not be served for rewake attach: " + err.Error())
	}
	defer func() { _ = listener.Close() }()
	pty, err := terminal.OpenPlain(h.spec.Size)
	if err != nil {
		return h.never(err.Error())
	}
	defer func() { _ = pty.Close() }()
	cmd, err := h.command()
	if err == nil {
		err = pty.Prepare(cmd)
	}
	if err == nil {
		err = cmd.Start()
	}
	_ = pty.ReleaseSide()
	if err != nil {
		return h.never("the worker's launch did not start: " + err.Error())
	}
	reaped := make(chan struct{})
	var waited error
	go func() { waited = cmd.Wait(); close(reaped) }()
	r := &relay{pty: pty}
	go r.pump()
	defer r.close()

	workerRun, err := childRun(cmd.Process.Pid)
	if err == nil {
		h.record, err = space.Opened(h.spec.Dir, h.spec.ID, workerRun)
	}
	if err != nil {
		stopGroup(cmd.Process.Pid, reaped, stopping)
		return h.never("the worker was started and stopped: its run could not be recorded: " + err.Error())
	}
	go r.serve(listener)
	why := h.watch(reaped)
	if why == "" {
		why = "the worker exited by itself" + exitWords(waited)
		if line := r.lastLine(); line != "" {
			why += "; its last line: " + line
		}
	}
	if record, err := space.BeginClosing(h.spec.Dir, h.spec.ID, why); err == nil {
		h.record = record
	}
	switch took := stopGroup(cmd.Process.Pid, reaped, stopping); took {
	case "":
		// Not seen gone, so not seen to end: the record stays closing
		// rather than claim an end nobody observed.
		return errors.New("the space stays closing: the worker's process group was still there after SIGKILL")
	case "nothing":
	default:
		why += "; the worker's process group ended after " + took
	}
	return h.close(why)
}

// watch checks main's run once a second until it has ended, by whatever
// cause, or the worker exits by itself; it fills the record's checkout once
// the worker's session claims one. It says why the space closes, empty for a
// worker that exited by itself.
func (h *host) watch(reaped <-chan struct{}) string {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-reaped:
			return ""
		case <-tick.C:
		}
		if space.MainRuns(h.record) == proc.IdentityEnded {
			return "main's run ended"
		}
		if h.spec.Worktree && h.record.Checkout.Path == "" {
			_ = h.captureCheckout()
		}
	}
}

// captureCheckout records the checkout the worker's session claimed, if it
// claimed one yet.
func (h *host) captureCheckout() error {
	checkout, found, err := claimedCheckout(h.spec.Dir, h.record.Worker.Run)
	if err != nil || !found {
		return err
	}
	record, err := space.SetCheckout(h.spec.Dir, h.spec.ID, checkout)
	if err != nil {
		return err
	}
	h.record = record
	return nil
}

// never closes the record of a worker that did not start.
func (h *host) never(why string) error {
	_, err := space.Close(h.spec.Dir, h.spec.ID, space.End{
		At: time.Now().UTC(), Why: why, Worker: space.WorkerNeverStarted, Checkout: space.Seen{Presence: space.Gone},
	})
	return errors.Join(errors.New(why), err)
}

// close records the worker's observed end and its checkout as found. The
// worker may claim its checkout up to its end, after the last poll, so the
// capture is made once more here; one that fails leaves the space closing,
// never closed as if it had made none.
func (h *host) close(why string) error {
	if h.spec.Worktree && h.record.Checkout.Path == "" {
		if err := h.captureCheckout(); err != nil {
			return fmt.Errorf("the space stays closing: its checkout could not be captured: %w", err)
		}
	}
	_, err := space.Close(h.spec.Dir, h.spec.ID, space.End{
		At: time.Now().UTC(), Why: why, Worker: space.WorkerEnded, Checkout: seen(h.spec.Dir, h.record),
	})
	return err
}

// command is the worker's launch: this rewake, in main's working directory,
// with the worker's environment.
func (h *host) command() (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	env, err := WorkerEnv(os.Environ(), h.spec.Base, h.spec.Strip)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(self, h.spec.Args...)
	cmd.Dir, cmd.Env = h.spec.Cwd, env
	return cmd, nil
}

// waitDetached waits until the launcher is no longer this process's parent:
// until then the space host is below whatever started the launcher.
func waitDetached(launcher int, limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for os.Getppid() == launcher {
		if time.Now().After(deadline) {
			return fmt.Errorf("the launcher %d was still the space host's parent after %s", launcher, limit)
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

func selfRun() (string, error) { return childRun(os.Getpid()) }

// childRun names the run of a process as an epoch: pid, start time, boot.
func childRun(pid int) (string, error) {
	start, err := proc.StartTime(pid)
	if err != nil {
		return "", err
	}
	boot, err := registry.CurrentBoot()
	if err != nil {
		return "", err
	}
	return registry.RunEpoch(pid, start, boot), nil
}

func exitWords(err error) string {
	var exit *exec.ExitError
	switch {
	case err == nil:
		return " with code 0"
	case errors.As(err, &exit) && exit.ExitCode() >= 0:
		return fmt.Sprintf(" with code %d", exit.ExitCode())
	}
	return " (" + err.Error() + ")"
}
