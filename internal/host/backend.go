package host

import (
	"context"
	"os"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// stopWithProcess ends the harness when the process that speaks for it is
// gone.
func stopWithProcess(ctx context.Context, process adapter.ProcessService, child *os.Process, start uint64) {
	select {
	case <-ctx.Done():
		return
	case <-process.Done():
	}
	if !proc.Alive(child.Pid, start) {
		return
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
