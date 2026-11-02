package spacehost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/host/stream"
	"github.com/praline-labs/rewake/internal/host/switching"
	"github.com/praline-labs/rewake/internal/host/terminal"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A space host joins a switch of the room's build (host/switching) as the
// worker wrappers do: on the drop it stops its relay where nothing moves,
// and execs its own image parked, with the worker's terminal handed over
// (stream.Handover); the parked image execs the switch's target on go, or
// takes the terminal again on its own build on resume. The pid, its start
// and its parent stay, so the record's host run and the lineage of SP9 stay
// true without a write, and the worker, still this process's child, runs on
// without seeing it (docs/space.md#a-switch-of-the-rooms-build).

// HandoverVersion is the shape of Handover and of the screen it carries
// (stream.ScreenVersion): an image adopts only its own, and a switch admits
// only a build that takes the same (the contract handover.space).
const HandoverVersion = 1

// handBound bounds each step of the relay's stop: a client's write in
// flight, the answers the closing grid writes.
const handBound = 2 * time.Second

// Handover is what a space host hands to the next image of itself.
type Handover struct {
	Version int             `json:"version"`
	Stream  stream.Handover `json:"stream"`
	// Worker is the worker's process, this process's child.
	Worker      int    `json:"worker"`
	WorkerStart uint64 `json:"workerStart"`
	// Count is how many execs this space host has been through.
	Count int `json:"count"`
	// Park is set on an image a switch parked: it holds no lease until its
	// leader says go or resume.
	Park *switching.Park `json:"park,omitempty"`
}

// handoff is a switch's drop, which the watch answers when the exec failed.
type handoff struct {
	park  switching.Park
	reply chan string
}

// Who names the space host to a switch's leader.
func (h *host) Who() string { return "the space host of " + h.spec.ID }

// Answer is whether the space host can be handed over now: once the space
// is open and its terminal served, and until it closes, whose close goes on
// in this image. It never cannot.
func (h *host) Answer() (string, string) {
	switch {
	case h.closing.Load():
		return switching.Busy, "the space is closing"
	case !h.open.Load():
		return switching.Busy, "the space is opening"
	}
	return switching.Ready, ""
}

// Drop hands the space host over to its own image, parked for the switch,
// through the watch; it returns only when it did not.
func (h *host) Drop(park switching.Park) string {
	if kind, why := h.Answer(); kind != switching.Ready {
		return why
	}
	request := handoff{park: park, reply: make(chan string, 1)}
	select {
	case h.handoff <- request:
	case <-time.After(5 * time.Second):
		return "the space host's watch did not take the switch"
	}
	return <-request.reply
}

// handOff stops the relay and execs this image parked; it returns only when
// the exec did not happen, the relay serving again.
func (h *host) handOff(park switching.Park) string {
	handing, err := h.relay.HandOver(handBound)
	if err != nil {
		return err.Error()
	}
	start, err := proc.StartTime(h.worker)
	if err != nil {
		return errors.Join(fmt.Errorf("the worker's run could not be read: %w", err), handing.Resume()).Error()
	}
	next := h.spec
	next.Stage, next.Launcher = stageAdopt, 0
	next.Handover = &Handover{Version: HandoverVersion, Stream: handing.Handover, Worker: h.worker, WorkerStart: start, Count: 1, Park: &park}
	if h.spec.Handover != nil {
		next.Handover.Count = h.spec.Handover.Count + 1
	}
	env, err := withSpec(os.Environ(), next)
	if err == nil {
		// The image this process runs, which the install's path may no
		// longer hold.
		err = fmt.Errorf("the exec failed: %w", syscall.Exec("/proc/self/exe", os.Args, env))
	}
	return errors.Join(err, handing.Resume()).Error()
}

// adopt is the stage of the space host's next image: parked, it waits for
// its switch's leader, then takes the room's lease again and serves the
// worker's terminal it was handed.
func adopt(spec Spec) int {
	handed := spec.Handover
	if handed == nil || handed.Version != HandoverVersion {
		_, _ = fmt.Fprintf(os.Stderr, "rewake: the space host's handover is not version %d\n", HandoverVersion)
		return 2
	}
	if handed.Park != nil {
		park := *handed.Park
		// Nothing is served while parked: the worker's output waits in the
		// kernel's buffer, a client in the listener's backlog.
		decided, err := switching.Wait(park, "the space host of "+spec.ID)
		if err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "rewake: the space host parked for a switch was lost: "+err.Error())
			return 1
		}
		handed.Park = nil
		if decided == switching.Go {
			env, err := withSpec(os.Environ(), spec)
			if err == nil {
				err = syscall.Exec(park.Staged, os.Args, env)
			}
			_, _ = fmt.Fprintln(os.Stderr, "rewake: the space host parked for a switch could not exec the new build: "+err.Error())
			return 1
		}
	}
	lease, err := build.Follow(spec.Base, spec.Room)
	if err != nil {
		return 1
	}
	defer lease.Release()
	h := &host{spec: spec, handoff: make(chan handoff)}
	defer switching.Watch(context.Background(), spec.Dir, h)()
	if err := h.adopted(handed); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "rewake: "+err.Error())
		return 1
	}
	return 0
}

// adopted runs the space handed over: its record read again from disk, the
// worker's terminal taken on, the worker waited for as the child it still is.
func (h *host) adopted(handed *Handover) error {
	self, err := selfRun()
	if err != nil {
		return err
	}
	h.run, h.worker = self, handed.Worker
	reaped, waited := waitChild(handed.Worker, handed.WorkerStart)
	// A record that does not read this time is read again, as the watch
	// does: the space is still this host's, and its worker runs.
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		if h.record, err = space.Load(h.spec.Dir, h.spec.ID); err == nil {
			break
		}
		if time.Now().After(deadline) {
			return h.finish(handed.Worker, reaped, "the space's record could not be read after a switch of the room's build: "+err.Error(), stopping)
		}
	}
	relay, listener, err := stream.Adopt(handed.Stream, state.SpaceSocketPath(h.spec.Dir, h.spec.ID), os.Getenv(terminal.DefaultsEnv), nil)
	if err != nil {
		return h.finish(handed.Worker, reaped, "the worker's terminal could not be taken on after a switch of the room's build: "+err.Error(), stopping)
	}
	defer func() { _ = listener.Close() }()
	defer func() { _ = relay.Grid().Close() }()
	h.relay = relay
	return h.serving(listener, reaped, waited)
}

// waitChild waits for the worker started by an earlier image; waited is its
// exit, read once reaped is closed. A worker that is not the one handed over
// was reaped already.
func waitChild(pid int, start uint64) (<-chan struct{}, func() error) {
	reaped := make(chan struct{})
	var waited error
	go func() {
		defer close(reaped)
		if got, err := proc.StartTime(pid); err != nil || got != start {
			waited = errors.New("it had ended before the space host's switch")
			return
		}
		process, err := os.FindProcess(pid)
		if err != nil {
			waited = err
			return
		}
		ended, err := process.Wait()
		switch {
		case err != nil:
			waited = err
		case !ended.Success():
			waited = &exec.ExitError{ProcessState: ended}
		}
	}()
	return reaped, func() error { return waited }
}
