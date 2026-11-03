package modhost

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The mod's endpoint across an exec of its wrapper, the Go-level probe of
// .scratch/reexec-probe.md: the listener goes as a file, and the order of
// the run's loads — the current one with its generation, the superseded,
// the admitted, the numbers — as Handed, so the unchanged mod's next poll is
// answered as before the exec rather than with the 400 a fresh server gives a
// load it never heard. What is not handed over keeps the exec from going
// (Busy). The ends are not handed over: one the core took is answered from
// its done journal however a copy of it is captured, so only an end whose
// capture the core has not taken keeps the exec from going — captured afresh
// in the next image, its boundary would take what was read since, and its
// answer could answer a later task (docs/delivery-turn-end.md).

// handedVersion is the version of Handed; a server adopts only its own.
const handedVersion = 1

// Handed is the endpoint's order of loads as one image hands it to the next.
type Handed struct {
	Version    int                `json:"version"`
	Harness    adapter.RunProcess `json:"harness"`
	Current    *HandedLoad        `json:"current,omitempty"`
	Superseded []string           `json:"superseded,omitempty"`
	Loads      []string           `json:"loads,omitempty"`
	Generation int                `json:"generation"`
	Transition uint64             `json:"transition"`
	// Cache is the last response's cache usage that had input: the mod
	// sends only its last step's, which may be a placeholder's by now.
	Cache *session.Cache `json:"cache,omitempty"`
	// Turn is the main loop's running turn as the host knows it, and
	// Interrupts who stopped which turn: an end heard by the next image still
	// carries its actor and its trace.
	Turn       string                `json:"turn,omitempty"`
	Interrupts []control.Attribution `json:"interrupts,omitempty"`
	// Conversation is the one the last state named, which a command, a late
	// trace among them, is matched against until the next state names one.
	Conversation string `json:"conversation,omitempty"`
}

// HandedLoad is the current load: its hello and its generation's state.
type HandedLoad struct {
	Hello      HelloRequest `json:"hello"`
	Generation int          `json:"generation"`
	Live       bool         `json:"live"`
}

// Busy answers what of the endpoint lives only in this image, "" when
// nothing does.
func (s *Server) Busy() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.busy()
}

// busy is Busy under mu.
func (s *Server) busy() string {
	switch {
	case s.closed:
		return "the mod's endpoint is closing"
	case len(s.management.queued) > 0 || s.management.awaiting(s.current):
		return "a control request is dispatched to the mod"
	case s.asks.Count() > 0:
		return "a permission request or a question of the worker is open"
	case s.untaken():
		return "a turn's end is captured and not yet taken by the core"
	case s.unread():
		return "a compaction's outcome is not yet read by the main that asked for it"
	}
	return ""
}

// unread says a compaction outcome this image holds is still awaited by its
// asker: main's wrapper reads it from the run's published snapshot and only
// then forgets the compaction. The next image starts with no outcome, and
// its first snapshot replaces the last one, so an outcome main had not read
// would end in its 15-minute fallback instead (found in review, November 2,
// 2026). A compaction asked for longer ago than that fallback holds nothing:
// main has said its bound letter, or is gone and would say it when back.
// The caller holds mu.
func (s *Server) unread() bool {
	if s.cfg.RoomDir == "" {
		return false
	}
	outcomes := s.management.observed.view.Snapshot(session.Reading{Counted: true}).CompactionOutcomes
	for _, outcome := range outcomes {
		for _, pending := range control.PendingOf(s.cfg.RoomDir, outcome.RequestedBy) {
			if pending.ID == outcome.Request && time.Since(pending.AskedAt) < unreadBound {
				return true
			}
		}
	}
	return false
}

// unreadBound is main's wrapper's letterBound (internal/host), past which
// it closes a compaction without its outcome.
const unreadBound = 15 * time.Minute

