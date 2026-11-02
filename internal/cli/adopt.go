package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/host"
	"github.com/praline-labs/rewake/internal/host/switching"
)

// adoptRun is a wrapper's new image after the exec of host/reexec.go: it runs
// the launch the handover carries, with the callbacks handleLaunch gives one,
// and takes the room's build lease again in host.Run, the earlier image's
// having gone with it. Its signals are caught before anything else
// (host.CatchSignals): the harness runs on through the exec.
func adoptRun(catalog adapter.Catalog, spec string, stderr io.Writer) int {
	host.CatchSignals()
	_ = os.Unsetenv(host.HandoverEnv)
	handover, err := host.ReadHandover(spec)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rewake: "+err.Error())
		return ExitFailed
	}
	harness, ok := catalog.Find(handover.Harness)
	if !ok {
		_, _ = fmt.Fprintf(stderr, "rewake: the run handed over is of harness %s, which this build does not have\n", handover.Harness)
		return ExitFailed
	}
	if handover.Park != nil {
		if code, done := unpark(handover, stderr); done {
			return code
		}
	}
	dir := handover.Dir
	request := handover.Request(harness)
	request.OnTurn = func(ctx context.Context, self registry.Session, result adapter.TurnEnd) (mail.Outcome, error) {
		return ReportCompletion(ctx, dir, self, result)
	}
	request.OnConfirm = func(ctx context.Context, self registry.Session, result adapter.TurnEnd) (mail.Outcome, error) {
		return ConfirmCompletion(ctx, dir, self, result)
	}
	code, err := host.Run(context.Background(), request)
	// Looked up once the harness is gone, as the launch settles it: a line
	// said during the run would land inside a worker's screen.
	if checkout := adoptedCheckout(handover, harness, stderr); checkout != nil {
		checkout.settle(err != nil || code != 0)
	}
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rewake: the run handed over could not be adopted: "+err.Error())
		return ExitFailed
	}
	return code
}

// unpark is a parked image of a switch (host/switching): it holds no lease
// and runs nothing until its leader, or the room's build when the leader is
// gone, decides. Go execs the staged copy of the target with the run; resume
// goes on here, done false, and adopts the run on this build.
func unpark(handover *host.Handover, stderr io.Writer) (code int, done bool) {
	park := *handover.Park
	// Nothing of the run is served while parked, and a signal that ended the
	// process here would orphan the harness and leave the run unadopted:
	// adoptRun caught the signals, and the run forwards a SIGTERM that came
	// while parked once it adopts the harness, here or, carried by Continue,
	// in the target. The wait is bounded by the drop's deadline.
	decided, err := switching.Wait(park, "session "+handover.Name)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rewake: the run parked for a switch was lost: "+err.Error())
		return ExitFailed, true
	}
	if decided == switching.Go {
		err := handover.Continue(park.Staged)
		_, _ = fmt.Fprintln(stderr, "rewake: the run parked for a switch could not exec the new build: "+err.Error())
		return ExitFailed, true
	}
	handover.Park = nil
	return 0, false
}
