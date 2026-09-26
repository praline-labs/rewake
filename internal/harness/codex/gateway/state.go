package gateway

import (
	"errors"
	"strings"
	"time"
)

// Binding rejects stale work even when A-B-A returns to the same thread ID.
type Binding struct {
	Epoch      string `json:"epoch"`
	Connection uint64 `json:"connection"`
	Generation uint64 `json:"generation"`
	Thread     string `json:"thread"`
	Ready      bool   `json:"ready"`
	Reason     string `json:"reason"`
}
type (
	pending struct {
		observationGeneration, observationFence, observationSerial uint64
		observationActivityVersion                                 uint64
		metadataOnly                                               bool
		sideSetup, sideDetach                                      bool
		fork, forkDetach                                           bool
		reconnect                                                  bool
		generation                                                 uint64
		readSerial                                                 uint64
		intent                                                     bool
		backfill                                                   bool
		target, method                                             string
		// sent is the request's place in the order of writes.
		sent uint64
	}
	state struct {
		side string
		fork *forkSelection
		Binding
		pending     map[string]pending
		initialized bool
		events      observer
		canBackfill bool
		// fresh says the selected thread was started on this connection and
		// has run no turn since: there is nothing to compact.
		fresh bool
		doing activity
		// ops outlive a selection and the connection, unlike the rest: the
		// gateway's, shared by its connections (operations).
		ops          *operations
		readSerial   uint64
		readClosedBy string
		backfill     map[string]bool
	}
)

func newState(epoch string, conn uint64) state {
	return state{Binding: Binding{Epoch: epoch, Connection: conn, Reason: "waiting for recognized primary intent"}, pending: map[string]pending{}, events: newObserver(), ops: newOperations()}
}

func (s *state) invalidate(reason string) {
	s.fork = nil
	s.side = ""
	s.Generation++
	s.Ready = false
	s.Thread = ""
	s.fresh = false
	s.doing = activity{}
	s.Reason = reason
	s.events.reset()
	s.closeReadContext("selection fence invalidated")
}

func isHelper(m meta) bool {
	return strings.HasPrefix(m.idText, "tui-dynamic-") || strings.HasPrefix(m.idText, "temporary-")
}

// recognized says a lifecycle request is the terminal's own selection. The
// roots or the permissions marked it until 0.155.1; 0.157.1 sends neither, and
// its terminal's configuration marks it instead (tuiConfig) — for a resume
// only in the shape of a resume by id, the history and the path left out, as
// the terminal sends one. A resume carrying the configuration's defaults only
// is the terminal rejoining a thread, for a reconnect or to attach a helper:
// that is a selection only as the reconnect a gateway correlated (m.reconnect).
func recognized(m meta) bool {
	switch m.method {
	case "thread/start":
		// legacy(codex <0.157.1): 0.155.1 marks its start by the roots or the permissions, later versions by tuiConfig only; remove when 0.155.1 is no longer supported
		return (m.numeric || strings.HasPrefix(m.idText, "startup-thread-start-")) && m.source == "user" && (m.roots || m.permissions || m.tuiConfig)
	case "thread/resume":
		// legacy(codex <0.157.1): the (roots || permissions) && config arm is 0.155.1's ordinary resume; remove when 0.155.1 is no longer supported
		return m.numeric && m.thread != "" && ((m.roots || m.permissions) && m.config || m.tuiConfig && m.byID || m.reconnect)
	}
	return false
}

