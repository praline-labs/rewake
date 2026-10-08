package channel

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
)

// The generated space of the channel: every sequence of events up to a
// length, over an alphabet that covers each kind with the parameters that
// change its meaning, from either launch: the tool offered, or no tool. Each step is
// checked against the rules of docs/mail-bridge-channel.md, not against what
// the fold happens to do.

// letter is one event of the alphabet; its time is relative to the step's.
// A letter with land set is no event: it is the mailboxes becoming writable,
// every notice in flight attempted as the keeper attempts it.
type letter struct {
	name  string
	event Event
	// earlier puts the event this long before the step: an event folded
	// after a later one, as a held close or a late observation is.
	earlier time.Duration
	land    bool
}

func alphabet() []letter {
	return []letter{
		{name: "hello-1", event: Event{Kind: Hello, Generation: 1}},
		{name: "hello-2", event: Event{Kind: Hello, Generation: 2}},
		// A hello told late, as a connection's callback delayed behind
		// another connection's events: from the third step it falls before
		// what earlier steps folded.
		{name: "hello-1-late", event: Event{Kind: Hello, Generation: 1}, earlier: 45 * time.Second},
		{name: "close-1", event: Event{Kind: Closed, Generation: 1, Alive: true}},
		{name: "close-1-held", event: Event{Kind: Closed, Generation: 1, Alive: true}, earlier: stepGap / 2},
		{name: "close-2", event: Event{Kind: Closed, Generation: 2, Alive: true}},
		{name: "close-1-ending", event: Event{Kind: Closed, Generation: 1}},
		{name: "refused-descendant", event: Event{Kind: HelloRefused, Descendant: true}},
		{name: "refused-stray", event: Event{Kind: HelloRefused}},
		// A call that met its binding.
		{name: "bound", event: Event{Kind: Validated}},
		{name: "bound-late", event: Event{Kind: Validated}, earlier: 90 * time.Second},
		// A ticket folded two and a half steps late: from the fourth step it
		// falls between the failures of earlier steps.
		{name: "bound-behind", event: Event{Kind: Validated}, earlier: 5 * stepGap / 2},
		{name: "timer", event: Event{Kind: TimerPassed}},
		{name: "timer-early", event: Event{Kind: TimerPassed}, earlier: stepGap / 2},
		{name: "denied", event: Event{Kind: Denied}},
		{name: "denied-held", event: Event{Kind: Denied}, earlier: stepGap / 2},
		{name: "shell-ok", event: Event{Kind: ShellObserved, OK: true}},
		{name: "shell-ok-late", event: Event{Kind: ShellObserved, OK: true}, earlier: 90 * time.Second},
		{name: "shell-fail", event: Event{Kind: ShellObserved, Class: ShellReadOnly}},
		{name: "exit", event: Event{Kind: Exited}},
		{name: "land", land: true},
	}
}

// starts are the launches the space begins from: the tool offered, and no
// tool.
func starts() []bool { return []bool{true, false} }

// launchReason is why a run started without the tool, as its launch note
// gives it.
const launchReason = "no tool offered"

// stepGap is the time between two steps: past the hello timer, so a timer
// event after any start passes it.
const stepGap = 20 * time.Second

var base = time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)

// stampAt is a moment of the test's run: the boot clock is never zero on a
// running machine, and a zero stamp means "none" in the record.
func stampAt(boot time.Duration) Stamp {
	return Stamp{Boot: int64(time.Hour + boot), Wall: base.Add(boot)}
}

// The recipients of the space's notices: the run's worker and its main.
var (
	spaceWorker = Recipient{Role: ToWorker, Name: "writer", Epoch: "e1"}
	spaceMain   = Recipient{Role: ToMain, Name: "main", Epoch: "e2"}
)

// world is a point of the space: the record and the notices it has fixed,
// carried along the whole path as the keeper carries them.
type world struct {
	record  Record
	notices Notices
	// folded are the events of the path so far, for the rules that speak
	// of a failure or a ticket the record no longer shows.
	folded []Event
}

// spaceStep is one step of a path, for a check.
type spaceStep struct {
	path          string
	before, after world
	e             Event
	land          bool
	planned       []Publication
}

