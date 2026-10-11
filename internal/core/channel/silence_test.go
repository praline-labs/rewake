package channel

import (
	"strings"
	"testing"
)

// C3, silence proves nothing (docs/rules/channel.md): over the whole space, a
// run that observed no failure and whose clock has not passed the hello
// timer's end is waiting, not failing; a wait is shown as one and tells the
// worker nothing; and a failure by silence names what was not observed, at
// the end of the bound it was waited for — never a call or a command that
// did not happen.
func TestSilenceProvesNothing(t *testing.T) {
	t.Parallel()
	depth := 4
	if testing.Short() {
		depth = 3
	}
	count := walk(t, depth, func(t *testing.T, s spaceStep) {
		r := s.after.record
		if s.land || r.Tool == ToolNone || r.Frozen {
			return
		}
		fail := func(rule string) {
			t.Helper()
			t.Fatalf("%s: %s\n%+v", s.path, rule, r)
		}
		if silent(s.after.folded) && r.Open() {
			fail("nothing failed and the timer has not run out, yet the tool is failing")
		}
		if r.Category() == CategoryPending {
			if line := Label(&r); line != "mail: tool starting" && line != "mail: tool connected, unused" {
				fail("a wait is shown as something else: " + line)
			}
			for _, p := range s.planned {
				if p.To == spaceWorker {
					fail("the worker was told of a wait: " + p.Body)
				}
			}
		}
		if r.Open() && r.Class == ClassNoHello {
			if r.ClassAt != stampAt(HelloAtStart) {
				fail("a failure by silence is not stamped at the end of the timer from the launch")
			}
			if line := Label(&r); r.Category() == CategoryFailing && !strings.Contains(line, "("+ClassNoHello+")") {
				fail("a failure by silence does not name what was not observed: " + line)
			}
		}
	})
	t.Logf("%d event sequences checked", count)
}

// silent says whether a path observed no failure and showed no time at or
// past the timer's end: only events that are no transport failure, before
// the end, and a stray process's refused hello.
func silent(folded []Event) bool {
	end := stampAt(HelloAtStart).Boot
	for _, e := range folded {
		switch {
		case e.Kind == HelloRefused && !e.Descendant:
		case e.Kind == Denied, e.Kind == ShellObserved, e.Kind == Exited:
		case failure(e.Kind) && e.Kind != TimerPassed:
			return false
		case e.At.Boot >= end:
			return false
		}
	}
	return true
}
