package channel

import (
	"cmp"
	"maps"
	"slices"
	"time"
)

// The transport's history. Its events reach the record in no fixed order:
// the endpoint serves each connection on its own goroutine, a startup status
// comes through the gateway's reader, and the keeper holds a close for a
// heartbeat. So the record keeps what the tool observation is derived from
// and derives it again on every fold, by event time (rule 5 of
// docs/mail-bridge-channel.md): an event folded late lands where it
// happened, and whatever followed it is judged again with it.
//
// A validated ticket ends everything before it — every failure, and the
// hello timer, since a ticket proves a server is connected — so on Claude
// Code, where every call adds an event, only those after the last ticket are
// kept. Codex keeps its few events for the run, and its tickets with them:
// a ticket is the proof of the conversation its connection serves, which a
// selection may change before or after it, so on Codex a ticket folds by its
// own time like any other event rather than ending the history
// (selection.go). Connections and selections are kept for the run: one
// opened before the ticket may still be live after it, and its close may
// come at any time.
type history struct {
	// conns are the run's connections by generation, each with when it was
	// accepted and when it closed, zero while not told, and the thread it
	// is bound to.
	conns map[uint64]conn
	// events are the other transport events: after the last ticket on
	// Claude Code, all of them on Codex.
	events []Event
	// selections are Codex's selection events, for the run.
	selections []Event
	// tickets are Codex's validated tickets, every one: a later ticket of
	// the same connection does not make an earlier one redundant, since a
	// selection between them may hold the later one and not the earlier.
	tickets []Event
	// session is the run's first SessionStart, kept apart from the events:
	// only it opens the timer, so a later one must not take its place once
	// a ticket ended it.
	session Stamp
	// clock is the latest time an event showed has come — the keeper's
	// heartbeat or any transport event: a timer passes once the clock is
	// past its end, never because time is assumed to have gone by.
	clock int64
}

// conn is one connection: its hello, its close, whether the harness lived
// past that close, and the thread its first request named, which holds
// from its hello (docs/mail-bridge-channel-codex.md#conversation-connections).
type conn struct {
	hello, closed Stamp
	alive         bool
	thread        string
	bound         Stamp
}

// earlier orders two stamps of the same event told twice; the wall clock
// breaks a tie so neither arrival wins it.
func earlier(a, b Stamp) bool {
	return a.Boot < b.Boot || a.Boot == b.Boot && a.Wall.Before(b.Wall)
}

// add keeps one transport event, and says whether it changes anything:
// a stray process's refused hello does not, nor a repeat, a timer event or
// anything a ticket ended that leaves the clock where it was. prune is the
// last ticket's time where what it ended is dropped, zero where it is kept.
func (h *history) add(e Event, prune int64) bool {
	if e.Kind == HelloRefused && !e.Descendant {
		return false
	}
	moved := e.At.Boot > h.clock
	h.clock = max(h.clock, e.At.Boot)
	return h.keep(e, prune) || moved
}

// keep adds an event to the history: a connection's earliest hello, close
// and binding, a selection, the first SessionStart, or another event.
func (h *history) keep(e Event, prune int64) bool {
	switch e.Kind {
	case Hello, Closed, Bound:
		return h.connection(e)
	case SelectionAdmitted, Selected, SelectionFailed:
		if slices.ContainsFunc(h.selections, func(o Event) bool { return o.Kind == e.Kind && o.Thread == e.Thread && o.At == e.At }) {
			return false
		}
		h.selections = append(h.selections, e)
		return true
	case SessionStarted:
		if h.session.Boot != 0 && !earlier(e.At, h.session) {
			return false
		}
		h.session = e.At
		return true
	case TimerPassed:
		return false
	}
	if prune != 0 && e.At.Boot <= prune {
		return false
	}
	h.events = append(h.events, e)
	return true
}

// connection keeps a connection's earliest hello, close and binding.
func (h *history) connection(e Event) bool {
	if h.conns == nil {
		h.conns = map[uint64]conn{}
	}
	c := h.conns[e.Generation]
	switch e.Kind {
	case Hello:
		if c.hello.Boot != 0 && !earlier(e.At, c.hello) {
			return false
		}
		c.hello = e.At
	case Bound:
		if e.Thread == "" || c.bound.Boot != 0 && !earlier(e.At, c.bound) {
			return false
		}
		c.thread, c.bound = e.Thread, e.At
	default:
		switch {
		case c.closed.Boot == 0 || earlier(e.At, c.closed):
			c.closed, c.alive = e.At, e.Alive
		case e.At.Boot == c.closed.Boot && e.At.Wall.Equal(c.closed.Wall) && c.alive && !e.Alive:
			// The same close told both ways: the end's, whichever came first.
			c.alive = false
		default:
			return false
		}
	}
	h.conns[e.Generation] = c
	return true
}

