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
	o.event(meta{method: "thread/status/changed", thread: "A", status: "active"}, nil, now.Add(time.Millisecond))
	o.event(meta{method: "turn/started", thread: "A", turn: "compact"}, nil, now.Add(2*time.Millisecond))
	// No reply names the compaction's turn and it has no other item, so the
	// connection keeps its outcome; no model task has run on this observer
	// before.
	o.event(meta{method: "thread/status/changed", thread: "A", status: "idle"}, nil, now.Add(3*time.Millisecond))
	o.event(meta{method: "turn/completed", thread: "A", turn: "compact", status: "completed"}, nil, now.Add(4*time.Millisecond))
	c := &connection{admitted: newAdmittedWork()}
	if out := c.proven(o.drain(), time.Now()); len(out) != 0 {
		t.Fatalf("first manual compaction became a task result: %+v", out)
	}
}
