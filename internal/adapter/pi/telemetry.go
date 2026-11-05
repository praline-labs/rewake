package pi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
)

// A sample's sequence orders event pushes and answers to polls on one
// connection. A late poll answer cannot replace a newer pushed observation.
// Native is what the session read of itself, as pi answered it (native.go).
type sample struct {
	Sequence   uint64          `json:"sequence"`
	Idle       bool            `json:"idle"`
	Compacting bool            `json:"compacting"`
	Native     json.RawMessage `json:"native,omitempty"`
}

// observations is what the run keeps of its samples beside the session's
// observer: the connection the last came on and its sequence, by which a
// late answer to a poll cannot replace a newer pushed sample.
type observations struct {
	view     *session.Observer
	link     *link
	sequence uint64
	// cache is the last assistant message's cache usage that had input.
	cache *cacheSeen
	// model is the last sample's model, whose levels the face's Alt+T
	// cycles (levelsOf).
	model *nativeModel
}

// sampleExpiry is how long a sample stays fresh: the run polls every second.
const sampleExpiry = 3 * time.Second

func (r *run) observe(l *link, thread string, value *sample) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.observeLocked(l, thread, value)
}

func (r *run) observeLocked(l *link, thread string, value *sample) bool {
	if value == nil || value.Sequence == 0 || l != r.current || thread != r.thread {
		return false
	}
	if r.observed.link == l && value.Sequence <= r.observed.sequence {
		return false
	}
	activity := "working"
	if value.Idle {
		activity = "idle"
	}
	compacting := value.Compacting
	observed := session.Sample{Thread: thread, Activity: activity, Compacting: &compacting}
	r.projectLocked(thread, value.Native, time.Now(), &observed)
	r.observed.view.Sample(observed)
	r.observed.link, r.observed.sequence = l, value.Sequence
	return true
}

// SessionState only copies cached observations. The host publishes four times
// a second; a stalled native runtime must not stall that publisher.
func (r *run) SessionState() session.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	live := r.current != nil && !r.current.gone() && r.current == r.observed.link
	return r.observed.view.Snapshot(session.Reading{Live: live, Counted: true})
}

func (r *run) pollState(l *link, thread string) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-l.closed:
			return
		case <-r.ctx.Done():
			return
		case <-ticker.C:
		}
		if !r.on(l) {
			return
		}
		ctx, cancel := context.WithTimeout(r.ctx, time.Second)
		answer, err := l.ask(ctx, frame{Op: "state", Thread: thread})
		cancel()
		if err == nil && answer.OK {
			r.observe(l, thread, answer.State)
		}
	}
}
