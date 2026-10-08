package gateway

import (
	"errors"
	"time"
)

type (
	admittedRequest struct {
		binding Binding
		// sent is the request's place in the order of writes (operations.sent).
		sent uint64
	}
	admittedTurn struct {
		binding Binding
		sent    uint64
	}
	admittedThread struct {
		completed map[string]bool
		observed  observer
		turns     map[string]admittedTurn
	}
	// manualWork is a compaction in flight, the terminal's /compact or a
	// main's rewake compact. For a main's it also carries the request and the
	// asker, and ended, closed when its turn completes with status, the
	// failure's message and the context after it, when a usage update said,
	// or when the hold ends first, or sight is lost; answered says it was.
	// tied is closed when the compaction's item first ties the mark to a
	// turn: main's command is answered started by it.
	manualWork struct {
		before              map[string]bool
		turn                string
		request, by         string
		ended, tied         chan struct{}
		answered            bool
		status, failure     string
		tokensBefore, after *int64
		// generation is the selection the request was sent in. lost says
		// the selection changed before the mark was tied: the stream that
		// would show its turn has a gap, so it never ties (lostSight).
		generation uint64
		lost       bool
		// sent is the request's place in the order of writes, the key of
		// its open operation (threadTurns.open).
		sent uint64
		// asked is when the request was sent, from which the mark's bounds
		// run (steer.go); released says it has stopped holding deliveries.
		// The mark itself stays until its turn ends, so that main's letter
		// and the telemetry's author go by it.
		asked    time.Time
		released bool
		// late says main was told the compaction was still running when
		// the wait for its end ended: its end, when seen, is main's outcome
		// all the same (lateEnds).
		late bool
	}
	admittedWork struct {
		stopped      map[string]stoppedWork
		stoppedOrder []string
		ready        []Completion
		pending      map[string]admittedRequest
		threads      map[string]*admittedThread
		manual       map[string]*manualWork
		// proven names the turns known to be work, never a compaction's: one
		// a reply named, or one with an item other than contextCompaction.
		// The gateway's, shared by its connections.
		proven *proofs
		// authored is the asker of the compaction whose turn the last event
		// ended while the mark was tied to it, for the telemetry to write.
		authored compactionAuthor
		// lateEnds are the marks of main's compactions that ended after
		// main was told they were still running, for the gateway to record.
		lateEnds []*manualWork
	}
	compactionAuthor struct{ thread, turn, request, by string }
)

func newAdmittedWork() admittedWork {
	return admittedWork{pending: map[string]admittedRequest{}, threads: map[string]*admittedThread{}, manual: map[string]*manualWork{}, proven: newProofs(), stopped: map[string]stoppedWork{}}
}
func isTurnAdmission(method string) bool { return method == "turn/start" || method == "turn/steer" }
func (a *admittedWork) prepare(id string, b Binding, injected bool, sent uint64) error {
	if injected && a.holding(b.Thread) {
		return ErrCompacting
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
		a.threads[b.Thread] = &admittedThread{observed: o, turns: map[string]admittedTurn{}, completed: map[string]bool{}}
	}
	a.pending[id] = admittedRequest{binding: b, sent: sent}
	return nil
}

func (a *admittedWork) ack(m meta) {
	p, ok := a.pending[m.id]
	if !ok {
		return
	}
	delete(a.pending, m.id)
	if !m.failure && m.turn != "" {
		a.worked(p.binding.Thread, m.turn)
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
	}
}

func (a *admittedWork) event(m meta, raw []byte, now time.Time) {
	a.authored = compactionAuthor{}
	if marker := a.manual[m.thread]; marker != nil {
		// Only the compaction's own item ties the mark to a turn, and only
		// a turn not known to be work: an ordinary turn compacts inside
		// itself too, with the same item (docs/remote-control-codex.md).
		if marker.turn == "" && !marker.lost && m.turn != "" && !marker.before[m.turn] && !a.proven.has(m.thread+"/"+m.turn) && isCompactionItem(m, raw) {
			a.manualTurn(m.thread, m.turn)
		}
		if m.method == "thread/tokenUsage/updated" && m.turn != "" && marker.turn == m.turn {
			marker.after = telemetryInteger(field(raw, "params", "tokenUsage", "last", "totalTokens"))
		}
		if terminal(m) && marker.turn == m.turn {
			delete(a.manual, m.thread)
			if marker.by != "" {
				a.authored = compactionAuthor{thread: m.thread, turn: m.turn, request: marker.request, by: marker.by}
			}
			failure := decodeText(field(raw, "params", "turn", "error", "message"))
			if marker.late {
				marker.status, marker.failure = m.status, failure
				a.lateEnds = append(a.lateEnds, marker)
			} else {
				marker.finish(m.status, failure)
			}
		}
	}
	if m.turn != "" && (m.method == "item/started" || m.method == "item/completed") {
		if isCompactionItem(m, raw) {
			a.proven.compaction(m.thread + "/" + m.turn)
		} else {
			a.worked(m.thread, m.turn)
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

func isCompactionItem(m meta, raw []byte) bool {
	return (m.method == "item/started" || m.method == "item/completed") && str(raw, "params", "item", "type") == "contextCompaction"
}

// worked records a turn shown to be work. A mark tied to it was tied by an
// auto-compaction whose item came before the proof — before the turn's reply,
// or before its first other item — and is untied: a manual compaction's turn
// has no item but its own, and no reply names it.
func (a *admittedWork) worked(thread, turn string) {
	a.proven.add(thread + "/" + turn)
	if marker := a.manual[thread]; marker != nil && marker.turn == turn {
		marker.turn = ""
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
		intervals += len(s.observed.intervals) + len(s.completed)
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

// Steering may acknowledge a turn whose start was observed before the request.
// Copy actual evidence only; an acknowledgement itself is never an active event.
func (a *admittedWork) capture(selected observer, thread string) {
	s := a.threads[thread]
	if s == nil || len(s.observed.intervals) > 0 {
		return
	}
	for _, w := range selected.intervals {
		if w.thread != thread || w.done {
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
