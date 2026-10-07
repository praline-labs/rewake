package wrap

import (
	"context"
	"fmt"
	"os"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/sessionstate"
)

// startServing starts what serves the session beside the harness — a
// backend, or an observer and a lane — before the harness itself, and returns
// what stops them, in the reverse order.
func startServing(ctx context.Context, request Request, session registry.Session, name, epoch string, plan harness.LaunchPlan, tool *mailTool) (func(), error) {
	var stops []func()
	stated := false
	stop := func() {
		for i := len(stops) - 1; i >= 0; i-- {
			stops[i]()
		}
	}
	if plan.Backend != nil {
		reportSession := session
		clock, err := inbox.OpenReadClock(ctx, request.Dir, name, epoch)
		if err != nil {
			return stop, err
		}
		stops = append(stops, clock.Close)
		handler := tool.handler(clock.Snapshot, func(ctx context.Context, result harness.Completion) error {
			if request.OnTurn != nil {
				return request.OnTurn(ctx, reportSession, result)
			}
			return nil
		})
		if request.OnConfirm != nil {
			handler.Confirm = func(ctx context.Context, result harness.Completion) (string, error) {
				return request.OnConfirm(ctx, reportSession, result)
			}
		}
		if err := plan.Backend.Start(ctx, handler, func(note string) { _, _ = fmt.Fprintln(os.Stderr, "rewake: "+note) }); err != nil {
			stop()
			return func() {}, err
		}
		stops = append(stops, plan.Backend.Close)
		if withdrawer, ok := plan.Backend.(harness.ToolWithdrawer); ok {
			if reason := withdrawer.ToolWithdrawn(); reason != "" {
				// Nothing has connected yet: the record begins again as a run
				// without the tool, and the person is told why.
				tool.keeper.begin(false, reason)
				_, _ = fmt.Fprintln(os.Stderr, "rewake: "+toolNote(reason))
			}
		}
		if observer, ok := plan.Backend.(harness.ObservedBackend); ok {
			stops = append(stops, sessionstate.Start(ctx, request.Dir, name, epoch, tool.keeper.source(epoch, observer.SessionState)))
			stated = true
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
				stops = append(stops, clock.Close)
				reporter.ReportTurns(tool.handler(clock.Snapshot, func(ctx context.Context, result harness.Completion) error {
					return request.OnTurn(ctx, reportSession, result)
				}))
			}
		}
		// Started before the harness, so its first hook finds the socket.
		// Telemetry that cannot start costs telemetry, never the session.
		// Closed either way: what the launch wrote beside the socket goes.
		stops = append(stops, plan.Observer.Close)
		if err := plan.Observer.Start(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "rewake: not collecting telemetry: "+err.Error())
		} else {
			stops = append(stops, sessionstate.Start(ctx, request.Dir, name, epoch, tool.keeper.source(epoch, plan.Observer.SessionState)))
			stated = true
		}
	}
	if !stated && tool.keeper != nil {
		// The channel record is shown whether or not anything else of the
		// session is observed: whoami prints it in every run.
		stops = append(stops, sessionstate.Start(ctx, request.Dir, name, epoch, tool.keeper.source(epoch, nil)))
	}

	if plan.Backend == nil && plan.Lane != nil {
		// Before the harness, like the telemetry socket: its answer to the
		// first notice goes to an address that has to exist by then.
		if err := plan.Lane.Start(ctx); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, "rewake: "+err.Error())
		}
		stops = append(stops, plan.Lane.Close)
	}
	return stop, nil
}
