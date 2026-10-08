//go:build rewakefixture

package toolrig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/cli"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/state"
)

// wrapperFaults is the fault seam of the test's own process, which plays every
// rig's wrapper: the endpoint's steps and the acknowledgments run here. A plan
// applies to the paths of one rig's state directory — the endpoint names its
// steps by its context path, which lies there too — so rigs running side by
// side do not see each other's faults.
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
	// pause, when set, is called before the operation, outside the table's
	// lock: an order test holds an acknowledgment or a step there.
	pause func(op, path string)
	// killAt ends the program, with kill, before the endpoint's first step
	// of that name: the harness's process gone while its call runs.
	killAt string
	kill   func()
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
	var kill func()
	if op == state.OpStep && plan.killAt != "" && strings.HasSuffix(path, "/"+plan.killAt) {
		plan.killAt, kill = "", plan.kill
	}
	pause := plan.pause
	f.mu.Unlock()
	if kill != nil {
		kill()
	}
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

// killProgram ends the running program by SIGKILL and returns once it is
// gone, as a harness that died.
func (r *rig) killProgram() {
	pid := r.pid()
	if pid == 0 {
		return
	}
	start, _ := proc.StartTime(pid)
	_ = syscall.Kill(pid, syscall.SIGKILL)
	for until := time.Now().Add(10 * time.Second); time.Now().Before(until); time.Sleep(2 * time.Millisecond) {
		if now, err := proc.StartTime(pid); err != nil || now != start {
			return
		}
		if raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil && strings.Contains(string(raw), ") Z ") {
			return
		}
	}
}

// hold is the fault build's hold of one process: the directory it signals in.
type hold string

func newHold(t *testing.T) hold { return hold(t.TempDir()) }

func (h hold) spec(role string, step int) string {
	return fmt.Sprintf("%s:hold=%d@%s", role, step, h)
}

// specAt holds role before its first named step.
func (h hold) specAt(role, step string) string {
	return fmt.Sprintf("%s:holdat=%s@%s", role, step, h)
}

// reached waits until the process is held.
func (h hold) reached(t *testing.T) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(string(h), "held")); err == nil {
			return
		}
	}
	t.Fatal("the process was never held")
}

func (h hold) release(t *testing.T) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(string(h), "go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
}

// free lets a held process go on, if the test ends while it is held.
func (h hold) free() { _ = os.WriteFile(filepath.Join(string(h), "go"), nil, 0o600) }

// stepHold holds the endpoint's first step of a name, in this process, until
// released: the server's hold of the old rig, now the wrapper's.
type stepHold struct {
	reachedC chan struct{}
	releaseC chan struct{}
	once     sync.Once
	free     func()
}

func (r *rig) holdStep(name string) *stepHold {
	h := &stepHold{reachedC: make(chan struct{}), releaseC: make(chan struct{})}
	h.free = sync.OnceFunc(func() { close(h.releaseC) })
	r.t.Cleanup(h.free)
	r.plan(&wrapperPlan{pause: func(op, path string) {
		if op != state.OpStep || !strings.HasSuffix(path, "/"+name) {
			return
		}
		h.once.Do(func() {
			close(h.reachedC)
			<-h.releaseC
		})
	}})
	return h
}

// heldOr waits until the step is held, or until the call answers without
// reaching it: false then, with the answer in result.
func (h *stepHold) heldOr(t *testing.T, answered chan callResult, result *callResult) bool {
	t.Helper()
	select {
	case <-h.reachedC:
		return true
	case *result = <-answered:
		return false
	case <-time.After(20 * time.Second):
		t.Fatal("the step was neither held nor answered")
		return false
	}
}

// toolOf is the call a model makes for words of the surface: the tool the
// first word names, its arguments from the rest by the descriptor's
// parameters. A flag the descriptor does not name is passed as a switch of
// its name, which the endpoint refuses as the schema would.
func toolOf(words []string) (string, json.RawMessage) {
	if len(words) == 0 {
		return "", nil
	}
	tool, ok := bridge.FindTool(cli.ToolDescriptors(), words[0])
	if !ok {
		return words[0], nil
	}
	arguments := map[string]any{}
	var positionals []string
	rest := words[1:]
	for i := 0; i < len(rest); i++ {
		word := rest[i]
		if word == "--" {
			positionals = append(positionals, rest[i+1:]...)
			break
		}
		if !strings.HasPrefix(word, "--") {
			positionals = append(positionals, word)
			continue
		}
		name, value, inline := strings.Cut(strings.TrimPrefix(word, "--"), "=")
		kind := ""
		for _, param := range tool.Params {
			if param.Name == name {
				kind = param.Kind
			}
		}
		switch {
		case kind == "" || kind == bridge.ParamSwitch:
			arguments[name] = true
		case inline:
			arguments[name] = value
		case i+1 < len(rest):
			arguments[name] = rest[i+1]
			i++
		}
	}
	for _, param := range tool.Params {
		if param.Kind == bridge.ParamWord && len(positionals) > 0 {
			arguments[param.Name], positionals = positionals[0], positionals[1:]
		}
	}
	if len(positionals) > 0 {
		arguments["more words"] = strings.Join(positionals, " ")
	}
	encoded, _ := json.Marshal(arguments)
	return tool.Name, encoded
}