// walk runs every sequence up to depth from a start, calling check after
// each step. After every step the keeper's heartbeat is played: a notice is
// planned for each recipient, against the notices fixed so far.
func walk(t *testing.T, depth int, check func(t *testing.T, s spaceStep)) int {
	t.Helper()
	letters := alphabet()
	count := 0
	var visit func(w world, path []string, at int)
	visit = func(w world, path []string, at int) {
		if at == depth {
			return
		}
		now := stampAt(time.Duration(at+1) * stepGap)
		for _, l := range letters {
			e := l.event
			e.At = stampAt(time.Duration(at+1)*stepGap - l.earlier)
			if e.Kind == Validated {
				e.Issued = e.At.Boot - int64(time.Second)
			}
			after := cloneWorld(w)
			if l.land {
				for _, p := range after.notices.Unsent() {
					state := Landed
					if after.notices.Writable(&after.record, p) != nil {
						state = Dropped
					}
					after.notices.Settle(p.Seq, state, now.Boot)
				}
			} else {
				after.record.Fold(e)
				if !w.record.Frozen {
					// Rule 8: what comes after the exit is not of the run.
					after.folded = append(after.folded, e)
				}
			}
			var planned []Publication
			for _, to := range []Recipient{spaceWorker, spaceMain} {
				if p, ok := after.notices.Plan(&after.record, to, now); ok {
					planned = append(planned, p)
				}
			}
			count++
			name := strings.Join(append(path, l.name), " ")
			check(t, spaceStep{path: name, before: w, after: after, e: e, land: l.land, planned: planned})
			visit(after, append(path, l.name), at+1)
		}
	}
	for _, offered := range starts() {
		w := world{record: New(offered, launchReason, stampAt(0))}
		visit(w, []string{fmt.Sprintf("offered=%v:", offered)}, 0)
	}
	return count
}

func clone(r Record) Record {
	r.Live = append([]uint64(nil), r.Live...)
	r.h = r.h.clone()
	if r.Shell != nil {
		shell := *r.Shell
		r.Shell = &shell
	}
	return r
}

func cloneWorld(w world) world {
	n := w.notices
	n.Told = maps.Clone(n.Told)
	if n.Landings != nil {
		n.Landings = make(map[string][]landing, len(w.notices.Landings))
		for id, landings := range w.notices.Landings {
			n.Landings[id] = slices.Clone(landings)
		}
	}
	n.Publications = slices.Clone(n.Publications)
	return world{record: clone(w.record), notices: n, folded: slices.Clone(w.folded)}
}

// failure says whether an event is one of the failures that open an
// interval.
func failure(k Kind) bool {
	switch k {
	case Closed, HelloRefused, TimerPassed:
		return true
	}
	return false
}

func TestEveryEventSequenceKeepsTheRules(t *testing.T) {
	t.Parallel()
	depth := 4
	if testing.Short() {
		depth = 3
	}
	count := walk(t, depth, func(t *testing.T, s spaceStep) {
		before, after, e := s.before.record, s.after.record, s.e
		fail := func(rule string) {
			t.Helper()
			t.Fatalf("%s: %s\nbefore %+v\nafter  %+v", s.path, rule, before, after)
		}
		if s.land {
			checkLanding(t, s)
			return
		}
		if before.Frozen && !equal(before, after) {
			fail("rule 8: nothing changes after the harness exits")
		}
		if !before.Frozen && !equal(after, foldedAs(s.after.folded, after, sortedByTime)) {
			fail("rule 5: every arrival order folds as event time")
		}
		if !before.Frozen && !equal(after, foldedAs(s.after.folded, after, reversed)) {
			fail("rule 5: the reverse arrival order folds as event time")
		}
		if before.Open() && after.Open() && before.Interval != after.Interval && e.Kind != Validated && e.Kind != TimerPassed &&
			e.At.Boot > before.Interval.Boot {
			fail("rule 5: the interval's start moves only by an event no later than it, or by the clock passing a timer")
		}
		if after.Open() && after.Tool != ToolNone && !intervalAfterTickets(s.after.folded, after.Interval) {
			fail("rule 5: the interval starts at the earliest failure after the last ticket, by event time, whenever the ticket is folded")
		}
		if before.Open() && !after.Open() && e.Kind != Validated && (e.Kind != Hello || e.At.Boot > before.Interval.Boot) {
			fail("rule 2: only a validated ticket closes an interval, or a hello no later than it that shows a server was live, not " + string(e.Kind))
		}
		if !before.Blocked() && after.Blocked() && e.Kind != Denied {
			fail("the block is set by a denial only")
		}
		if e.Kind == Denied && e.At.Boot <= before.Issued && after.Block != before.Block {
			fail("a denial before the newest ticket's issue is lifted by it already")
		}
		if e.Kind == Denied && !before.Frozen && before.Tool != ToolNone && e.At.Boot > before.Issued && after.Block.Boot != max(e.At.Boot, before.Block.Boot) {
			fail("the block's boundary is the latest denial's")
		}
		if before.Blocked() && !after.Blocked() && (e.Kind != Validated || e.Issued <= before.Block.Boot) {
			fail("the block clears only with a ticket issued after the latest denial")
		}
		if before.Tool == ToolNone && after.Tool != ToolNone {
			fail("no tool is for the whole run")
		}
		if after.Tool == ToolNone && after.Blocked() {
			fail("a run without the tool cannot be denied it")
		}
		if (e.Kind == HelloRefused && !e.Descendant) && !equal(before, after) {
			fail("a stray process's refused hello changes nothing")
		}
		if e.Kind == TimerPassed && before.Timer != 0 && e.At.Boot < before.Timer && !equal(before, after) {
			fail("a timer event before the timer's end changes nothing")
		}
		if e.Kind == ShellObserved && before.Shell != nil && e.At.Boot < before.Shell.At.Boot && !equal(before, after) {
			fail("rule 5: a shell observation older than the one kept changes nothing")
		}
		category := after.Category()
		if (category == CategoryShell || category == CategoryNoChannel) && after.Shell.At.Boot < after.Interval.Boot {
			fail("the shell shown is observed since the interval opened")
		}
		if after.Blocked() && category != CategoryDenied {
			fail("the block comes first in the display")
		}
		if line := Label(&after); !strings.HasPrefix(line, "mail: ") || strings.Contains(line, "()") {
			fail("the line names its class or reason: " + line)
		}
		checkPlanned(t, s)
	})
	t.Logf("%d event sequences checked", count)
}

