package host

import (
	"context"
	"os"
	"sync"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/host/delivery"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// serve starts what serves the session beside the harness, before it: the
// first word of the harness goes to an address that has to exist by then.
func (r *run) serve(ctx context.Context) error {
	// Signals are caught before the child exists. In the gap between starting it
	// and installing the handlers, a SIGTERM meant for the wrapper would kill it
	// outright: the harness would keep running with nobody serving its mailbox
	// and its record left behind. Taken once: a held run forwards what came
	// meanwhile on this channel.
	signals, restore := takeSignals(r.entry.carried().signals)
	r.atUndo(restore)
	r.signals = signals
	// A service whose handlers write the room counts them among the run's
	// workers: its close is bounded, and what outlasts it holds the lease.
	if counter, ok := r.plan.Service.(adapter.WorkerCounter); ok {
		counter.CountWorkers(runWorkers{life: r.life})
	}
	// Finished turn ends reach the delivery server, which reminds of what
	// each turn left unread; a few are room for ends heard close together.
	r.ends = make(chan int64, 8)
	stopServing, err := r.startServing(ctx)
	if err != nil {
		return err
	}
	r.atUndo(stopServing)
	if err := refuseWithheld(r.session, r.request.Harness, r.plan); err != nil {
		return err
	}
	return awaitBoundary(ctx, r.session, r.request.Harness, r.live)
}

// start starts the harness, or takes the one running, and records it.
func (r *run) start(context.Context) error {
	harness, onStop, err := r.entry.startHarness(r.request, r.plan)
	if err != nil {
		return err
	}
	r.harness, r.onStop = harness, onStop
	if start, err := proc.StartTime(harness.Pid); err == nil {
		r.session.HarnessPID = harness.Pid
		r.session.HarnessStart = start
		// The record as it stands: the live set may have moved since the
		// copy here was read.
		_ = registry.Modify(r.request.Dir, r.session.Name, r.epoch, func(record *registry.Session) {
			record.HarnessPID, record.HarnessStart = harness.Pid, start
		})
		if watcher, ok := r.plan.Service.(adapter.HarnessWatcher); ok {
			watcher.HarnessStarted(adapter.RunProcess{PID: harness.Pid, Start: start})
		}
	}
	go forward(r.signals, harness, r.harnessAlive)
	return nil
}

// harnessAlive says the run's harness still runs.
func (r *run) harnessAlive() bool { return proc.Alive(r.harness.Pid, r.session.HarnessStart) }

// deliver serves the session's mailbox until the harness ends, and answers
// its exit code.
func (r *run) deliver(ctx context.Context) int {
	serveCtx, stopServing := context.WithCancel(ctx)
	r.atUndo(stopServing)
	served := make(chan struct{})
	serviceReady := make(chan struct{})
	carried := r.entry.carried()
	stopAvailability, notices := startAvailability(serveCtx, r.request.Dir, r.session, carried.notices, serviceReady, r.usable(), &r.life.workers)
	r.atUndo(stopAvailability)
	handover := &reexec{
		request: r.request, session: r.session, runDir: r.runDir, service: r.plan.Service, harness: r.harness,
		start: r.session.HarnessStart, model: r.model, notices: notices, count: carried.count,
		handoff: make(chan chan<- delivery.Stop), resume: make(chan struct{}),
	}
	r.atUndo(handover.watch(serveCtx))
	r.entry.served(r.runDir)
	server := r.deliveryServer(serviceReady, handover.handoff, carried.delivery)
	go func() {
		defer close(served)
		serveHandingOver(serveCtx, server, handover.resume)
	}()

	code := waitForHarness(r.harness.Pid, r.onStop)
	// The run begins to end with its harness: no decision is taken from here.
	r.life.closeAdmission()

	// Stop serving before the record goes away, and let the server refuse what
	// is still waiting: a sender blocked on a status should be told the session
	// ended rather than wait out its timeout.
	stopServing()
	<-served
	return code
}

// usable is when the run is available: its harness alive, the boundary it
// owes proven, and its socket, where it has one, listening.
func (r *run) usable() func() bool {
	boundaryProven := availableAfterStart(r.session, r.request.Harness, r.live)
	socket := r.plan.Socket
	return func() bool {
		if !r.harnessAlive() || !boundaryProven() {
			return false
		}
		if socket != "" {
			info, err := os.Stat(socket)
			return err == nil && info.Mode()&os.ModeSocket != 0
		}
		return true
	}
}

// deliveryServer is the server of the run's mailbox, carrying notices through
// the plan's Wake while it is live, and starting from what an earlier image
// remembered.
func (r *run) deliveryServer(ready chan struct{}, handoff chan chan<- delivery.Stop, remembered *delivery.Memory) *delivery.Server {
	dir, name, epoch := r.request.Dir, r.session.Name, r.epoch
	var thread func() (string, error)
	if r.plan.Thread != nil {
		thread = r.plan.Thread.Thread
	}
	server := &delivery.Server{
		Ready:  sync.OnceFunc(func() { close(ready) }),
		Dir:    dir,
		Thread: threadIfLive(thread, r.live),
		Name:   name,
		Epoch:  epoch,
		// Asked before the mailbox is touched at all: a serving goroutine
		// that starts late, after its harness is gone and the name has
		// changed hands, has no business in there.
		Owns: func() bool { return registry.OwnsName(dir, name, epoch) },
		// A burst of letters that ask for nothing wakes the session once.
		Window:     delivery.Coalescing,
		Deliver:    wakeIfLive(r.plan.Wake, r.live),
		Ends:       r.ends,
		Handoff:    handoff,
		Remembered: remembered,
	}
	if receiver, ok := r.plan.Wake.(adapter.Receiver); ok {
		server.Receipts, server.Opened = receiver.Receipts(), receiver.Opened()
	}
	if !role.Of(r.session.Role).Silent && r.plan.Telemetry != nil {
		// A run whose turns are reported to somebody is woken when it
		// stalls under a pending mark, judged idle by the snapshot its own
		// telemetry publishes.
		server.Stall = delivery.StallBound
		server.Idle = func() bool {
			verdict := session.Load(dir, name, epoch).Judge()
			return verdict.Activity == session.ActivityIdle && !verdict.Stale
		}
	}
	return server
}
