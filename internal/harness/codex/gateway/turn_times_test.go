package gateway

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
)

// A completion carries when its turn started and ended on the boot clock, so a
// `rewake pending` mark made in a later turn cannot be taken by this one's
// late publication. A turn whose start was not seen carries no start.
func TestACompletionCarriesItsTurnsTimes(t *testing.T) {
	now := time.Now()
	o := newObserver()
	o.bind("A", "idle", now)
	before := boottime.Now()
	o.event(meta{method: "turn/started", thread: "A", turn: "1"}, nil, now)
	o.event(meta{method: "turn/completed", thread: "A", turn: "1", status: "completed"}, nil, now)
	after := boottime.Now()
	out := o.drain()
	if len(out) != 1 {
		t.Fatalf("completions %+v", out)
	}
	if c := out[0]; c.Started < before || c.Ended < c.Started || c.Ended > after {
		t.Errorf("times %d..%d, want within %d..%d", c.Started, c.Ended, before, after)
	}

	o.event(meta{method: "thread/status/changed", thread: "A", status: "active"}, nil, now)
	o.event(meta{method: "turn/completed", thread: "A", turn: "2", status: "completed"}, nil, now)
	if out := o.drain(); len(out) == 1 && (out[0].Started != 0 || out[0].Ended == 0) {
		t.Errorf("a turn with no start seen carries %d..%d", out[0].Started, out[0].Ended)
	}
}
