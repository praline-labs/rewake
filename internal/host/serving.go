package host

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
)

// startServing starts what serves the session beside the harness — the run's
// turn boundary, its service and its telemetry — before the harness itself,
// and returns what stops them, in the reverse order. closeService closes the
// plan's service once: Run owns it from the moment the plan is returned, and
// a stop here only brings that close before the read clock's.
func startServing(ctx context.Context, request Request, self registry.Session, name, epoch string, plan adapter.LaunchPlan, tool *mailTool, live *liveSet, closeService func()) (func(), error) {
	var stops []func()
	stop := func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
	}
	if plan.TurnBoundary != nil {
		// The ends the boundary reports are published bounded by what was
		// read when each was heard, so the clock is open before the first
		// can be.
		reportSession := self
		clock, err := mail.OpenReadClock(ctx, request.Dir, name, epoch)
		if err != nil {
			return stop, err
		}
		stops = append(stops, clock.Close)
		// A timed start is the evidence a pending mark of an earlier turn
		// is judged by when that turn's end was lost
		// (docs/mail-bridge-turns.md#a-pending-mark-at-its-turns-end); a
		// start that cannot be recorded leaves the mark unproven, which
		// fails to the side of a report.
		record := func(at int64) { _ = mail.RecordTurnStart(request.Dir, name, epoch, at) }
		handler := tool.handler(record, clock.Snapshot, func(ctx context.Context, end adapter.TurnEnd) error {
			if request.OnTurn != nil {
				return request.OnTurn(ctx, reportSession, end)
			}
			return nil
		})
		if request.OnConfirm != nil {
			handler.Confirm = func(ctx context.Context, end adapter.TurnEnd) (string, error) {
				return request.OnConfirm(ctx, reportSession, end)
			}
		}
		plan.TurnBoundary.ReportTurns(handler)
	}

	// What the run proves reaches the record from its first report; a run
	// that reports nothing has live what its plan carries once its service
	// is up.
	if plan.Live != nil {
		plan.Live.ReportLive(live.report)
	}
	if plan.Service != nil {
		// Before the harness: its first word goes to an address that has to
		// exist by then. Closed either way: what the launch prepared for it
		// goes.
		err := plan.Service.Start(ctx, func(note string) { _, _ = fmt.Fprintln(os.Stderr, "rewake: "+note) })
		stops = append(stops, closeService)
		if err != nil {
			stop()
			return func() {}, err
		}
	}
	if plan.Live == nil {
		live.report(live.planned)
	}

	// The channel record is shown whether or not anything else of the
	// session is observed: whoami prints it in every run.
	var observed func() session.Snapshot
	if plan.Telemetry != nil {
		observed = plan.Telemetry.SessionState
	}
	if observed != nil || tool.keeper != nil {
		stops = append(stops, session.Start(ctx, request.Dir, name, epoch, tool.keeper.source(epoch, observed), live.workers))
	}
	return stop, nil
}

// closeOnce closes service the first time it is called and never again: Run
// and its stops both reach it, and an adapter's Close need not be idempotent.
func closeOnce(service adapter.Service) func() {
	return sync.OnceFunc(func() {
		if service != nil {
			service.Close()
		}
	})
}
