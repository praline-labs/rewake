package modhost

import (
	"net/http"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
)

type observations struct {
	load       string
	generation int
	sequence   int
	localCount int
	count      uint64
	partial    bool
	// effort is the latest a mod saw the main loop ask for, kept across
	// loads: a reload forgets it until the next request.
	effort string
	// told says main was told of the dialog the snapshot names.
	told     bool
	snapshot session.Snapshot
	events   []session.CompactionEvent
	outcomes []session.CompactionOutcome
	// life is the last the status line told of the prompt cache; the cache
	// outlives a reload of the mod, so it is kept across loads (statusline.go).
	life *session.CacheLife
}

// currentGeneration excludes old loads, even though their durable turn ends
// may still be accepted by the outbox's separate protocol. Called under mu.
func (s *Server) currentGeneration(load string, generation int) (int, error) {
	switch {
	case s.closed:
		return http.StatusServiceUnavailable, errClosing
	case s.current == nil || s.current.hello.LoadID != load:
		return http.StatusConflict, errNoHello
	case !s.current.live || s.current.generation != generation:
		return http.StatusConflict, errLapsed
	}
	return 0, nil
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	var sample StateRequest
	if status, err := decode(w, r, &sample, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if sample.Sequence <= 0 || sample.Compactions < 0 || sample.Conversation == "" || (sample.Activity != "working" && sample.Activity != "idle") {
		refuse(w, http.StatusBadRequest, "a state names its conversation, sequence, activity and compaction count")
		return
	}
	if err := s.fromHarness(r.Context()); err != nil {
		s.refuseEffect(w, http.StatusForbidden, err)
		return
	}
	s.mu.Lock()
	if status, err := s.currentGeneration(sample.LoadID, sample.Generation); err != nil {
		s.mu.Unlock()
		s.refuseEffect(w, status, err)
		return
	}
	observed := &s.management.observed
	if observed.load != sample.LoadID {
		if observed.load != "" {
			observed.partial = true
		}
		observed.load, observed.sequence, observed.localCount = sample.LoadID, 0, 0
	}
	if sample.Sequence > observed.sequence {
		s.observeLocked(sample)
	}
	dialog := observed.snapshot.Dialog
	tell := dialog != nil && !observed.told && s.cfg.Dialog != nil
	s.mu.Unlock()
	if tell {
		s.tellDialog(*dialog)
	}
	s.answer(w, r, Done{})
}

// tellDialog tells main of the dialog a worker waits at, outside the lock: a
// letter that could not be left is tried again with the next sample, which
// the heartbeat brings while the dialog lasts.
func (s *Server) tellDialog(message string) {
	if s.cfg.Dialog(message) != nil {
		return
	}
	s.mu.Lock()
	if dialog := s.management.observed.snapshot.Dialog; dialog != nil && *dialog == message {
		s.management.observed.told = true
	}
	s.mu.Unlock()
}

func (s *Server) observeLocked(sample StateRequest) {
	observed := &s.management.observed
	now := time.Now()
	snapshot := session.Unknown(s.cfg.Epoch)
	snapshot.Thread, snapshot.Selection = sample.Conversation, "ready"
	snapshot.ObservedAt, snapshot.ActivityAt, snapshot.SettingsAt, snapshot.ContextAt = &now, &now, &now, &now
	snapshot.Fresh, snapshot.ActivityFresh, snapshot.SettingsFresh, snapshot.ContextFresh = true, true, true, true
	snapshot.Activity, snapshot.Model = &sample.Activity, sample.Model
	snapshot.Compacting = &sample.Compacting
	if sample.Effort != nil && *sample.Effort != "" {
		observed.effort = *sample.Effort
	}
	switch {
	case observed.effort != "":
		effort := observed.effort
		snapshot.Effort = &effort
	case s.cfg.Effort != "":
		effort := s.cfg.Effort
		snapshot.Effort, snapshot.EffortSource = &effort, session.EffortFromLaunch
	}
	snapshot.Interruptions = session.InterruptionsObserved
	if sample.CacheRead != nil && sample.CacheWrite != nil && sample.CacheInput != nil {
		snapshot.Cache = session.NewCache(int64(*sample.CacheRead), int64(*sample.CacheWrite), int64(*sample.CacheInput))
	}
	snapshot.ContextUsed, snapshot.ContextWindow = tokenCount(sample.Tokens), tokenCount(sample.Window)
	if snapshot.ContextWindow != nil && *snapshot.ContextWindow == 0 {
		snapshot.ContextWindow = nil
	}
	// The mod's window and percent are the model's; the window the launch
	// limited the conversation to is the limit the person means.
	if limit := s.cfg.Window; limit > 0 && (snapshot.ContextWindow == nil || limit < *snapshot.ContextWindow) {
		snapshot.ContextWindow = &limit
		if snapshot.ContextUsed != nil {
			percent := min(100, int((*snapshot.ContextUsed*100+limit/2)/limit))
			snapshot.FilledPercent = &percent
		}
	} else if snapshot.ContextUsed != nil && snapshot.ContextWindow != nil && sample.Percent != nil && *sample.Percent >= 0 && *sample.Percent <= 100 {
		snapshot.FilledPercent = sample.Percent
	}
	if sample.Compactions > observed.localCount {
		observed.count += uint64(sample.Compactions - observed.localCount)
		observed.events = append(observed.events, session.CompactionEvent{Sequence: observed.count, ObservedAt: now, Request: sample.Request, RequestedBy: sample.From})
		if len(observed.events) > 64 {
			observed.events = observed.events[len(observed.events)-64:]
		}
		observed.localCount = sample.Compactions
	}
	if sample.Dialog != nil && *sample.Dialog != "" {
		dialog := *sample.Dialog
		snapshot.Dialog = &dialog
	} else {
		observed.told = false
	}
	observed.snapshot, observed.sequence, observed.generation = snapshot, sample.Sequence, sample.Generation
}

func tokenCount(value *int) *int64 {
	if value == nil || *value < 0 {
		return nil
	}
	count := int64(*value)
	return &count
}

// SessionState reads only cached observations; a native call never blocks
// the wrapper's publisher. Heartbeat loss makes every observation stale.
func (s *Server) SessionState() session.Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	observed := &s.management.observed
	snapshot := observed.snapshot
	if snapshot.Selection == "" {
		snapshot = session.Unknown(s.cfg.Epoch)
	}
	if s.closed || s.current == nil || !s.current.live || s.current.hello.LoadID != observed.load || s.current.generation != observed.generation {
		snapshot.Stale()
	}
	if observed.load != "" {
		count := observed.count
		snapshot.Compactions = &count
		snapshot.Coverage = "observed"
		if observed.partial {
			snapshot.Coverage = "partial"
		}
	}
	if observed.life != nil {
		life := *observed.life
		snapshot.CacheLife = &life
	}
	snapshot.CompactionEvents = append([]session.CompactionEvent{}, observed.events...)
	snapshot.CompactionOutcomes = append([]session.CompactionOutcome{}, observed.outcomes...)
	return snapshot
}
