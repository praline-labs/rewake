package pi

import (
	"context"
	"math"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
)

// A sample's sequence orders event pushes and answers to polls on one
// connection. A late poll answer cannot replace a newer pushed observation.
type sample struct {
	Sequence   uint64   `json:"sequence"`
	Activity   string   `json:"activity"`
	Model      *string  `json:"model"`
	ModelName  *string  `json:"modelName"`
	Effort     *string  `json:"effort"`
	Tokens     *int64   `json:"tokens"`
	Window     *int64   `json:"window"`
	Percent    *float64 `json:"percent"`
	Compacting bool     `json:"compacting"`
}

type observations struct {
	link     *link
	sequence uint64
	snapshot session.Snapshot
	count    uint64
	events   []session.CompactionEvent
	outcomes []session.CompactionOutcome
	partial  bool
}

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
	if value.Activity != "working" && value.Activity != "idle" {
		return false
	}
	now := time.Now()
	snapshot := session.Unknown(r.epoch)
	snapshot.Thread, snapshot.Selection = thread, "ready"
	snapshot.ObservedAt, snapshot.ActivityAt, snapshot.SettingsAt, snapshot.ContextAt = &now, &now, &now, &now
	snapshot.Fresh, snapshot.ActivityFresh, snapshot.SettingsFresh, snapshot.ContextFresh = true, true, true, true
	snapshot.Activity, snapshot.Model, snapshot.Effort = &value.Activity, value.Model, value.Effort
	if value.ModelName != nil && *value.ModelName != "" {
		snapshot.ModelName = value.ModelName
	}
	snapshot.Compacting = &value.Compacting
	snapshot.Interruptions = session.InterruptionsObserved
	if value.Window != nil && *value.Window > 0 {
		snapshot.ContextWindow = value.Window
	}
	if value.Tokens != nil && *value.Tokens >= 0 {
		snapshot.ContextUsed = value.Tokens
	}
	if snapshot.ContextUsed != nil && snapshot.ContextWindow != nil && value.Percent != nil && *value.Percent >= 0 && !math.IsNaN(*value.Percent) && !math.IsInf(*value.Percent, 0) {
		percent := int(math.Round(math.Min(100, *value.Percent)))
		snapshot.FilledPercent = &percent
	}
	r.observed.link, r.observed.sequence, r.observed.snapshot = l, value.Sequence, snapshot
	return true
}

// SessionState only copies cached observations. The host publishes four times
// a second; a stalled native runtime must not stall that publisher.
func (r *run) SessionState() session.Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := r.observed.snapshot
	if snapshot.Epoch == "" {
		snapshot.Epoch = r.epoch
	}
	if snapshot.Selection == "" {
		snapshot.Selection = "unknown"
	}
	if r.current == nil || r.current.gone() || r.current != r.observed.link || snapshot.ObservedAt == nil || time.Since(*snapshot.ObservedAt) > 3*time.Second {
		snapshot.Stale()
	}
	if r.observed.link != nil {
		count := r.observed.count
		snapshot.Compactions = &count
		snapshot.Coverage = "observed"
		if r.observed.partial {
			snapshot.Coverage = "partial"
		}
	}
	snapshot.CompactionEvents = append([]session.CompactionEvent{}, r.observed.events...)
	snapshot.CompactionOutcomes = append([]session.CompactionOutcome{}, r.observed.outcomes...)
	return snapshot
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