// ticket keeps a Codex ticket and says whether it changes anything: one
// told twice does not. Its time is the clock's as any event's.
func (h *history) ticket(e Event) bool {
	moved := e.At.Boot > h.clock
	h.clock = max(h.clock, e.At.Boot)
	if slices.Contains(h.tickets, e) {
		return moved
	}
	h.tickets = append(h.tickets, e)
	return true
}

// worked drops what a ticket at worked has ended.
func (h *history) worked(worked int64) {
	h.events = slices.DeleteFunc(h.events, func(e Event) bool { return e.At.Boot <= worked })
}

// clone is a history that shares nothing with h.
func (h history) clone() history {
	h.conns = maps.Clone(h.conns)
	h.events = slices.Clone(h.events)
	h.selections = slices.Clone(h.selections)
	h.tickets = slices.Clone(h.tickets)
	return h
}

// liveAt says whether a connection was live at a time: its hello no later,
// its close, if any, after.
func (c conn) liveAt(at int64) bool {
	return c.hello.Boot != 0 && c.hello.Boot <= at && (c.closed.Boot == 0 || c.closed.Boot > at || earlier(c.closed, c.hello))
}

// ranks break a tie of event times by kind, so no arrival order decides it:
// the starts a timer waits for, the selections, then hellos, then closes,
// then failures.
var ranks = map[Kind]int{
	SessionStarted: 0, ThreadAdmitted: 1, SelectionAdmitted: 2, Selected: 3, SelectionFailed: 4,
	CallSeen: 5, Hello: 6, Closed: 7, HelloRefused: 8, StartupFailed: 9, CannotStart: 10, NotObserved: 11,
	// A ticket ends what came at its own time, as one ending the history
	// does on Claude Code.
	Validated: 12,
}

// order is the timeline's: by event time, then by rank, then by generation
// and thread. Two events still equal under it have the same effect in
// either order.
func order(a, b Event) int {
	return cmp.Or(
		cmp.Compare(a.At.Boot, b.At.Boot), a.At.Wall.Compare(b.At.Wall),
		cmp.Compare(ranks[a.Kind], ranks[b.Kind]), cmp.Compare(a.Generation, b.Generation),
		cmp.Compare(a.Thread, b.Thread),
	)
}

// derive is the tool observation from the history: the state the last
// ticket left — working, with the conversation's connections live at its
// time — and every transport event after it, applied in event time. On
// Codex the tickets are in the timeline too, each the proof of its own
// connection's conversation (selection.go).
func (r *Record) derive() {
	worked := r.Worked.Boot
	if r.Harness == Codex {
		worked = 0
	}
	r.Tool, r.Class, r.ClassAt, r.Interval, r.Reconnected, r.Timer = ToolStarting, "", Stamp{}, Stamp{}, Stamp{}, 0
	if worked != 0 {
		r.Tool = ToolWorking
	}
	r.Live, r.Generation = nil, 0
	s := sweep{r: r}
	from := worked
	timeline := slices.Clone(r.h.tickets)
	for _, e := range r.h.events {
		if e.At.Boot > from {
			timeline = append(timeline, e)
		}
	}
	for _, e := range r.h.selections {
		if e.At.Boot > worked {
			timeline = append(timeline, e)
		}
	}
	if r.h.session.Boot > worked {
		timeline = append(timeline, Event{Kind: SessionStarted, At: r.h.session})
	}
	for g, c := range r.h.conns {
		if c.hello.Boot != 0 {
			r.Generation = max(r.Generation, g)
		}
		// A connection of the conversation live when the ticket came is
		// the ticket's; any other event after the start is replayed.
		ticketed := s.ofConversation(g) && c.liveAt(worked)
		switch {
		case ticketed:
			r.Live = append(r.Live, g)
		case c.hello.Boot > from:
			timeline = append(timeline, Event{Kind: Hello, Generation: g, At: c.hello})
		}
		if c.closed.Boot > worked || c.closed.Boot > from && !ticketed {
			timeline = append(timeline, Event{Kind: Closed, Generation: g, At: c.closed, Alive: c.alive})
		}
	}
	slices.Sort(r.Live)
	slices.SortFunc(timeline, order)
	for _, e := range timeline {
		s.pass(e.At.Boot)
		s.apply(e)
	}
	s.pass(0)
	r.Conversation = s.conv
}

