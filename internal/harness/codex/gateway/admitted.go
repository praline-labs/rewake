package gateway

import (
	"errors"
	"strings"
	"time"
)

type (
	admittedRequest struct {
		binding      Binding
		duringManual bool
	}
	admittedTurn struct {
		binding      Binding
		duringManual bool
	}
	admittedThread struct {
		completed   map[string]bool
		maintenance map[string]bool
		observed    observer
		turns       map[string]admittedTurn
	}
	manualWork struct {
		before map[string]bool
		turn   string
	}
	admittedWork struct {
		stopped       map[string]stoppedWork
		stoppedOrder  []string
		ready         []Completion
		pending       map[string]admittedRequest
		threads       map[string]*admittedThread
		manual        map[string]*manualWork
		excluded      map[string]bool
		excludedOrder []string
	}
)

func newAdmittedWork() admittedWork {
	return admittedWork{pending: map[string]admittedRequest{}, threads: map[string]*admittedThread{}, manual: map[string]*manualWork{}, excluded: map[string]bool{}, stopped: map[string]stoppedWork{}}
}
func isTurnAdmission(method string) bool { return method == "turn/start" || method == "turn/steer" }
func (a *admittedWork) prepare(id string, b Binding, injected bool) error {
	if injected && a.manual[b.Thread] != nil {
		return errors.New("manual maintenance is pending; no work sent")
	}
	count := len(a.pending)
	for _, s := range a.threads {
		count += len(s.turns)
	}
	if count >= 64 || len(a.threads) >= 16 && a.threads[b.Thread] == nil {
		return errors.New("admitted-work capacity reached; no work sent")
	}
	if a.threads[b.Thread] == nil {
		o := newObserver()
		o.bind(b.Thread, "idle", time.Now())
		a.threads[b.Thread] = &admittedThread{observed: o, turns: map[string]admittedTurn{}, maintenance: map[string]bool{}, completed: map[string]bool{}}
	}
	a.pending[id] = admittedRequest{binding: b, duringManual: a.manual[b.Thread] != nil}
	return nil
}

func (a *admittedWork) ack(m meta) {
	p, ok := a.pending[m.id]
	if !ok {
		return
	}
	delete(a.pending, m.id)
	if !m.failure && m.turn != "" {
		s := a.threads[p.binding.Thread]
		if s.completed[m.turn] {
			return
		}
		old, continuation := a.stopped[p.binding.Thread+"/"+m.turn]
		if continuation {
			p.binding = old.binding
			a.forgetStopped(p.binding.Thread + "/" + m.turn)
		}
		if w := s.observed.find(p.binding.Thread, m.turn); w != nil && w.done && !continuation {
			queued := false
			for _, v := range s.observed.out {
				if v.ID == p.binding.Thread+"/"+m.turn {
					queued = true
				}
			}
			if _, live := s.turns[m.turn]; !live && !queued {
				return
			}
		}
		if continuation {
			if w := s.observed.find(p.binding.Thread, m.turn); w != nil {
				w.done = false
				w.active = false
				w.seenActive = false
				w.idle = time.Time{}
				w.endRead = nil
				w.text = old.text
			}
		}
		if _, exists := s.turns[m.turn]; !exists {
			s.turns[m.turn] = admittedTurn(p)
		}
		if !p.duringManual {
			a.allowWork(p.binding.Thread + "/" + m.turn)
			delete(s.maintenance, m.turn)
		}
	}
}

