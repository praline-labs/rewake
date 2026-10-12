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

// SetDraws makes the ids a clearing draws for its candidate those given, in
// order, for one test.
func SetDraws(t testing.TB, ids ...string) {
	old := drawTreeID
	drawTreeID = func(proc.Reader) string {
		id := ids[0]
		ids = ids[1:]
		return id
	}
	t.Cleanup(func() { drawTreeID = old })
}

// NewTreeID draws an id as a clearing does.
func NewTreeID() string { return newTreeID(procReader) }

// Count is the counter over one tree.
var Count = count
