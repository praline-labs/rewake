package server_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// The fault test (docs/mail-bridge-checks.md#the-fault-test). Each scenario's
// faulted call is run once clean with every operation logged, of the server,
// its child and the wrapper's acknowledgment. Its steps, not a list written
// by hand, give the cases: the child ended at each of its durable steps, the
// same with the binding unreadable to the server, the child's write failed
// at each, the server ended at each of its steps, each directory the child
// reads made unreadable, and the acknowledgment's write failed at each. Each
// case runs in a fresh run; after the fault, clean calls of the same turn
// recover. Every case must answer only what the rules let it answer, read a
// letter only when its answer arrived whole, and lose or repeat nothing.

// step is one logged operation.
type step struct{ role, op, path string }

func parseLog(t *testing.T, path, root string) []step {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var steps []step
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.SplitN(line, " ", 3)
		if len(fields) != 3 {
			t.Fatalf("a log line: %q", line)
		}
		steps = append(steps, step{fields[0], fields[1], strings.TrimPrefix(fields[2], root)})
	}
	return steps
}

func durable(steps []step, role string) []step {
	var kept []step
	for _, s := range steps {
		if s.role == role && s.op != state.OpRead {
			kept = append(kept, s)
		}
	}
	return kept
}

// readDirs are the directories a role reads, as a part of a path every run
// of the scenario shares.
func readDirs(steps []step, role string) []string {
	var dirs []string
	for _, s := range steps {
		if s.role != role || s.op != state.OpRead {
			continue
		}
		dir := filepath.Dir(s.path) + "/"
		if !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

var idSegment = regexp.MustCompile(`[0-9a-f.-]{12,}`)

// shape names a path without the ids of one run.
func shape(path string) string {
	return idSegment.ReplaceAllString(strings.TrimPrefix(path, "/rooms/default/inbox/"), "*")
}

// faultCase is one fault: the server's plan, and the wrapper's.
type faultCase struct {
	name    string
	fault   string
	wrapper int
	// unreadable fails the wrapper's reads of a directory.
	unreadable string
}

func TestEveryStepOfACallSurvivesAFault(t *testing.T) {
	total := 0
	for _, s := range faultScenarios {
		cases := casesOf(t, s)
		total += len(cases)
		t.Run(s.name, func(t *testing.T) {
			for _, fc := range cases {
				t.Run(fc.name, func(t *testing.T) {
					t.Parallel()
					faulted(t, s, fc)
				})
			}
		})
	}
	t.Logf("%d cases in all", total)
}

// casesOf runs the scenario's faulted call clean, logged, and derives its
// cases from the log.
func casesOf(t *testing.T, s faultScenario) []faultCase {
	t.Helper()
	clean := newRig(t, bridge.CodexTransport)
	words := s.prepare(clean)
	logPath := filepath.Join(clean.root, "fault.log")
	clean.fault = "server:log=" + logPath + ";child:log=" + logPath
	clean.start()
	wrapper := clean.plan(&wrapperPlan{})
	c := clean.call(words...)
	if !s.acceptable(c.result.text()) {
		t.Fatalf("%s: the clean call: %+v", s.name, c.result)
	}
	_ = clean.complete(c, !c.result.IsError)
	steps := parseLog(t, logPath, clean.root)
	children, servers := durable(steps, "child"), durable(steps, "server")
	bindingFirst(t, children)
	acknowledgment := 0
	var wrapperSteps []step
	for _, line := range wrapper.logged() {
		op, path, _ := strings.Cut(line, " ")
		wrapperSteps = append(wrapperSteps, step{"wrapper", op, strings.TrimPrefix(path, clean.root)})
		if op != state.OpRead && op != state.OpStep {
			acknowledgment++
		}
	}
	var cases []faultCase
	for i, at := range children {
		n, where := i+1, at.op+" "+shape(at.path)
		cases = append(cases,
			faultCase{name: fmt.Sprintf("child ends before %d %s", n, where), fault: fmt.Sprintf("child:crash=%d", n)},
			faultCase{name: fmt.Sprintf("child ends before %d, binding unreadable", n), fault: fmt.Sprintf("child:crash=%d;server:readfail=/calls/", n)},
			faultCase{name: fmt.Sprintf("child fails %d %s", n, where), fault: fmt.Sprintf("child:fail=%d", n)},
		)
	}
	for i, at := range servers {
		cases = append(cases, faultCase{name: "server ends before " + at.path, fault: fmt.Sprintf("server:crash=%d", i+1)})
	}
	for _, dir := range readDirs(steps, "child") {
		cases = append(cases, faultCase{name: "child cannot read " + shape(dir), fault: "child:readfail=" + dir})
	}
	for n := 1; n <= acknowledgment; n++ {
		cases = append(cases, faultCase{name: fmt.Sprintf("acknowledgment fails %d", n), wrapper: n})
	}
	for _, dir := range readDirs(wrapperSteps, "wrapper") {
		cases = append(cases, faultCase{name: "acknowledgment cannot read " + shape(dir), unreadable: dir})
	}
	t.Logf("%s: %d child steps, %d server steps, %d acknowledgment writes, %d and %d read directories: %d cases",
		s.name, len(children), len(servers), acknowledgment, len(readDirs(steps, "child")), len(readDirs(wrapperSteps, "wrapper")), len(cases))
	if len(servers) < 3 {
		t.Fatalf("%s: the clean log is too short to be the call's: %+v", s.name, steps)
	}
	return cases
}

// journalPath is a path of api's receipt journal of this run, rest a pattern.
func journalPath(rest string) *regexp.Regexp {
	return regexp.MustCompile(`^/rooms/default/inbox/api/receipts/[^/]+/` + rest + `$`)
}

// beforeBinding are the only durable steps a child may take before its
// binding, none of them an effect: the step after the wrapper confirmed its
// ticket, the record's opening and its index, and the record dropped when
// another call's index landed first.
var beforeBinding = []struct {
	op   string
	path *regexp.Regexp
}{
	{state.OpStep, regexp.MustCompile(`^confirmed$`)},
	{state.OpPublish, journalPath(`[0-9a-f]{24}\.json`)},
	{state.OpPublish, journalPath(`key-[0-9a-f]{32}`)},
	{state.OpRemove, journalPath(`[0-9a-f]{24}\.json`)},
}

var bindingPath = journalPath(`calls/[^/]+`)

// bindingFirst checks that in the child no step outside beforeBinding comes
// before the call's binding: a child that dies before it made no effect.
func bindingFirst(t *testing.T, children []step) {
	t.Helper()
next:
	for _, s := range children {
		if s.op == state.OpPublish && bindingPath.MatchString(s.path) {
			return
		}
		for _, allowed := range beforeBinding {
			if s.op == allowed.op && allowed.path.MatchString(s.path) {
				continue next
			}
		}
		t.Fatalf("the child takes %s %s before its binding", s.op, s.path)
	}
	// A call that holds no record — one stopped by an operation an earlier
	// turn left open — writes no binding, and so no effect either.
}

var retryWords = regexp.MustCompile(`rewake retry ([0-9a-f]{24})`)

func faulted(t *testing.T, s faultScenario, fc faultCase) {
	r := newRig(t, bridge.CodexTransport)
	words := s.prepare(r)
	r.fault = fc.fault
	r.start()
	wrapper := &wrapperPlan{fail: fc.wrapper, readFail: fc.unreadable, during: &r.observing}
	r.plan(wrapper)
	r.blind = fc.unreadable != ""
	readBefore := s.read != nil && s.read(r)
	settledBefore := s.settled(r)
	c := r.call(words...)
	answer := c.result.text()
	key := bridge.CallKey(bridge.CodexTransport, "conversation", c.id)
	token, boundErr := receipt.Bound(r.dir, "api", r.self.Epoch(), key)
	switch {
	case c.ended:
	case strings.Contains(answer, "nothing ran"):
		if !errors.Is(boundErr, receipt.ErrUnbound) {
			t.Fatalf("the call said nothing ran and holds %q (%v): %s", token, boundErr, answer)
		}
		if !settledBefore && s.settled(r) {
			t.Fatalf("the call said nothing ran and its effect is there: %s", answer)
		}
	case retryWords.MatchString(answer):
		// The call's own operation, or, for a call that holds none, the
		// unfinished one its words stopped at.
		named := retryWords.FindStringSubmatch(answer)[1]
		_, loadErr := receipt.Load(r.dir, "api", r.self.Epoch(), named)
		if named != token && (!errors.Is(boundErr, receipt.ErrUnbound) || loadErr != nil) {
			t.Fatalf("the answer names %s and the call holds %q (%v): %s", named, token, boundErr, answer)
		}
	case strings.Contains(answer, "outcome is unknown") && !strings.Contains(fc.fault, "readfail"):
		t.Fatalf("an unknown outcome with a readable binding: %s", answer)
	}
	whole := !c.ended && s.shows(answer) && (s.read == nil || !c.result.IsError)
	_ = r.complete(c, !c.ended && !c.result.IsError)
	// A call that holds no binding leaves the observer one read, of a
	// binding that is not there: failing it changes nothing, and it may come
	// after the completion was handed on.
	if fc.unreadable != "" && wrapper.failedReads() == 0 && !errors.Is(boundErr, receipt.ErrUnbound) {
		t.Fatalf("no read of %s was made to fail", fc.unreadable)
	}
	if s.read != nil && !readBefore && s.read(r) && !whole {
		t.Fatalf("the letter was read though its answer did not arrive whole: %q", answer)
	}

	// The faults end, and a server without them takes the next calls, as a
	// harness restarts one; clean calls of the same turn finish what was
	// begun.
	r.fault, r.blind = "", false
	r.plan(&wrapperPlan{})
	r.start()
	var again []string
	if found := retryWords.FindStringSubmatch(answer); found != nil {
		again = []string{"retry", found[1]}
	}
	shown, absent := whole, s.absent != nil && s.absent(answer)
	for attempt := 0; !absent && (again != nil || !s.settled(r)); attempt++ {
		if attempt > 8 {
			t.Fatalf("not settled after %d clean calls; the faulted answer: %q", attempt, answer)
		}
		if again == nil {
			again = words
		}
		recovered := r.call(again...)
		text := recovered.result.text()
		// A refusal that names the next words is followed: a retry of a
		// read that froze nothing sends the caller back to its words.
		if recovered.ended || !s.acceptable(text) && !retryWords.MatchString(text) && !strings.Contains(text, "run its words again") && (s.absent == nil || !s.absent(text)) {
			t.Fatalf("a clean call after the fault: %q; the faulted answer: %q", text, answer)
		}
		shown = shown || s.shows(text)
		absent = s.absent != nil && s.absent(text)
		_ = r.complete(recovered, !recovered.result.IsError)
		again = next(text)
		if found := retryWords.FindStringSubmatch(text); found != nil {
			again = []string{"retry", found[1]}
		}
	}
	switch {
	case absent && s.settled(r):
		t.Fatalf("an answer proved the effect absent, and it is there; the faulted answer: %q", answer)
	case s.read != nil && !readBefore && !absent && !shown:
		t.Fatalf("settled and never shown; the faulted answer: %q", answer)
	}
	s.check(t, r)
}
