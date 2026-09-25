package workflow

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Scenarios run side by side, because a case spends most of its time waiting
// on a timer of the product or the fixture, not on the machine. The suite
// schedules them itself rather than leaving it to go test's -parallel
// semaphore, for one reason: that semaphore hands out slots in whatever order
// the paused tests happen to wake, and a half-minute case that wakes last runs
// alone at the end of the run. The pool starts the longest known scenarios
// first.
//
// -parallel still sets the width. TestMain reads it before m.Run and lifts go
// test's own limit out of the way, so the number the person gave is the
// number of cases running at once.
//
// What must not overlap anything stays serial: go test resumes the parallel
// tests only once every serial one has finished. That covers a test that
// changes the process environment with t.Setenv or a package variable such as
// terminationBudget, and every test that is not a scenario at all.

// defaultPool is the width without -parallel. Eight at once ran the suite in
// 2m23s against six's 3m06s on September 25, 2026, both well under a gigabyte
// more memory in use; six leaves room for another session's build or checks
// running beside it (docs/testing.md).
const defaultPool = 6

// arrivalSettle is how long the pool collects arrivals before it hands out the
// first slot. The paused tests all wake at once when the serial phase ends; a
// slot handed to the first to arrive would be handed in wake order, which is
// the order the pool exists to replace.
const arrivalSettle = 200 * time.Millisecond

// longScenarios name the scenarios measured longest, by the prefix of the case
// name, and how long one case took on the run of 2026-09-25. Only the order
// matters: a scenario not listed starts after these, in arrival order.
var longScenarios = []struct {
	prefix  string
	seconds int
}{
	{"claude-steered", 32},
	{"claude-interrupted", 26},
	{"claude-inbound", 20},
	{"codex-steered", 19},
	{"mid-turn", 12},
}

var pool = struct {
	mu      sync.Mutex
	size    int
	running int
	opened  bool
	timer   *time.Timer
	waiting []*ticket
	// parallel are the tests that called t.Parallel through here, which must
	// not call it twice; holders are the names of the tests holding a slot.
	parallel map[*testing.T]bool
	holders  map[string]bool
}{size: defaultPool, parallel: map[*testing.T]bool{}, holders: map[string]bool{}}

type ticket struct {
	priority int
	granted  chan struct{}
}

// takePoolWidth reads -parallel into the pool and lifts go test's own limit,
// which would otherwise decide the order. Called after flag.Parse and before
// m.Run, which is when go test reads the limit.
func takePoolWidth() error {
	set := false
	flag.Visit(func(f *flag.Flag) { set = set || f.Name == "test.parallel" })
	if set {
		width, err := strconv.Atoi(flag.Lookup("test.parallel").Value.String())
		if err != nil {
			return err
		}
		// go test refuses a width below one itself, but only when it reads
		// the flag, and the limit is lifted before it does: a pool of none
		// would hold every case until -timeout.
		if width < 1 {
			return fmt.Errorf("%d cases at a time runs none; give 1 or more", width)
		}
		pool.size = width
	}
	return flag.Set("test.parallel", "1000")
}

// runParallel makes t a parallel test unless it already is. A test whose
// scenario runs in its subtests calls it so that its subtests can overlap
// other tests' rather than only each other's; it holds no slot itself.
func runParallel(t *testing.T) {
	t.Helper()
	pool.mu.Lock()
	already := pool.parallel[t]
	pool.parallel[t] = true
	pool.mu.Unlock()
	if !already {
		t.Parallel()
	}
}

// joinPool runs the scenario t is entering in the pool: parallel, once a slot
// is free. A test under one that already holds a slot runs inside that slot,
// as it did before — a parent waiting on subtests that wait on the parent's
// slot would never finish.
func joinPool(t *testing.T, scenario string) {
	t.Helper()
	pool.mu.Lock()
	name := t.Name()
	// Entered twice by one test: the slot it holds is the one it runs in.
	inside := pool.holders[name]
	for i := range name {
		if name[i] == '/' && pool.holders[name[:i]] {
			inside = true
		}
	}
	pool.mu.Unlock()
	if inside {
		return
	}
	runParallel(t)
	own := &ticket{priority: priorityOf(scenario), granted: make(chan struct{})}
	pool.mu.Lock()
	pool.waiting = append(pool.waiting, own)
	if pool.timer == nil {
		pool.timer = time.AfterFunc(arrivalSettle, func() {
			pool.mu.Lock()
			pool.opened = true
			dispatch()
			pool.mu.Unlock()
		})
	}
	dispatch()
	pool.mu.Unlock()
	<-own.granted
	pool.mu.Lock()
	pool.holders[name] = true
	pool.mu.Unlock()
	t.Cleanup(func() {
		pool.mu.Lock()
		delete(pool.holders, name)
		pool.running--
		dispatch()
		pool.mu.Unlock()
	})
}

// enterScenarioAround enters a scenario whose cases are its subtests, each of
// which joins the pool itself — the crosswise checks, a case per pair. The
// test registers the scenario and runs in parallel, holding no slot: its
// subtests would otherwise run one at a time inside it.
func enterScenarioAround(t *testing.T, name string) string {
	t.Helper()
	binary := enterSerialScenario(t, name)
	runParallel(t)
	return binary
}

// dispatch hands free slots to the waiting tickets, highest priority first and
// in arrival order among equals. Called with pool.mu held.
func dispatch() {
	for pool.opened && pool.running < pool.size && len(pool.waiting) > 0 {
		next := 0
		for i, candidate := range pool.waiting {
			if candidate.priority > pool.waiting[next].priority {
				next = i
			}
		}
		chosen := pool.waiting[next]
		pool.waiting = append(pool.waiting[:next], pool.waiting[next+1:]...)
		pool.running++
		close(chosen.granted)
	}
}

func priorityOf(scenario string) int {
	for _, long := range longScenarios {
		if strings.HasPrefix(scenario, long.prefix) {
			return long.seconds
		}
	}
	return 0
}