// sweep applies the timeline to the record, carrying the running timer's
// wall clock beside the boot time the record shows, and on Codex the
// conversation and a pending selection.
type sweep struct {
	r         *Record
	timerWall time.Time
	// conv is the conversation; pending, when set, the selection admitted
	// and not answered yet, with what it holds (selection.go).
	conv    string
	pending *selection
	// owned is set while an answer refolds an own event of its pending
	// period: a failure of it leaves the timer as it did then, while a hello
	// or a ticket of it now ends the start the answer proved expected.
	owned bool
}

// pass fails the run with "no hello observed" at the timer's end when the
// clock is past it and the next event, if any (next is zero for none), is
// later: an event at the end itself comes first. A pending selection's
// timer passes only once it is answered.
func (s *sweep) pass(next int64) {
	r := s.r
	if s.pending != nil || r.Timer == 0 || r.Timer > r.h.clock || next != 0 && next <= r.Timer {
		return
	}
	at := Stamp{Boot: r.Timer, Wall: s.timerWall}
	r.Timer = 0
	if len(r.Live) == 0 {
		s.fail(ClassNoHello, at)
	}
}

func (s *sweep) apply(e Event) {
	r := s.r
	if r.Harness == Codex && s.codex(e) {
		return
	}
	switch e.Kind {
	case SessionStarted:
		if r.Harness == Claude && r.Tool == ToolStarting {
			s.start(e.At, HelloAtStart)
		}
	case ThreadAdmitted:
		if r.Harness == Codex {
			s.start(e.At, HelloAtStart)
		}
	case CallSeen:
		if r.Harness == Claude {
			s.start(e.At, HelloAtCall)
		}
	case Hello:
		if r.live(e.Generation) {
			return
		}
		r.Live = append(r.Live, e.Generation)
		slices.Sort(r.Live)
		if s.pending == nil {
			r.Timer = 0
		}
		if r.Tool != ToolWorking {
			r.Tool = ToolConnected
		}
		if r.Open() {
			r.Reconnected = e.At
		}
	case Closed:
		s.closed(e)
	case HelloRefused:
		// Only a descendant's is kept.
		if len(r.Live) == 0 {
			s.fail(ClassServerRefused, e.At)
		}
	case CannotStart:
		s.fail(ClassCannotStart, e.At)
	case StartupFailed:
		if len(r.Live) == 0 {
			s.fail(ClassCannotStart, e.At)
		}
	case NotObserved:
		s.fail(ClassNotObserved, e.At)
	case Validated:
		// Codex's ticket, which Claude Code's history never holds: it ends
		// every failure up to it and the conversation's timer, never a
		// pending selection's.
		r.Tool, r.Class, r.ClassAt, r.Interval, r.Reconnected = ToolWorking, "", Stamp{}, Stamp{}, Stamp{}
		if s.pending == nil {
			r.Timer = 0
		}
	}
}

// closed ends one connection. The server is gone only when no other
// connection of the conversation lives at that time, whichever generation
// is newer, and only while the harness lives: neither harness starts a dead
// server again on its own (Codex: probe 1; Claude Code 2.1.284: live,
// October 4, 2026).
func (s *sweep) closed(e Event) {
	r := s.r
	if !r.live(e.Generation) {
		return
	}
	r.Live = slices.DeleteFunc(r.Live, func(g uint64) bool { return g == e.Generation })
	if len(r.Live) > 0 || !e.Alive {
		return
	}
	s.fail(ClassServerGone, e.At)
}

// fail moves the tool to failing: the interval starts at the first failure
// after the last ticket, the class shown is the latest one's. It ends the
// conversation's hello timer, never a pending selection's.
func (s *sweep) fail(class string, at Stamp) {
	r := s.r
	r.Tool = ToolFailing
	if !s.failureKeepsTimer() {
		r.Timer = 0
	}
	if !r.Open() {
		r.Interval = at
	}
	r.Class, r.ClassAt = class, at
}

// start opens the hello timer when the run expects a start: no live
// connection, no timer already running and no selection pending.
func (s *sweep) start(at Stamp, bound time.Duration) {
	r := s.r
	if len(r.Live) > 0 || r.Timer != 0 || s.pending != nil {
		return
	}
	s.open(at, bound)
}

// open sets the timer's end.
func (s *sweep) open(at Stamp, bound time.Duration) {
	s.r.Timer = at.Boot + int64(bound)
	s.timerWall = at.Wall.Add(bound)
}
