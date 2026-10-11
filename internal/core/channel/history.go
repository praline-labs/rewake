package channel

import (
	"cmp"
	"maps"
	"slices"
	"time"
)

// The transport's history. Its events reach the record in no fixed order:
// the endpoint serves each connection on its own goroutine, and the keeper
// holds a close for a heartbeat. So the record keeps what the tool
// observation is derived from and derives it again on every fold, by event
// time (rule 5 of docs/mail-bridge-channel.md): an event folded late lands
// where it happened, and whatever followed it is judged again with it.
//
// A validated ticket ends everything before it — every failure, and the
// hello timer, since a ticket proves a transport is connected — so only the
// events after the last ticket are kept. Connections are kept for the run:
// one opened before the ticket may still be live after it, and its close may
// come at any time.
type history struct {
	// conns are the run's connections by generation, each with when it was
	// accepted and when it closed, zero while not told.
	conns map[uint64]conn
	// events are the other transport events after the last ticket.
	events []Event
	// start is the run's start, from which the hello timer runs.
	start Stamp
	// clock is the latest time an event showed has come — the keeper's
	// heartbeat or any transport event: a timer passes once the clock is
	// past its end, never because time is assumed to have gone by.
	clock int64
}

// conn is one connection: its hello, its close, and whether the harness
// lived past that close.
type conn struct {
	hello, closed Stamp
	alive         bool
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

// keep adds an event to the history: a connection's earliest hello and
// close, or another event.
func (h *history) keep(e Event, prune int64) bool {
	switch e.Kind {
	case Hello, Closed:
		return h.connection(e)
	case TimerPassed:
		return false
	}
	if prune != 0 && e.At.Boot <= prune {
		return false
	}
	h.events = append(h.events, e)
	return true
}

// connection keeps a connection's earliest hello and close.
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

// worked drops what a ticket at worked has ended.
func (h *history) worked(worked int64) {
	h.events = slices.DeleteFunc(h.events, func(e Event) bool { return e.At.Boot <= worked })
}

// clone is a history that shares nothing with h.
func (h history) clone() history {
	h.conns = maps.Clone(h.conns)
	h.events = slices.Clone(h.events)
	return h
}

// liveAt says whether a connection was live at a time: its hello no later,
// its close, if any, after.
func (c conn) liveAt(at int64) bool {
	return c.hello.Boot != 0 && c.hello.Boot <= at && (c.closed.Boot == 0 || c.closed.Boot > at || earlier(c.closed, c.hello))
}

// ranks break a tie of event times by kind, so no arrival order decides it:
// hellos, then closes, then failures.
var ranks = map[Kind]int{Hello: 0, Closed: 1, HelloRefused: 2}

// order is the timeline's: by event time, then by rank, then by generation.
// Two events still equal under it have the same effect in either order.
func order(a, b Event) int {
	return cmp.Or(
		cmp.Compare(a.At.Boot, b.At.Boot), a.At.Wall.Compare(b.At.Wall),
		cmp.Compare(ranks[a.Kind], ranks[b.Kind]), cmp.Compare(a.Generation, b.Generation),
	)
}

// derive is the tool observation from the history: the state the last
// ticket left — working, with the connections live at its time — or the
// start's, with its hello timer running, and every transport event after
// it, applied in event time.
func (r *Record) derive() {
	worked := r.Worked.Boot
	r.Tool, r.Class, r.ClassAt, r.Interval, r.Reconnected, r.Timer = ToolStarting, "", Stamp{}, Stamp{}, Stamp{}, 0
	r.Live, r.Generation = nil, 0
	s := sweep{r: r}
	if worked != 0 {
		r.Tool = ToolWorking
	} else {
		s.open(r.h.start, HelloAtStart)
	}
	// The history holds no event the last ticket ended (worked).
	timeline := slices.Clone(r.h.events)
	for g, c := range r.h.conns {
		if c.hello.Boot != 0 {
			r.Generation = max(r.Generation, g)
		}
		// A connection live when the ticket came is the ticket's; any
		// other event after it is replayed.
		ticketed := worked != 0 && c.liveAt(worked)
		switch {
		case ticketed:
			r.Live = append(r.Live, g)
		case c.hello.Boot > worked:
			timeline = append(timeline, Event{Kind: Hello, Generation: g, At: c.hello})
		}
		if c.closed.Boot > worked {
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
}

// sweep applies the timeline to the record, carrying the running timer's
// wall clock beside the boot time the record shows.
type sweep struct {
	r         *Record
	timerWall time.Time
}

// pass fails the run with "no hello observed" at the timer's end when the
// clock is past it and the next event, if any (next is zero for none), is
// later: an event at the end itself comes first.
func (s *sweep) pass(next int64) {
	r := s.r
	if r.Timer == 0 || r.Timer > r.h.clock || next != 0 && next <= r.Timer {
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
	switch e.Kind {
	case Hello:
		if r.live(e.Generation) {
			return
		}
		r.Live = append(r.Live, e.Generation)
		slices.Sort(r.Live)
		r.Timer = 0
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
	}
}

// closed ends one connection. The transport is gone only when no other
// connection lives at that time, whichever generation is newer, and only
// while the harness lives: no harness is taken to start a dead transport
// again on its own.
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
// hello timer.
func (s *sweep) fail(class string, at Stamp) {
	r := s.r
	r.Tool = ToolFailing
	r.Timer = 0
	if !r.Open() {
		r.Interval = at
	}
	r.Class, r.ClassAt = class, at
}

// open sets the timer's end.
func (s *sweep) open(at Stamp, bound time.Duration) {
	s.r.Timer = at.Boot + int64(bound)
	s.timerWall = at.Wall.Add(bound)
}
