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
)

// adoptRun is a wrapper's new image after the exec of host/reexec.go: it runs
// the launch the handover carries, with the callbacks handleLaunch gives one,
// and takes the room's build lease again in host.Run, the earlier image's
// having gone with it.
func adoptRun(catalog adapter.Catalog, spec string, stderr io.Writer) int {
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
	dir := handover.Dir
	request := handover.Request(harness)
	request.OnTurn = func(ctx context.Context, self registry.Session, result adapter.TurnEnd) (mail.Outcome, error) {
		return ReportCompletion(ctx, dir, self, result)
	}
	request.OnConfirm = func(ctx context.Context, self registry.Session, result adapter.TurnEnd) (mail.Outcome, error) {
		return ConfirmCompletion(ctx, dir, self, result)
	}
	code, err := host.Run(context.Background(), request)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "rewake: the run handed over could not be adopted: "+err.Error())
		return ExitFailed
	}
	return code
}
