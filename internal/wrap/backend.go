package wrap

import (
	"context"
	"os"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// stopWithProcess ends the harness when the process that speaks for it is
// gone. ending is told first: the wrapper's own teardown has begun, and the
// mail tool, which that process hosted, goes with it rather than failing.
func stopWithProcess(ctx context.Context, process adapter.ProcessService, child *os.Process, start uint64, ending func()) {
	select {
	case <-ctx.Done():
		return
	case <-process.Done():
	}
	if !proc.Alive(child.Pid, start) {
		return
	}
	if ending != nil {
		ending()
	}
	_ = child.Signal(syscall.SIGTERM)
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	if proc.Alive(child.Pid, start) {
		_ = child.Kill()
	}
}
