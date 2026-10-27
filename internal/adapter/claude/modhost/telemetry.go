package modhost

import (
	"net/http"

	"github.com/praline-labs/rewake/internal/core/session"
)

// observations is what the host keeps of the mod's samples beside the
// session's observer: the wire's own order — the load, its generation and
// the last sequence —, the conversation and the dialog the last sample named,
// and whether main was told of it.
type observations struct {
	// view merges every observation into the session's snapshot; New makes
	// it from the launch's effort and window.
	view       *session.Observer
	load       string
	generation int
	sequence   int
	thread     string
	dialog     *string
	// told says main was told of the dialog the last sample names.
	told bool
}

// newObservations is a run's observations, nothing seen yet beside what its
// launch passed.
func newObservations(cfg Config) observations {
	return observations{view: session.NewObserver(session.ObserverConfig{Epoch: cfg.Epoch, Effort: cfg.Effort, Window: cfg.Window})}
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
		observed.load, observed.sequence = sample.LoadID, 0
	}
	if sample.Sequence > observed.sequence {
		s.observeLocked(sample)
	}
	dialog := observed.dialog
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
	if dialog := s.management.observed.dialog; dialog != nil && *dialog == message {
		s.management.observed.told = true
	}
	s.mu.Unlock()
}

func (s *Server) observeLocked(sample StateRequest) {
	observed := &s.management.observed
	if sample.Effort != nil {
		observed.view.Effort("request", *sample.Effort)
	}
	observed.view.Compacted(sample.LoadID, uint64(sample.Compactions), sample.From, sample.Request)
	var dialog, waiting string
	observed.dialog = nil
	if sample.Dialog != nil && *sample.Dialog != "" {
		dialog = *sample.Dialog
		observed.dialog = &dialog
	} else {
		observed.told = false
	}
	if sample.Waiting != nil {
		waiting = string(*sample.Waiting)
	}
	compacting := sample.Compacting
	var cache *session.Cache
	if sample.CacheRead != nil && sample.CacheWrite != nil && sample.CacheInput != nil {
		cache = session.NewCache(int64(*sample.CacheRead), int64(*sample.CacheWrite), int64(*sample.CacheInput))
	}
	observed.view.Sample(session.Sample{
		Thread: sample.Conversation, Activity: sample.Activity, Compacting: &compacting, Cache: cache,
		Context:  &session.Context{Used: tokenCount(sample.Tokens), Window: tokenCount(sample.Window), Percent: sample.Percent},
		Settings: &session.Settings{Model: sample.Model},
		Dialog:   dialog, Waiting: waiting,
	})
	observed.thread, observed.sequence, observed.generation = sample.Conversation, sample.Sequence, sample.Generation
}

func tokenCount(value *int) *int64 {
	if value == nil {
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
	live := !s.closed && s.current != nil && s.current.live && s.current.hello.LoadID == observed.load && s.current.generation == observed.generation
	return observed.view.Snapshot(session.Reading{Live: live, Counted: true})
}
