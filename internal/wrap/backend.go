package wrap

import (
	"context"
	"os"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/proc"
)

// stopWithBackend ends the harness when its backend is gone. ending is told
// first: the wrapper's own teardown has begun, and the mail tool, which the
// backend hosted, goes with it rather than failing.
func stopWithBackend(ctx context.Context, backend harness.Backend, child *os.Process, start uint64, ending func()) {
	select {
	case <-ctx.Done():
		return
	case <-backend.Done():
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