func (s *state) request(m meta) error {
	if m.id == "" {
		return nil
	}
	if _, ok := s.pending[m.id]; ok {
		return errors.New("duplicate outstanding TUI request id")
	}
	if len(s.pending) >= 256 {
		return errors.New("pending TUI request limit")
	}
	if nativeReadBoundary(m) {
		s.closeReadContext("native " + m.method)
	}
	p := pending{metadataOnly: m.method == "thread/read" && metadataRead(m), method: m.method, target: m.thread, readSerial: s.readSerial, reconnect: m.reconnect}
	p.sent = m.sent
	if m.method == "review/start" || isTurnAdmission(m.method) {
		// Only a reply of the current selection names its running turn.
		p.generation = s.Generation
	}
	if m.method == "thread/loaded/list" && m.numeric && s.canBackfill && s.Ready {
		p.backfill = true
		p.generation = s.Generation
		s.canBackfill = false
	}
	if s.sideRequest(m, &p) || s.forkRequest(m, &p) {
		s.pending[m.id] = p
		return nil
	}
	if recognized(m) {
		s.invalidate("primary intent pending")
		p.intent = true
		p.generation = s.Generation
		p.readSerial = s.readSerial
		p.target = m.thread
	} else if !isHelper(m) {
		switch m.method {
		case "thread/start", "thread/resume", "thread/fork":
			s.invalidate("ambiguous lifecycle request; select a conversation with /resume or /new")
		case "thread/read":
			switch s.readContext(m) {
			case "accepted-thread", "overview":
			case "resume-backfill":
				delete(s.backfill, m.thread)
			default:
				s.invalidate("unclassified read; selection not proved, delivery waits for an explicit /resume or /new")
			}
		case "turn/start", "turn/steer":
			if m.thread != "" && m.thread != s.Thread {
				s.invalidate("unexplained different-thread activity; select a conversation with /resume or /new")
			}
		case "thread/archive", "thread/unsubscribe":
			if m.thread == s.Thread {
				s.invalidate("primary detached; select a conversation with /resume or /new")
			}
		default:
			// Unreviewed thread control operations cannot silently preserve an assumed target.
			if strings.HasPrefix(m.method, "thread/") && !knownNonSelection(m.method) {
				s.invalidate("unclassified thread operation; select a conversation with /resume or /new")
			}
		}
	}
	s.pending[m.id] = p
	return nil
}

func knownNonSelection(method string) bool {
	switch method {
	case "thread/settings/update", "thread/metadata/update", "thread/memoryMode/set",
		"thread/list", "thread/loaded/list", "thread/name/set", "thread/turns/list", "thread/items/list", "thread/compact/start", "thread/goal/get", "thread/goal/set", "thread/goal/clear":
		return true
	}
	return false
}

func (s *state) response(m meta, raw ...[]byte) meta {
	p, ok := s.pending[m.id]
	if !ok {
		return m
	}
	delete(s.pending, m.id)
	if p.sideDetach && p.generation == s.Generation && detached(m) && p.target == s.side {
		s.side = ""
	}
	if p.fork || p.forkDetach || p.sideSetup {
		s.forkResponse(m, p)
		return m
	}
	if p.method == "thread/loaded/list" && len(raw) > 0 {
		m.loaded, m.loadedValid = loadedIDs(raw[0])
		s.acceptBackfill(m, p)
	}

	if p.method == "initialize" && !m.failure {
		s.initialized = true
	}
	if !p.intent || p.generation != s.Generation {
		return m
	}
	if m.failure || m.thread == "" || !m.directKnown || !m.direct || p.target != "" && p.target != m.thread {
		s.invalidate("primary reply refused, unknown or read-only; select a conversation with /resume or /new")
		return m
	}
	s.Thread = m.thread
	s.Ready = true
	s.Reason = "accepted primary intent"
	s.fresh = p.method == "thread/start"
	s.events.bind(m.thread, m.status, time.Now())
	s.canBackfill = p.method == "thread/resume" && !p.reconnect && p.readSerial == s.readSerial
	if s.canBackfill {
		s.readClosedBy = ""
	}
	return m
}

func (s *state) observing() bool {
	if s.Ready || s.fork != nil {
		return true
	}
	for _, p := range s.pending {
		if p.intent && p.generation == s.Generation {
			return true
		}
	}
	return false
}
