package switching

import (
	"fmt"
	"net"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Bounds are the leader's waits (docs/rules/state.md#switching-a-running-room).
type Bounds struct {
	// Settle is the least time asking lasts: every participant looks at the
	// mark within its poll, so one silent past it is no participant.
	Settle time.Duration
	// Ask bounds the wait for every participant to be ready.
	Ask time.Duration
	// Drop bounds the wait for every participant to say it is parked.
	Drop time.Duration
	// Drain bounds the wait for the commands still holding the lease.
	Drain time.Duration
	// Confirm bounds the wait for every participant to hold the lease again.
	Confirm time.Duration
}

// Default are the bounds of a switch the person or main asks for.
var Default = Bounds{Settle: time.Second, Ask: 60 * time.Second, Drop: 5 * time.Second, Drain: 5 * time.Second, Confirm: 10 * time.Second}

// hold is how long a mark past a phase's own bound still holds the room:
// the writers' bound, which a parked image's lease takes at most.
const hold = 10 * time.Second

// Outcome is what an attempt came to.
type Outcome struct {
	// Switched says the room's build is the target: the commit happened.
	Switched bool
	// Why says why the room stays on its build, or what was lost after the
	// commit.
	Why string
	// Adopted and Lost name the participants by who they said they are.
	Adopted, Lost []string
}

// member is a participant as the leader knows it, by its pid.
type member struct {
	who, answer, why string
	start            uint64
	conn             *line
}

// event is one line a connection brought, or its end.
type event struct {
	conn   *line
	answer answer
	closed bool
}

type leader struct {
	sw      *build.Switch
	bounds  Bounds
	events  chan event
	members map[int]*member
}

// Lead runs the attempt sw claimed and staged to its end, from asking, and
// leaves its lease to the caller. Before the commit any refusal leaves the
// room on its build with every participant holding its lease again; after
// it, the room is the target's and a participant that does not adopt is lost.
func Lead(sw *build.Switch, bounds Bounds) Outcome {
	// The socket of an earlier attempt, whose leader's lease was revoked
	// before it could remove it.
	_ = state.Discard(sw.Mark.Socket)
	listener, err := state.Listen(sw.Mark.Socket)
	if err != nil {
		_ = sw.End()
		return Outcome{Why: "the switch's socket could not be served: " + err.Error()}
	}
	defer func() { _ = listener.Close() }()
	l := &leader{sw: sw, bounds: bounds, events: make(chan event, 64), members: map[int]*member{}}
	go l.accept(listener)
	return l.run()
}

func (l *leader) accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			line := newLine(conn)
			for {
				var a answer
				if line.receive(&a) != nil || a.Version != state.SwitchVersion {
					_ = conn.Close()
					l.events <- event{conn: line, closed: true}
					return
				}
				l.events <- event{conn: line, answer: a}
			}
		}()
	}
}

// take records one event; an answer is a participant's latest.
func (l *leader) take(e event) {
	if e.closed {
		return
	}
	m := l.members[e.answer.Pid]
	if m == nil || m.start != e.answer.Start {
		m = &member{start: e.answer.Start}
		l.members[e.answer.Pid] = m
	}
	if e.answer.Who != "" {
		m.who = e.answer.Who
	}
	m.answer, m.why, m.conn = e.answer.Answer, e.answer.Why, e.conn
}

// until takes events until done says so or the bound passes; false then.
func (l *leader) until(bound time.Duration, done func() bool) bool {
	deadline := time.After(bound)
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for !done() {
		select {
		case e := <-l.events:
			l.take(e)
		case <-tick.C:
		case <-deadline:
			return done()
		}
	}
	return true
}