// checkPlanned checks what the heartbeat after a step fixed, against the
// notices fixed before it.
func checkPlanned(t *testing.T, s spaceStep) {
	t.Helper()
	r := s.after.record
	for _, p := range s.planned {
		switch {
		case r.Frozen:
			t.Fatalf("%s: a frozen record fixed %q", s.path, p.Body)
		case p.To == spaceWorker && (r.Category() != CategoryFailing || !strings.HasPrefix(p.Body, FirstWorkerFailing+"\n") || !p.Advice):
			t.Fatalf("%s: the worker is told only of a failing tool, with the shell advice: %q", s.path, p.Body)
		case p.To == spaceMain && (r.Category() == CategoryTool && s.before.notices.Told[spaceMain.id()] == "" || r.Category() == CategoryPending || p.Advice):
			t.Fatalf("%s: main told %q of %s", s.path, p.Body, r.Category())
		}
	}
	for _, to := range []Recipient{spaceWorker, spaceMain} {
		inFlight := 0
		for _, p := range s.after.notices.Publications {
			if p.State == Pending && p.To == to {
				inFlight++
			}
		}
		if inFlight > 1 {
			t.Fatalf("%s: %d notices in flight to %s: those fixed while a mailbox is closed would land together", s.path, inFlight, to.Role)
		}
	}
}

// checkLanding checks a step on which the mailboxes became writable: what
// landed keeps both windows, and nothing lands once the record froze or,
// for shell advice, once the block is set.
func checkLanding(t *testing.T, s spaceStep) {
	t.Helper()
	for _, to := range []Recipient{spaceWorker, spaceMain} {
		before, after := s.before.notices.Landings[to.id()], s.after.notices.Landings[to.id()]
		if len(after) > len(before) && s.after.record.Frozen {
			t.Fatalf("%s: a notice landed after the record froze", s.path)
		}
		for i, landed := range after {
			recent := 0
			for _, other := range after[:i] {
				if other.Key == landed.Key && landed.At-other.At < int64(KeyWindow) {
					t.Fatalf("%s: key %s landed twice within %v", s.path, landed.Key, KeyWindow)
				}
				if landed.At-other.At < int64(HourWindow) {
					recent++
				}
			}
			if recent >= HourLimit {
				t.Fatalf("%s: more than %d landed within an hour", s.path, HourLimit)
			}
		}
	}
	for _, p := range s.before.notices.Unsent() {
		landed := false
		for _, q := range s.after.notices.Publications {
			landed = landed || q.Seq == p.Seq && q.State == Landed
		}
		if landed && p.Advice && s.after.record.Blocked() {
			t.Fatalf("%s: shell advice landed under the block", s.path)
		}
	}
}
