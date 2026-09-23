package workflow

import (
	"errors"
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

// shimOwedFile makes a session run `rewake inbox --owed` in its first turn,
// after reading its mail and before the turn ends — a worker re-reading its
// task after a compaction — and write what that printed to this file: the
// machine form, a separator line, then the plain form.
const shimOwedFile = "RW_SHIM_OWED_FILE"

// owedSeparator divides the two forms in the file.
const owedSeparator = "\n--- plain ---\n"

var owedOnce sync.Once

// recordOwedOnce is called by either fixture just before a turn ends.
func recordOwedOnce() {
	target := os.Getenv(shimOwedFile)
	if target == "" {
		return
	}
	owedOnce.Do(func() {
		machine, errMachine := exec.Command("rewake", "inbox", "--owed", "--json").CombinedOutput()
		plain, errPlain := exec.Command("rewake", "inbox", "--owed").CombinedOutput()
		record := string(machine) + owedSeparator + string(plain)
		if err := errors.Join(errMachine, errPlain); err != nil {
			record = "refused: " + err.Error() + "\n" + record
		}
		_ = os.WriteFile(target, []byte(record), 0o600)
	})
}
