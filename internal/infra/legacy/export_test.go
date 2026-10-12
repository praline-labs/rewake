package legacy

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/proc"
)

// SetProc points the clearing at a /proc-shaped tree for one test.
func SetProc(t testing.TB, root string) {
	old := procReader
	procReader = proc.Reader{Root: root}
	t.Cleanup(func() { procReader = old })
}

// SetWaits shortens the clearing's bounds for one test.
func SetWaits(t testing.TB, clearing, room time.Duration) {
	oldClear, oldRoom := clearWait, roomWait
	clearWait, roomWait = clearing, room
	t.Cleanup(func() { clearWait, roomWait = oldClear, oldRoom })
}

// Waits are the clearing's bounds as built.
func Waits() (time.Duration, time.Duration) { return clearWait, roomWait }

// Count is the counter over one tree.
var Count = count
