package gateway

import (
	"crypto/sha256"
	"sync"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

const (
	maxCompactionIDs      = 4096
	maxStagedCompactions  = 64
	maxObservationThreads = 8
	maxCompactionEvents   = 64
)

type telemetryRun struct {
	mu      sync.Mutex
	seen    map[[32]byte]string
	count   uint64
	partial bool
	last    sessionstate.Snapshot
	binding Binding
	events  []sessionstate.CompactionEvent
	// counted is the event of each turn's compaction, by thread and turn,
	// until the turn ends and its asker, if any, is known.
	counted map[string]uint64
	// outcomes are the ends of main's compactions its command answered
	// before, for main's wrapper to send as letters.
	outcomes []sessionstate.CompactionOutcome
}

type threadObservation struct {
	snapshot                        sessionstate.Snapshot
	activityVersion, activitySerial uint64
	active, activeTurn              string
	settingsFence, confirmedSerial  uint64
	confirmedFresh, contextValid    bool
	proposals                       map[uint64]struct{}
	proposalOverflow                bool
	priorAwait                      bool
	priorTurn                       string
	awaitingNewTurn                 bool
	usageTurn                       string
}

type compactionObservation struct {
	// request and by are set on the end of the turn of a compaction a main
	// asked for with rewake compact: main's wrapper sends no notice of it,
	// and the command reads its count by the request.
	request, by         string
	thread, item, turn  string
	observedAt          time.Time
	completed, terminal bool
}

type connectionObservations struct {
	threads    map[string]*threadObservation
	staged     []compactionObservation
	hadPrimary bool
	partial    bool
	serial     uint64
}

// SessionState has no I/O and never promotes an unproved connection or selection.
func (g *Gateway) SessionState() sessionstate.Snapshot {
	binding := g.Binding()
	g.telemetry.mu.Lock()
	defer g.telemetry.mu.Unlock()
	result := g.telemetry.last
	if result.Epoch == "" {
		result = sessionstate.Unknown(g.cfg.Epoch)
	}
	count := g.telemetry.count
	result.Compactions = &count
	result.CompactionEvents = append([]sessionstate.CompactionEvent(nil), g.telemetry.events...)
	result.CompactionOutcomes = append([]sessionstate.CompactionOutcome(nil), g.telemetry.outcomes...)
	result.Coverage = "observed"
	if g.telemetry.partial {
		result.Coverage = "partial"
	}
	if !binding.Ready || !sameBinding(binding, g.telemetry.binding) {
		result.Selection = "unavailable"
		result.Stale()
	}
	return result
}

// Call under the connection lock, following the existing connection-to-owner lock order.
func (c *connection) observationThread(thread string) *threadObservation {
	if thread == "" {
		return nil
	}
	allowed := c.state.Ready && thread == c.state.Thread
	if f := c.state.fork; f != nil {
		allowed = thread == f.parent || thread == f.child || f.child == ""
	}
	if !allowed {
		for _, p := range c.state.pending {
			if p.intent && (p.target == "" || p.target == thread) {
				allowed = true
				break
			}
		}
	}
	if !allowed || !c.owner.owns(c) {
		return nil
	}
	return c.observationEntry(thread)
}

func (c *connection) observationEntry(thread string) *threadObservation {
	if c.observations.threads == nil {
		c.observations.threads = map[string]*threadObservation{}
	}
	if existing := c.observations.threads[thread]; existing != nil {
		return existing
	}
	if len(c.observations.threads) >= maxObservationThreads {
		c.observations.partial = true
		accepted := c.state.Ready && c.state.Thread == thread
		child := c.state.fork != nil && c.state.fork.child == thread
		if !accepted && !child {
			return nil
		}
		for candidate := range c.observations.threads {
			if child && candidate == c.state.fork.parent {
				continue
			}
			delete(c.observations.threads, candidate)
		}
	}
	entry := &threadObservation{snapshot: sessionstate.Unknown(c.owner.cfg.Epoch)}
	entry.snapshot.Thread = thread
	c.observations.threads[thread] = entry
	return entry
}

func (c *connection) syncObservation() {
	if !c.owner.owns(c) {
		return
	}
	run := &c.owner.telemetry
	entry := (*threadObservation)(nil)
	if c.state.Ready {
		entry = c.observationEntry(c.state.Thread)
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	run.partial = run.partial || c.observations.partial
	if !c.state.Ready {
		run.last.Stale()
		run.last.Selection = "unavailable"
		if c.observations.hadPrimary && c.state.fork == nil && c.state.Reason != "primary intent pending" {
			run.partial = true
		}
		return
	}
	if entry == nil {
		run.last = sessionstate.Unknown(c.owner.cfg.Epoch)
		run.partial = true
		return
	}
	for _, event := range c.observations.staged {
		if event.thread == c.state.Thread {
			run.compaction(entry, event)
		}
	}
	c.observations.staged = nil
	c.observations.hadPrimary = true
	value := entry.snapshot
	value.Epoch = c.owner.cfg.Epoch
	value.Selection = "ready"
	value.Fresh = true
	active := entry.active != ""
	value.Compacting = &active
	run.last = value
	run.binding = c.state.Binding
	// Keep only the accepted primary and an unresolved fork candidate, never a
	// growing cache of old dialogs. Resume waits for new native observations.
	for thread := range c.observations.threads {
		if thread != c.state.Thread {
			delete(c.observations.threads, thread)
		}
	}
}

// CompactionEnded keeps how a compaction a main asked for ended, for that
// main's wrapper to send as a letter: its end, once its command has answered
// started or requested, and every final answer, which the letter needs when
// the command's own wait was cut short.
//
// A started answer is published after the connection's lock is let go, so the
// compaction's end, recorded under that lock, can come first: a started that
// arrives after a final outcome of the same request is dropped, or the
// letter would take the compaction for still running.
func (g *Gateway) CompactionEnded(request, by string, answer control.Answer) {
	g.telemetry.mu.Lock()
	defer g.telemetry.mu.Unlock()
	if answer.Outcome == control.Started {
		for _, kept := range g.telemetry.outcomes {
			if kept.Request == request && kept.RequestedBy == by && kept.Outcome != control.Started {
				return
			}
		}
	}
	g.telemetry.outcomes = sessionstate.KeepOutcome(g.telemetry.outcomes, sessionstate.CompactionOutcome{
		Request: request, RequestedBy: by, Outcome: answer.Outcome, Reason: answer.Reason, Detail: answer.Detail,
		TokensBefore: answer.TokensBefore, TokensAfter: answer.TokensAfter, EndedAt: time.Now(),
	})
}

func (r *telemetryRun) compaction(entry *threadObservation, event compactionObservation) {
	key := event.thread + "\x00" + event.turn
	if event.terminal {
		if entry.activeTurn == event.turn {
			entry.active = ""
			entry.activeTurn = ""
		}
		// The asker is written only now, at the end of the turn the mark was
		// still tied to: a tie undone before it leaves nothing to take back.
		if sequence, ok := r.counted[key]; ok && event.by != "" {
			for i := range r.events {
				if r.events[i].Sequence == sequence {
					r.events[i].RequestedBy, r.events[i].Request = event.by, event.request
				}
			}
		}
		delete(r.counted, key)
		return
	}
	identity := sha256.Sum256([]byte(event.thread + "\x00" + event.item))
	if r.seen[identity] != "" {
		return
	}
	if !event.completed {
		entry.active, entry.activeTurn = event.item, event.turn
		return
	}
	if entry.active == event.item && entry.activeTurn == event.turn {
		entry.active = ""
		entry.activeTurn = ""
	}
	if r.seen == nil {
		r.seen = map[[32]byte]string{}
	}
	if len(r.seen) >= maxCompactionIDs {
		r.partial = true
		return
	}
	r.seen[identity] = event.turn
	r.count++
	if !event.observedAt.IsZero() {
		if len(r.events) == maxCompactionEvents {
			copy(r.events, r.events[1:])
			r.events = r.events[:maxCompactionEvents-1]
		}
		r.events = append(r.events, sessionstate.CompactionEvent{Sequence: r.count, ObservedAt: event.observedAt})
		if r.counted == nil || len(r.counted) >= maxCompactionEvents {
			r.counted = map[string]uint64{}
		}
		r.counted[key] = r.count
	}
}

func (c *connection) observeCompaction(thread, item, turn string, completed bool) {
	entry := c.observationThread(thread)
	if entry == nil {
		return
	}
	if item == "" || turn == "" {
		c.owner.telemetry.mu.Lock()
		c.owner.telemetry.partial = true
		c.owner.telemetry.mu.Unlock()
		return
	}
	now := observedNow(&entry.snapshot)
	c.recordCompaction(entry, compactionObservation{observedAt: now, thread: thread, item: item, turn: turn, completed: completed})
}

func (c *connection) recordCompaction(entry *threadObservation, event compactionObservation) {
	thread := event.thread
	run := &c.owner.telemetry
	primary := c.state.Ready && c.state.Thread == thread
	if f := c.state.fork; f != nil && !f.startup && f.parentLive && f.parent == thread {
		primary = true
	}
	switch {
	case primary:
		run.mu.Lock()
		run.compaction(entry, event)
		run.mu.Unlock()
	case len(c.observations.staged) < maxStagedCompactions:
		c.observations.staged = append(c.observations.staged, event)
	default:
		run.mu.Lock()
		run.partial = true
		run.mu.Unlock()
	}
}

func (c *connection) observationClosed() {
	if !c.observations.hadPrimary && !c.state.observing() {
		return
	}
	run := &c.owner.telemetry
	run.mu.Lock()
	run.partial = true
	run.mu.Unlock()
}

func (c *connection) observeRequest(m meta) {
	if recognized(m) || m.startupFork {
		// A new primary intent must not recycle a prior measurement as fresh.
		c.observations.threads = map[string]*threadObservation{}
		c.observations.staged = nil
	}
	c.observeSettingsRequest(m)
	c.syncObservation()
}

func observedNow(snapshot *sessionstate.Snapshot) time.Time {
	now := time.Now()
	snapshot.ObservedAt = &now
	return now
}
