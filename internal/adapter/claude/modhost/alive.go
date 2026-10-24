package modhost

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// The heartbeat (docs/claude-turns.md#the-live-set): the run's current
// load, the order its hellos are bound in, its generation's polls and the
// watch that withdraws what the mod proved when they stop. Every transition
// — a hello that opens a generation, a lapse, the close — takes the next
// number of one sequence under mu, and its report carries it, so a report
// heard late is dropped by its number rather than undoing a later one.

// loadState is the current load: its hello, its heartbeat's generation,
// whether that generation is live, and the lapse watch's timer.
type loadState struct {
	hello      HelloRequest
	generation int
	live       bool
	// arm numbers the watch's timers: each poll re-arms it, and a timer that
	// fires for an arm or a generation no longer current does nothing.
	arm  int
	stop func() bool
}

func (l *loadState) disarm() {
	if l.stop != nil {
		l.stop()
		l.stop = nil
	}
}

// errSuperseded refuses a hello or a poll of a load the run no longer
// orders first; holder is the current load's number, which a hello's
// refusal names.
type errSuperseded struct{ holder int }

func (e errSuperseded) Error() string {
	return "a later load of the mod holds this run; this one is not served again"
}

// errLapsed refuses a poll of a generation that lapsed or is not the load's
// current one: the load's newest generation says hello again.
var errLapsed = errors.New("this heartbeat's generation lapsed or is not the load's current one; say hello again")

// admitLoad binds a hello under mu, ordered by its number alone: the current
// load's retry keeps its live generation and reports nothing; the current
// load after a lapse, a higher number, or another load of the same number —
// a counter that did not advance — opens a generation; a lower number or a
// superseded load is refused with no effect. It answers the generation and,
// when one was opened, the transition to report.
func (s *Server) admitLoad(hello HelloRequest) (generation int, transition uint64, err error) {
	current := s.current
	switch {
	case s.superseded[hello.LoadID]:
		return 0, 0, errSuperseded{holder: current.hello.LoadSeq}
	case current != nil && current.hello.LoadID != hello.LoadID && hello.LoadSeq < current.hello.LoadSeq:
		return 0, 0, errSuperseded{holder: current.hello.LoadSeq}
	case current != nil && current.hello.LoadID == hello.LoadID:
		if current.live {
			return current.generation, 0, nil
		}
	default:
		if current != nil {
			current.disarm()
			s.superseded[current.hello.LoadID] = true
		}
		current = &loadState{hello: hello}
		s.current = current
	}
	s.generation++
	current.generation, current.live = s.generation, true
	s.loads[hello.LoadID] = true
	s.rearm(current)
	s.transition++
	return current.generation, s.transition, nil
}

// rearm restarts the lapse watch of the current generation from now: the
// hello counts as its first poll. Called under mu.
func (s *Server) rearm(load *loadState) {
	load.disarm()
	load.arm++
	generation, arm := load.generation, load.arm
	after := s.after
	if after == nil {
		after = func(d time.Duration, f func()) func() bool { return time.AfterFunc(d, f).Stop }
	}
	load.stop = after(s.lapse(), func() { s.lapsed(generation, arm) })
}

// lapsed withdraws what the mod proved when no poll of the generation came
// within the lapse. Counted among the run's workers, as a handler is: past
// the close it does nothing, and one that began reports before the lease
// goes.
func (s *Server) lapsed(generation, arm int) {
	if !s.admit() {
		return
	}
	defer s.dismiss()
	s.mu.Lock()
	current := s.current
	if s.closed || current == nil || current.generation != generation || current.arm != arm || !current.live {
		s.mu.Unlock()
		return
	}
	current.live = false
	current.stop = nil
	s.transition++
	transition := s.transition
	s.mu.Unlock()
	s.report(transition, false)
}

func (s *Server) report(transition uint64, proven bool) {
	if s.cfg.Live != nil {
		s.cfg.Live(transition, proven)
	}
}

func (s *Server) hold() time.Duration {
	if s.cfg.Hold > 0 {
		return s.cfg.Hold
	}
	return AliveHoldSeconds * time.Second
}

func (s *Server) lapse() time.Duration {
	if s.cfg.Lapse > 0 {
		return s.cfg.Lapse
	}
	return AliveLapseSeconds * time.Second
}

// alive renews the current generation on the poll's arrival and holds the
// answer up to the hold, with no lock held, so a held poll delays no other
// request.
func (s *Server) alive(w http.ResponseWriter, r *http.Request) {
	var poll AliveRequest
	if status, err := decode(w, r, &poll, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if poll.LoadID == "" {
		refuse(w, http.StatusBadRequest, "a poll names its load and its generation")
		return
	}
	if status, err := s.renew(r.Context(), poll); err != nil {
		s.refuseEffect(w, status, err)
		return
	}
	if command := s.takeCommand(poll); command != nil {
		s.answer(w, r, AliveAnswer{Control: command})
		return
	}
	timer := time.NewTimer(s.hold())
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-s.management.wake:
	case <-s.stop:
		refuse(w, http.StatusServiceUnavailable, errClosing.Error())
		return
	case <-r.Context().Done():
		return
	}
	s.answer(w, r, AliveAnswer{Control: s.takeCommand(poll)})
}

// renew is a poll's effect, decided as a turn's is: the peer checked again
// and the server open, under the lock Close takes.
func (s *Server) renew(ctx context.Context, poll AliveRequest) (int, error) {
	if s.deciding != nil {
		s.deciding()
	}
	if err := s.fromHarness(ctx); err != nil {
		return http.StatusForbidden, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.current
	switch {
	case s.closed:
		return http.StatusServiceUnavailable, errClosing
	case s.superseded[poll.LoadID]:
		return http.StatusGone, errSuperseded{holder: current.hello.LoadSeq}
	case current == nil || current.hello.LoadID != poll.LoadID:
		return http.StatusBadRequest, errNoHello
	case current.generation != poll.Generation || !current.live:
		return http.StatusConflict, errLapsed
	}
	s.rearm(current)
	return 0, nil
}
