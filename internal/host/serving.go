package host

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/host/herdr"
	"github.com/praline-labs/rewake/internal/infra/build"
)

// startServing starts what serves the session beside the harness — the run's
// turn boundary, its service and its telemetry — before the harness itself,
// and returns what stops them, in the reverse order. closeService closes the
// plan's service once: Run owns it from the moment the plan is returned, and
// a stop here only brings that close before the read clock's.
func startServing(ctx context.Context, request Request, self registry.Session, name, epoch string, plan adapter.LaunchPlan, live *liveSet, closeService func()) (func(), error) {
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
		// (docs/rules/effects.md#a-pending-mark); a
		// start that cannot be recorded leaves the mark unproven, which
		// fails to the side of a report.
		record := func(at int64) { _ = mail.RecordTurnStart(request.Dir, name, epoch, at) }
		handler := adapter.TurnHandler{Capture: clock.Snapshot, Publish: func(ctx context.Context, end adapter.TurnEnd) error {
			if request.OnTurn != nil {
				return request.OnTurn(ctx, reportSession, end)
			}
			return nil
		}}
		handler.Started = func(_, _ string, at int64) {
			if at > 0 {
				record(at)
			}
		}
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

	if plan.Telemetry != nil {
		stops = append(stops, session.Start(ctx, request.Dir, name, epoch, plan.Telemetry.SessionState, live.workers))
		if pane, ok := herdrPane(self); ok {
			stops = append(stops, herdr.Start(ctx, herdrConfig(pane, request.Harness, plan.Telemetry)))
		}
	}
	return stop, nil
}

// herdrPane is the herdr pane main runs in, which it reports itself to
// (docs/launch.md#herdr); a worker runs in none. A test binary never reports:
// it inherits the variables of the pane its session runs in, a person's own.
func herdrPane(self registry.Session) (herdr.Pane, bool) {
	if self.Role != role.Main.ID || testing.Testing() {
		return herdr.Pane{}, false
	}
	return herdr.FromEnv(os.Getenv)
}

func herdrConfig(pane herdr.Pane, harness adapter.Harness, telemetry adapter.Telemetry) herdr.Config {
	config := herdr.Config{Pane: pane, Agent: harness.ID(), State: telemetry.SessionState}
	if resumer, ok := harness.(adapter.Resumer); ok {
		config.Build, _ = build.ID()
		config.Resume = func(conversation string) []string {
			return append([]string{harness.ID()}, resumer.ResumeArgs(conversation)...)
		}
	}
	return config
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
