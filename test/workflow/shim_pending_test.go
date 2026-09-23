package workflow

import (
	"os"
	"os/exec"
	"sync"
)

// shimPendingOnce makes a session run `rewake pending <text>` in its first
// turn, before that turn ends: the worker that ends a turn while background
// work goes on. What rewake answered is recorded beside the session's turns.
const shimPendingOnce = "RW_SHIM_PENDING_ONCE"

var pendingOnce sync.Once

// markPendingOnce is called by either fixture just before a turn ends.
func markPendingOnce() {
	text := os.Getenv(shimPendingOnce)
	if text == "" {
		return
	}
	pendingOnce.Do(func() {
		out, err := exec.Command("rewake", "pending", text).CombinedOutput()
		line := "pending ok"
		if err != nil {
			line = "pending refused: " + err.Error() + ": " + string(out)
		}
		(&shimSession{}).recordTurn(line)
	})
}