func (l *leader) run() Outcome {
	settled := time.Now().Add(l.bounds.Settle)
	var refused string
	l.until(l.bounds.Ask, func() bool {
		if why := l.with(Cannot); why != "" {
			refused = "restart the room: " + why
			return true
		}
		return time.Now().After(settled) && l.with(Busy) == "" && l.all(Ready)
	})
	if refused == "" && (l.with(Busy) != "" || !l.all(Ready)) {
		refused = "not every session was idle within " + l.bounds.Ask.String() + ": " + l.with(Busy)
	}
	if refused != "" {
		_ = l.sw.End()
		return Outcome{Why: refused}
	}
	if err := l.sw.Phase(state.PhaseDropping, time.Now().Add(l.bounds.Drop+l.bounds.Drain+hold)); err != nil {
		_ = l.sw.End()
		return Outcome{Why: err.Error()}
	}
	l.order(Drop)
	if !l.until(l.bounds.Drop, func() bool { return l.with(Busy) != "" || l.all(Parked) }) || l.with(Busy) != "" {
		return l.abort("not every session could hand over: " + l.with(Busy) + l.missing(Parked))
	}
	holders, err := l.sw.Exclusive(l.bounds.Drain)
	switch {
	case err != nil:
		return l.abort("the room's lease could not be taken alone: " + err.Error())
	case len(holders) > 0:
		return l.abort("the room's lease is still held by " + names(holders))
	}
	if err := l.sw.Commit(time.Now().Add(l.bounds.Confirm + hold)); err != nil {
		if theirs, _ := state.ReadBuildRecord(l.sw.Room()); theirs != l.sw.Mark.Target {
			return l.abort("the switch could not be committed: " + err.Error())
		}
	}
	l.order(Go)
	adopted, lost := l.confirm(Go)
	out := Outcome{Switched: true, Adopted: adopted, Lost: lost}
	if len(lost) > 0 {
		out.Why = "lost after the switch: " + strings.Join(lost, ", ")
	}
	return out
}

// abort ends an attempt that dropped and did not commit: the mark aborting,
// every parked image told to resume, and the mark kept until each holds the
// lease again or its deadline passes.
func (l *leader) abort(why string) Outcome {
	if err := l.sw.Abort(time.Now().Add(l.bounds.Confirm + hold)); err != nil {
		// The dropping mark holds the room until its deadline, and the
		// parked images decide by the build, which did not change.
		return Outcome{Why: why + "; the switch's mark is left to its deadline: " + err.Error()}
	}
	l.order(Resume)
	l.confirm(Resume)
	_ = l.sw.End()
	return Outcome{Why: why}
}

// order sends every participant the order; a parked image that arrives
// later is told by confirm.
func (l *leader) order(do string) {
	for _, m := range l.members {
		if m.conn != nil {
			_ = m.conn.send(order{Do: do})
		}
	}
}

// confirm waits until every participant holds the room's lease again, telling
// each parked image that arrives late the same order, and names who did and
// who did not.
func (l *leader) confirm(do string) (adopted, lost []string) {
	told := map[*line]bool{}
	for _, m := range l.members {
		told[m.conn] = true
	}
	holding := func() map[int]bool {
		held := map[int]bool{}
		for _, h := range l.sw.Holders() {
			held[h.Pid] = true
		}
		return held
	}
	l.until(l.bounds.Confirm, func() bool {
		for _, m := range l.members {
			if m.answer == Parked && !told[m.conn] {
				told[m.conn] = true
				_ = m.conn.send(order{Do: do})
			}
		}
		held := holding()
		for pid := range l.members {
			if !held[pid] {
				return false
			}
		}
		return true
	})
	held := holding()
	for pid, m := range l.members {
		if held[pid] {
			adopted = append(adopted, m.who)
		} else {
			lost = append(lost, m.who)
		}
	}
	sort.Strings(adopted)
	sort.Strings(lost)
	return adopted, lost
}

// with names the participants that answered kind, with why.
func (l *leader) with(kind string) string {
	var said []string
	for _, m := range l.members {
		if m.answer == kind {
			said = append(said, m.who+" "+kind+": "+m.why)
		}
	}
	sort.Strings(said)
	return strings.Join(said, "; ")
}

func (l *leader) all(kind string) bool {
	for _, m := range l.members {
		if m.answer != kind {
			return false
		}
	}
	return true
}

// missing names the participants that have not answered kind.
func (l *leader) missing(kind string) string {
	var who []string
	for _, m := range l.members {
		if m.answer != kind {
			who = append(who, m.who)
		}
	}
	if len(who) == 0 {
		return ""
	}
	sort.Strings(who)
	return fmt.Sprintf("%s did not say %s", strings.Join(who, ", "), kind)
}

func names(holders []build.Holder) string {
	text := make([]string, 0, len(holders))
	for _, h := range holders {
		text = append(text, h.String())
	}
	return strings.Join(slices.Compact(text), ", ")
}
