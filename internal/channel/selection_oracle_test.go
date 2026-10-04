package channel

import "slices"

// The oracle of the generated selection space (selection_space_test.go). It
// reads the rules of docs/mail-bridge-channel-codex.md and the rows of
// docs/mail-bridge-channel-failures.md over the whole path, and is built
// unlike the record on purpose: the record holds what a pending selection
// may not judge yet and folds it at the answer, while the oracle first reads
// ahead how every selection ended and then folds the path once, in event
// time, deciding for each event whether it counts. So a mistake in the
// order the record folds held events cannot be repeated here.

// expected is the oracle's tool observation.
type expected struct {
	conv                                  string
	live                                  []uint64
	tool                                  string
	interval, classAt, timer, reconnected int64
	class                                 string
}

// period is one selection from its first admission to its answer: the
// admissions after the first replace it and keep what it held.
type period struct {
	first, last, end int64
	// timer is the expected start's end the last admission opened, zero
	// when it named a thread one of whose connections was live then.
	timer int64
	// outcome is "selected", "failed", or "" while it is not answered.
	outcome, thread string
	// before is the conversation when it was admitted.
	before string
}

// within says whether a time falls between the first admission and the
// answer, or after the first admission of one not answered.
func (p period) within(at int64) bool {
	return at >= p.first && (p.outcome == "" || at < p.end)
}

// oracle is the path's facts and the fold's state.
type oracle struct {
	expected
	threads        map[uint64]string
	hellos, closes map[uint64]int64
}

func expectSelection(path []Event) expected {
	events := slices.Clone(path)
	slices.SortStableFunc(events, func(a, b Event) int { return int(a.At.Boot - b.At.Boot) })
	o := oracle{threads: map[uint64]string{}, hellos: map[uint64]int64{}, closes: map[uint64]int64{}}
	var clock int64
	for _, e := range events {
		clock = max(clock, e.At.Boot)
		g := e.Generation
		switch e.Kind {
		case Bound:
			// A binding holds from the connection's hello: its first one.
			if _, ok := o.threads[g]; !ok {
				o.threads[g] = e.Thread
			}
		case Hello:
			if _, ok := o.hellos[g]; !ok {
				o.hellos[g] = e.At.Boot
			}
		case Closed:
			if _, ok := o.closes[g]; !ok {
				o.closes[g] = e.At.Boot
			}
		}
	}
	periods := o.periods(events)
	// The conversation is the one the last answer that changed it selected;
	// whatever came before that answer's selection was another's.
	epoch := -1
	for i, p := range periods {
		if p.outcome == "selected" && p.thread != p.before {
			epoch = i
		}
	}
	o.tool = ToolStarting
	if epoch < 0 {
		return o.expected
	}
	start := periods[epoch]
	o.conv = start.thread
	o.live = o.liveOf(o.conv, start.last)
	if len(o.live) > 0 {
		o.tool = ToolConnected
	}
	o.timer = start.timer
	later := periods[epoch+1:]
	open := len(later) > 0 && later[len(later)-1].outcome == ""
	for _, e := range events {
		at := e.At.Boot
		if at < start.first || !o.first(e) {
			continue
		}
		if start.within(at) {
			// Held for the selection that made this conversation: what was
			// the old one's own is not this one's.
			if !o.ownOf(e, start.before) {
				o.normal(e)
			}
			continue
		}
		if p, ok := periodAt(later, at); ok {
			o.pending(e, p)
			continue
		}
		if e.Kind == SelectionFailed && ended(later, at) {
			// The refusal cancels the timer of the selection it answers.
			o.timer = 0
			continue
		}
		o.normal(e)
	}
	if !open {
		o.pass(clock + 1)
	}
	slices.Sort(o.live)
	return o.expected
}

// periods reads the selections of the path, each with how it ended.
func (o *oracle) periods(events []Event) []period {
	var periods []period
	conv := ""
	for _, e := range events {
		at := e.At.Boot
		n := len(periods)
		pending := n > 0 && periods[n-1].outcome == ""
		switch e.Kind {
		case SelectionAdmitted:
			timer := o.expects(e)
			if pending {
				periods[n-1].last, periods[n-1].timer = at, timer
			} else {
				periods = append(periods, period{first: at, last: at, timer: timer, before: conv})
			}
		case Selected:
			if !pending {
				// An answer with no admission is admitted at its own time,
				// with no timer.
				periods = append(periods, period{first: at, last: at, before: conv})
				n++
			}
			periods[n-1].end, periods[n-1].outcome, periods[n-1].thread = at, "selected", e.Thread
			conv = e.Thread
		case SelectionFailed:
			if pending {
				periods[n-1].end, periods[n-1].outcome = at, "failed"
			}
		}
	}
	return periods
}

// expects is the timer an admission opens: none when it names a thread one
// of whose connections was live then.
func (o *oracle) expects(e Event) int64 {
	if e.Thread != "" && len(o.liveOf(e.Thread, e.At.Boot)) > 0 {
		return 0
	}
	return e.At.Boot + int64(HelloAtStart)
}

// ended says whether a selection was refused at a time.
func ended(periods []period, at int64) bool {
	return slices.ContainsFunc(periods, func(p period) bool { return p.outcome == "failed" && p.end == at })
}

// periodAt is the selection pending at a time, if any.
func periodAt(periods []period, at int64) (period, bool) {
	for _, p := range periods {
		if p.within(at) {
			return p, true
		}
	}
	return period{}, false
}

