package channel

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The generated space of Codex's conversations: every sequence of selection,
// binding and connection events up to a length, from a run whose thread A
// works, checked step by step against an oracle that reads the rules of
// docs/mail-bridge-channel-codex.md over the whole path in event time
// (selection_oracle_test.go).

func selectionAlphabet() []letter {
	named := func(name string, kind Kind, opts ...func(*Event)) letter {
		e := Event{Kind: kind}
		for _, o := range opts {
			o(&e)
		}
		return letter{name: name, event: e}
	}
	return []letter{
		named("admit", SelectionAdmitted),
		named("admit-B", SelectionAdmitted, thread("B")),
		named("admit-A", SelectionAdmitted, thread("A")),
		named("selected-B", Selected, thread("B")),
		named("selected-A", Selected, thread("A")),
		named("failed", SelectionFailed),
		named("hello-2", Hello, gen(2)),
		named("bound-2-B", Bound, gen(2), thread("B")),
		named("bound-2-sub", Bound, gen(2), thread("sub")),
		named("close-1", Closed, gen(1), alive),
		named("close-2", Closed, gen(2), alive),
		named("status-B", StartupFailed, thread("B")),
		named("status-none", StartupFailed),
		named("timer", TimerPassed),
	}
}

// callAlphabet adds what a call tells: a missing observation, a command that
// cannot start, a ticket, each over A's connection or the second one — a
// wait admitted for A may end after B is selected, and its outcome is A's.
func callAlphabet() []letter {
	letters := selectionAlphabet()
	for _, g := range []uint64{1, 2} {
		name := strconv.FormatUint(g, 10)
		letters = append(letters,
			letter{name: "unobserved-" + name, event: Event{Kind: NotObserved, Generation: g}},
			letter{name: "cannot-start-" + name, event: Event{Kind: CannotStart, Generation: g}},
			letter{name: "ticket-" + name, event: Event{Kind: Validated, Generation: g}},
		)
	}
	return letters
}

// selectionPrefix is the run before the first step: thread A selected, its
// server bound to it, a ticket.
func selectionPrefix() []Event {
	var events []Event
	for _, st := range conversationA() {
		e := st.event
		e.At = stampAt(st.at)
		events = append(events, e)
	}
	return events
}

func TestEverySelectionSequenceKeepsTheRules(t *testing.T) {
	t.Parallel()
	depth := 5
	if testing.Short() {
		depth = 4
	}
	t.Logf("%d selection sequences checked", walkSelections(t, selectionAlphabet(), depth, stepGap))
}

// Every letter, calls among them, and the second connection bound to A, a
// step apart shorter than the hello timer, so a timer still runs at the
// next events, a pending selection's among them: at the space's own gap
// every timer has passed before the next step. The gap stays past the
// prefix's ticket.
func TestEverySelectionSequenceWithinATimerKeepsTheRules(t *testing.T) {
	t.Parallel()
	depth := 4
	if testing.Short() {
		depth = 3
	}
	letters := append(callAlphabet(), letter{name: "bound-2-A", event: Event{Kind: Bound, Generation: 2, Thread: "A"}})
	t.Logf("%d sequences within a timer checked", walkSelections(t, letters, depth, 6*time.Second))
}

// The calls' letters with every selection letter, one step shorter: twenty
// letters at depth five would take minutes.
func TestEveryCallOutcomeIsItsConnectionsConversations(t *testing.T) {
	t.Parallel()
	depth := 4
	if testing.Short() {
		depth = 3
	}
	t.Logf("%d sequences with calls checked", walkSelections(t, callAlphabet(), depth, stepGap))
}

// walkSelections folds every sequence of letters up to depth after the
// prefix, a gap apart, checking each step, and answers how many it checked.
func walkSelections(t *testing.T, letters []letter, depth int, gap time.Duration) int {
	t.Helper()
	count := 0
	var visit func(w world, path []string, at int)
	visit = func(w world, path []string, at int) {
		if at == depth {
			return
		}
		now := stampAt(time.Duration(at+1) * gap)
		for _, l := range letters {
			e := l.event
			e.At = now
			after := cloneWorld(w)
			after.record.Fold(e)
			after.folded = append(after.folded, e)
			var planned []Publication
			for _, to := range []Recipient{spaceWorker, spaceMain} {
				if p, ok := after.notices.Plan(&after.record, to, now); ok {
					planned = append(planned, p)
				}
			}
			count++
			name := strings.Join(append(path, l.name), " ")
			checkSelection(t, spaceStep{path: name, before: w, after: after, e: e, planned: planned})
			visit(after, append(path, l.name), at+1)
		}
	}
	w := world{record: New(Codex, true, "", stampAt(0))}
	for _, e := range selectionPrefix() {
		w.record.Fold(e)
		w.folded = append(w.folded, e)
	}
	visit(w, []string{"codex A working:"}, 0)
	return count
}

func checkSelection(t *testing.T, s spaceStep) {
	t.Helper()
	got, e := s.after.record, s.e
	fail := func(rule string) {
		t.Helper()
		t.Fatalf("%s: %s\nbefore %+v\nafter  %+v", s.path, rule, s.before.record, got)
	}
	want := expectSelection(s.after.folded)
	switch {
	case got.Conversation != want.conv:
		fail("the conversation is the last selected thread: want " + want.conv)
	case !slices.Equal(got.Live, want.live):
		fail("the live connections are the conversation's")
	case got.Tool != want.tool:
		fail("the tool observation: want " + want.tool)
	case got.Interval.Boot != want.interval || got.Open() && (got.Class != want.class || got.ClassAt.Boot != want.classAt):
		fail("the interval and its class: want " + want.class)
	case got.Timer != want.timer:
		fail("the hello timer")
	case !equal(got, foldedAs(s.after.folded, got, sortedByTime)):
		fail("rule 5: every arrival order folds as event time")
	case !equal(got, foldedAs(s.after.folded, got, reversed)):
		fail("rule 5: the reverse arrival order folds as event time")
	}
	before := s.before.record
	if e.Kind == Selected && e.Thread != before.Conversation {
		for _, p := range s.planned {
			if p.To == spaceMain && strings.HasPrefix(p.Body, FirstMainWorks) {
				fail("the old conversation's interval closes at the answer with no notice")
			}
		}
		if got.Tool == ToolWorking && !ticketAfter(s.after.folded, len(selectionPrefix())) {
			fail("a new conversation never works on the old one's proof")
		}
	}
	if e.Kind == SelectionFailed {
		// Against the same moment without the refusal: a timer may have
		// ended between the steps.
		still := cloneWorld(s.before)
		still.record.Fold(Event{Kind: TimerPassed, At: e.At})
		if still.record.Category() != got.Category() {
			fail("a refused selection tells nothing and leaves the display the old conversation's")
		}
	}
}

// ticketAfter says whether a ticket was folded after the prefix: only one
// can make a new conversation work.
func ticketAfter(folded []Event, prefix int) bool {
	return slices.ContainsFunc(folded[prefix:], func(e Event) bool { return e.Kind == Validated })
}