// effecting are the paths whose requests take an effect that lives in this
// image: a load admitted, a turn's start or end captured, an ask opened, the
// face's events, a state, which changes the cache observation, the running
// turn and the conversation handed over. A handover waits for those under
// way and refuses the next, which the mod sends again to the next image; a
// poll or a status line changes nothing handed over and is served
// throughout.
var effecting = map[string]bool{
	PathHello: true, PathTurnStart: true, PathTurnEnd: true, PathAskOpen: true,
	PathAskSettle: true, PathModControl: true, PathMirrorEvents: true, PathModState: true,
}

// errHanding refuses an effect between a handover's snapshot and its exec.
var errHanding = errors.New("the run is being handed over to rewake's next image; send it again")

// enterEffect takes a request's effect in unless a handover froze them,
// holding effects for it until leaveEffect; any other request goes through.
func (s *Server) enterEffect(path string) bool {
	if !effecting[path] {
		return true
	}
	s.effects.RLock()
	s.mu.Lock()
	frozen := s.frozen
	s.mu.Unlock()
	if frozen {
		s.effects.RUnlock()
		return false
	}
	return true
}

func (s *Server) leaveEffect(path string) {
	if effecting[path] {
		s.effects.RUnlock()
	}
}

// HandOver answers the order of loads and a copy of the listener. It is the
// endpoint's quiescent point: every effect under way is waited for, and none
// is taken after it, so what it answers is all this image holds; until the
// exec replaces the image, or Thaw after one that failed, an effect is
// refused with 503 and the mod sends it again.
func (s *Server) HandOver() (Handed, *os.File, error) {
	s.effects.Lock()
	defer s.effects.Unlock()
	s.mu.Lock()
	if why := s.busy(); why != "" {
		s.mu.Unlock()
		return Handed{}, nil, errors.New(why)
	}
	s.frozen = true
	handed := Handed{
		Version: handedVersion, Harness: s.harness, Generation: s.generation, Transition: s.transition, Cache: s.management.observed.cache,
		Turn: s.management.turn, Interrupts: s.management.interrupts.Handed(), Conversation: s.management.observed.thread,
	}
	if s.current != nil {
		handed.Current = &HandedLoad{Hello: s.current.hello, Generation: s.current.generation, Live: s.current.live}
	}
	for load := range s.superseded {
		handed.Superseded = append(handed.Superseded, load)
	}
	for load := range s.loads {
		handed.Loads = append(handed.Loads, load)
	}
	s.mu.Unlock()
	slices.Sort(handed.Superseded)
	slices.Sort(handed.Loads)
	file, err := s.listener.File()
	if err != nil {
		s.Thaw()
	}
	return handed, file, err
}

// Thaw takes effects again after a handover whose exec failed.
func (s *Server) Thaw() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.frozen = false
}

// Adopt serves the listener an earlier image handed over, in place of Start,
// with its order of loads. A live generation is watched from now, the
// adoption counting as its poll, and reported proven, since this image's run
// has heard nothing yet.
func (s *Server) Adopt(handed Handed, listener *os.File) error {
	if handed.Version != handedVersion {
		_ = listener.Close()
		return fmt.Errorf("the mod's endpoint was handed over as version %d; this build takes %d", handed.Version, handedVersion)
	}
	inherited, err := state.Inherit(s.cfg.Socket, listener)
	if err != nil {
		return fmt.Errorf("could not take the mod socket %s: %w", s.cfg.Socket, err)
	}
	s.mu.Lock()
	s.generation, s.transition = handed.Generation, handed.Transition
	s.management.observed.cache, s.management.observed.thread = handed.Cache, handed.Conversation
	s.management.turn = handed.Turn
	s.management.interrupts.Adopt(handed.Interrupts)
	for _, load := range handed.Superseded {
		s.superseded[load] = true
	}
	for _, load := range handed.Loads {
		s.loads[load] = true
	}
	var transition uint64
	if current := handed.Current; current != nil {
		s.current = &loadState{hello: current.Hello, generation: current.Generation, live: current.Live}
		if current.Live {
			s.rearm(s.current)
			s.transition++
			transition = s.transition
		}
	}
	s.mu.Unlock()
	if handed.Harness.PID != 0 {
		s.HarnessStarted(handed.Harness)
	}
	s.serveOn(inherited)
	if transition != 0 {
		s.report(transition, true)
	}
	return nil
}