// pending folds an event that came while a selection of the conversation
// was pending. Each admission opens its own timer at its own time, and
// nothing passes it before the answer. One that selected the conversation
// again: everything of the conversation counts, by event time, as if none
// had been pending — a hello or a ticket of its own connections proves the
// start expected, save that a failure of them, counted while the selection
// was pending, ended no timer then and ends none now. One refused or not
// answered: only the conversation's own connections count, ending nothing.
func (o *oracle) pending(e Event, p period) {
	if p.outcome == "selected" || e.At.Boot == p.first {
		// A timer that ended before the admission ended unanswered.
		o.pass(e.At.Boot)
	}
	if e.Kind == SelectionAdmitted {
		o.timer = o.expects(e)
		return
	}
	own := o.ownOf(e, o.conv)
	switch {
	case p.outcome == "selected":
		o.apply(e, answered, !own)
	case own:
		o.apply(e, pendingFold, false)
	}
}

// ownOf says whether an event is of a connection bound to a conversation.
func (o *oracle) ownOf(e Event, conv string) bool {
	switch e.Kind {
	case Hello, Closed, CannotStart, NotObserved, Validated:
		return e.Generation != 0 && conv != "" && o.threads[e.Generation] == conv
	}
	return false
}

// normal folds an event of no pending selection: the timer passes before
// anything later than its end, then the event counts if it is the
// conversation's.
func (o *oracle) normal(e Event) {
	o.pass(e.At.Boot)
	o.apply(e, answered, true)
}

// first says whether an event is the first of its kind for its connection:
// a hello or close told twice happened once, at the earlier time.
func (o *oracle) first(e Event) bool {
	switch e.Kind {
	case Hello:
		return o.hellos[e.Generation] == e.At.Boot
	case Closed:
		return o.closes[e.Generation] == e.At.Boot
	}
	return true
}

// of says whether a connection serves the conversation: bound to its thread
// or not bound at all.
func (o *oracle) of(g uint64) bool {
	thread := o.threads[g]
	return thread == "" || thread == o.conv
}

// belongs says whether a call's outcome is the conversation's: it is its
// connection's; one naming no connection is the conversation's.
func (o *oracle) belongs(e Event) bool {
	return e.Generation == 0 || o.of(e.Generation)
}

// liveOf are the connections bound to a thread that were live at a time:
// accepted by then, not closed by then — a close before the hello is not
// that connection's.
func (o *oracle) liveOf(thread string, at int64) []uint64 {
	var live []uint64
	for g, hello := range o.hellos {
		closed, ok := o.closes[g]
		if o.threads[g] == thread && hello <= at && (!ok || closed > at || closed < hello) {
			live = append(live, g)
		}
	}
	slices.Sort(live)
	return live
}

// pass ends the timer before an event later than its end: "no hello
// observed" when nothing of the conversation lives.
func (o *oracle) pass(next int64) {
	if o.timer == 0 || next <= o.timer {
		return
	}
	end := o.timer
	o.timer = 0
	if len(o.live) == 0 {
		o.fail(ClassNoHello, end, true)
	}
}

// Whether a hello or a ticket ends the timer: not while the selection that
// expects a start is pending, since it may select another conversation;
// once answered, the start expected is the conversation's, and its own hello
// proves it whenever it came.
const (
	pendingFold = false
	answered    = true
)

// apply is one event of the conversation's. ends says whether a hello or a
// ticket ends the timer, settled whether a failure does: a failure of the
// conversation's own connections while a selection was pending ended none,
// and the answer does not change what it did.
func (o *oracle) apply(e Event, ends, settled bool) {
	g, at := e.Generation, e.At.Boot
	switch e.Kind {
	case Hello:
		if !o.of(g) || slices.Contains(o.live, g) {
			return
		}
		o.live = append(o.live, g)
		if ends {
			o.timer = 0
		}
		if o.tool != ToolWorking {
			o.tool = ToolConnected
		}
		if o.interval != 0 {
			// A hello during an open interval is noted, not told.
			o.reconnected = at
		}
	case Closed:
		if !slices.Contains(o.live, g) {
			return
		}
		o.live = slices.DeleteFunc(o.live, func(l uint64) bool { return l == g })
		if len(o.live) == 0 {
			o.fail(ClassServerGone, at, settled)
		}
	case StartupFailed:
		if (e.Thread == "" || e.Thread == o.conv) && len(o.live) == 0 {
			o.fail(ClassCannotStart, at, settled)
		}
	case NotObserved:
		// Calls refused for no hook observation: failing, whatever lives.
		if o.belongs(e) {
			o.fail(ClassNotObserved, at, settled)
		}
	case CannotStart:
		// A server's call whose command could not start: failing.
		if o.belongs(e) {
			o.fail(ClassCannotStart, at, settled)
		}
	case Validated:
		// A child validated its ticket: working, the interval closed.
		if o.belongs(e) {
			o.tool, o.interval, o.class, o.classAt, o.reconnected = ToolWorking, 0, "", 0, 0
			if ends {
				o.timer = 0
			}
		}
	}
}

func (o *oracle) fail(class string, at int64, settled bool) {
	o.tool = ToolFailing
	if settled {
		o.timer = 0
	}
	if o.interval == 0 {
		o.interval = at
	}
	o.class, o.classAt = class, at
}
