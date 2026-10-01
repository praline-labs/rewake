package server_test

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/praline-labs/rewake/internal/state"
)

// wrapperFaults is the fault seam of the test's own process, which plays
// every rig's wrapper: the acknowledgments run here. A plan applies to the
// paths of one rig's state directory, so rigs running side by side do not
// see each other's faults.
var wrapperFaults = &faultTable{plans: map[string]*wrapperPlan{}}

type faultTable struct {
	mu    sync.Mutex
	plans map[string]*wrapperPlan
}

// wrapperPlan logs a rig's operations and fails its fail-th durable step.
type wrapperPlan struct {
	log   []string
	fail  int
	steps int
	// readFail fails every read of a path containing it; readFailed
	// counts those that did.
	readFail   string
	readFailed int
	// during, when set, limits readFail to while it holds: the test's own
	// checks read in this process too.
	during *atomic.Bool
	// pause, when set, is called before the durable step it names, outside
	// the table's lock: an order test holds an acknowledgment there.
	pause func(op, path string)
}

func (f *faultTable) apply(op, path string) error {
	f.mu.Lock()
	var plan *wrapperPlan
	for dir, candidate := range f.plans {
		if strings.HasPrefix(path, dir+string(os.PathSeparator)) {
			plan = candidate
		}
	}
	if plan == nil {
		f.mu.Unlock()
		return nil
	}
	plan.log = append(plan.log, op+" "+path)
	failed := op == state.OpRead && plan.readFail != "" && strings.Contains(path, plan.readFail) && (plan.during == nil || plan.during.Load())
	if failed {
		plan.readFailed++
	}
	if op != state.OpRead && op != state.OpStep {
		plan.steps++
		failed = plan.steps == plan.fail
	}
	pause := plan.pause
	f.mu.Unlock()
	if pause != nil {
		pause(op, path)
	}
	if failed {
		return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	return nil
}

// plan installs a plan for the rig's directory until the test ends.
func (r *rig) plan(plan *wrapperPlan) *wrapperPlan {
	wrapperFaults.mu.Lock()
	wrapperFaults.plans[r.dir] = plan
	wrapperFaults.mu.Unlock()
	r.t.Cleanup(func() {
		wrapperFaults.mu.Lock()
		delete(wrapperFaults.plans, r.dir)
		wrapperFaults.mu.Unlock()
	})
	return plan
}

// failedReads is how many reads the plan failed.
func (p *wrapperPlan) failedReads() int {
	wrapperFaults.mu.Lock()
	defer wrapperFaults.mu.Unlock()
	return p.readFailed
}

// logged is a copy of the plan's log.
func (p *wrapperPlan) logged() []string {
	wrapperFaults.mu.Lock()
	defer wrapperFaults.mu.Unlock()
	return append([]string(nil), p.log...)
}