func (a *admittedWork) manualStart(thread string, selected observer) bool {
	if a.manual[thread] == nil && len(a.manual) >= 64 {
		return false
	}
	before := map[string]bool{}
	for _, w := range selected.intervals {
		if w.thread == thread && w.turn != "" {
			before[w.turn] = true
		}
	}
	if s := a.threads[thread]; s != nil {
		for id := range s.turns {
			before[id] = true
		}
	}
	for id, old := range a.stopped {
		if old.binding.Thread == thread {
			before[strings.TrimPrefix(id, thread+"/")] = true
		}
	}
	a.manual[thread] = &manualWork{before: before}
	return true
}
func (a *admittedWork) manualRefused(thread string) { delete(a.manual, thread) }
func (a *admittedWork) event(m meta, raw []byte, now time.Time) {
	if marker := a.manual[m.thread]; marker != nil {
		if (m.method == "turn/started" || m.method == "item/started" && str(raw, "params", "item", "type") == "contextCompaction") && m.turn != "" && !marker.before[m.turn] {
			marker.turn = m.turn
			a.exclude(m.thread + "/" + m.turn)
			if s := a.threads[m.thread]; s != nil {
				s.maintenance[m.turn] = true
			}
		}
		if m.method == "turn/completed" && m.turn != "" && marker.turn == m.turn {
			delete(a.manual, m.thread)
		}
	}
	a.stoppedEvent(m, raw)
	if s := a.threads[m.thread]; s != nil {
		if _, known := s.turns[m.turn]; known && m.method == "turn/completed" {
			a.confirmedTerminal(s, m, raw)
			return
		}
		if m.turn != "" && (m.method == "turn/completed" || m.method == "item/completed") && s.observed.find(m.thread, m.turn) == nil {
			current := s.observed.watches[m.thread]
			if w := s.observed.allocate(m.thread); w != nil {
				w.turn = m.turn
			}
			if current != nil {
				s.observed.watches[m.thread] = current
			}
		}
		s.observed.event(m, raw, now)
	}
}

func (a *admittedWork) exclude(id string) {
	if a.excluded[id] {
		return
	}
	a.excluded[id] = true
	a.excludedOrder = append(a.excludedOrder, id)
	if len(a.excludedOrder) > 128 {
		old := a.excludedOrder[0]
		a.excludedOrder = a.excludedOrder[1:]
		delete(a.excluded, old)
	}
}

func (a *admittedWork) expire(now time.Time) {
	for _, s := range a.threads {
		s.observed.expire(now)
	}
}

func (a *admittedWork) collect() []Completion {
	out := a.ready
	a.ready = nil
	for thread, s := range a.threads {
		pending := false
		for _, p := range a.pending {
			if p.binding.Thread == thread {
				pending = true
				break
			}
		}
		keep := s.observed.out[:0]
		for _, v := range s.observed.out {
			turn := ""
			for id := range s.turns {
				if v.ID == thread+"/"+id {
					turn = id
					break
				}
			}
			if turn == "" {
				if pending {
					keep = append(keep, v)
				}
				continue
			}
			known := s.turns[turn]
			delete(s.turns, turn)
			if v.Kind != "stopped" {
				s.completed[turn] = true
			}
			if (a.excluded[v.ID] || s.maintenance[turn]) && known.duringManual {
				continue
			}
			if v.Kind == "stopped" {
				a.rememberStopped(v.ID, known.binding)
			}
			v.Epoch = known.binding.Epoch
			v.Connection = known.binding.Connection
			v.Generation = known.binding.Generation
			v.Retained = true
			out = append(out, v)
		}
		s.observed.out = keep
		if !pending && len(s.turns) == 0 {
			delete(a.threads, thread)
		}
	}
	return out
}

func (a *admittedWork) overLimit() bool {
	events, intervals, text := 0, 0, 0
	for _, s := range a.threads {
		if s.observed.overflow {
			return true
		}
		events += len(s.observed.out)
		intervals += len(s.observed.intervals) + len(s.maintenance) + len(s.completed)
		for _, v := range s.observed.out {
			text += len(v.Text)
		}
		for _, v := range s.observed.intervals {
			text += len(v.text)
		}
	}
	for _, old := range a.stopped {
		text += len(old.text)
	}
	return events > 64 || intervals > 128 || text > 4<<20
}

func (a *admittedWork) allowWork(id string) {
	delete(a.excluded, id)
	for i, key := range a.excludedOrder {
		if key == id {
			a.excludedOrder = append(a.excludedOrder[:i], a.excludedOrder[i+1:]...)
			break
		}
	}
}

// Steering may acknowledge a turn whose start was observed before the request.
// Copy actual evidence only; an acknowledgement itself is never an active event.
func (a *admittedWork) capture(selected observer, thread string) {
	s := a.threads[thread]
	if s == nil || len(s.observed.intervals) > 0 {
		return
	}
	for _, w := range selected.intervals {
		if w.thread != thread || w.done || w.compact {
			continue
		}
		snapshot := *w
		s.observed.intervals = append(s.observed.intervals, &snapshot)
		s.observed.watches[thread] = &snapshot
		if snapshot.serial > s.observed.sequence {
			s.observed.sequence = snapshot.serial
		}
	}
}
