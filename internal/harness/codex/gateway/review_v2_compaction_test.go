package gateway

import (
	"testing"
	"time"
)

func TestReviewV2ManualCompactionAfterIdleBeforeFirstObservedTurn(t *testing.T) {
	now := time.Now()
	o := newObserver()
	o.event(meta{method: "thread/status/changed", thread: "A", status: "idle"}, nil, now)
	o.bind("A", "idle", now)
	// The gateway records a manual marker on request admission; no model task has run on this observer yet.
	o.manual["A"] = true
	o.event(meta{method: "thread/status/changed", thread: "A", status: "active"}, nil, now.Add(time.Millisecond))
	o.event(meta{method: "turn/started", thread: "A", turn: "compact"}, nil, now.Add(2*time.Millisecond))
	o.event(meta{method: "thread/status/changed", thread: "A", status: "idle"}, nil, now.Add(3*time.Millisecond))
	o.event(meta{method: "turn/completed", thread: "A", turn: "compact", status: "completed"}, nil, now.Add(4*time.Millisecond))
	if out := o.drain(); len(out) != 0 {
		t.Fatalf("first manual compaction became a task result: %+v", out)
	}
}
