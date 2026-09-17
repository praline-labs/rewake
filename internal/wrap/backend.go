package wrap

import (
	"context"
	"os"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/proc"
)

func stopWithBackend(ctx context.Context, backend harness.Backend, child *os.Process, start uint64) {
	select {
	case <-ctx.Done():
		return
	case <-backend.Done():
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
