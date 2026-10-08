//go:build rewakefixture

// Package toolrig is the neutral rig of the mail tool
// (docs/v2/stage3-tests-tcl.md#the-neutral-rig): the wrapper's endpoint, its
// end gate, its read clock and the CLI's acknowledgment, met by the fixture's
// tool transport as a harness meets them, with a built rewake as every call's
// child and the shell. It stands beside bridge/server's rig, whose oracles it
// rebuilds on a transport of no harness, until S8 removes that one.
package toolrig

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/bridge/endpoint"
	"github.com/praline-labs/rewake/internal/cli"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/harness/fixture"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/registry/registrytest"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// binary is cmd/rewake built with the fault seam: every call's child, and the
// shell.
var binary string

func TestMain(m *testing.M) {
	if os.Getenv(controlEnv) != "" {
		os.Exit(runProgram(os.Args[1:]))
	}
	dir, err := os.MkdirTemp("", "toolrig")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "rewake")
	build := exec.Command("go", "build", "-tags", "rewakefault", "-o", binary, "../../cmd/rewake")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		_ = os.RemoveAll(dir)
		fmt.Fprintln(os.Stderr, "the test binary does not build:", err)
		os.Exit(1)
	}
	state.Fault = wrapperFaults.apply
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// rig is one run: the session api with web as a live peer, its mailbox, the
// wrapper's endpoint and read clock, and the fixture's backend with the
// program it started.
type rig struct {
	t         *testing.T
	root, dir string
	self, web registry.Session
	endpoint  *endpoint.Endpoint
	clock     *inbox.ReadClock
	// fault is the plan of the next program started, of every child it
	// asks for, and of the shell: roles transport, child and other.
	fault string
	// switches are the next program's variables, NAME=value.
	switches []string
	// acknowledged receives the outcome of every acknowledgment.
	acknowledged chan error
	turn         int
	calls        int
	// observing is set while a result is reported, calling while a request
	// runs: a plan may fail the wrapper's reads only then (wrapperPlan.during).
	observing, calling atomic.Bool
	blind              bool
	// completed are the calls whose result the program already reported:
	// the endpoint handles only the first.
	completed map[string]bool

	mu      sync.Mutex
	backend harness.Backend
	control string
	starts  int
	// boundary is the end the current turn captured, once it did.
	boundary *inbox.ReadBoundary
}

const capability = "rig-capability"

func newRig(t *testing.T, change ...func(*endpoint.Config)) *rig {
	t.Helper()
	// A short directory of its own: a socket path has a bound.
	root, err := os.MkdirTemp("", "tr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	_ = os.Chmod(root, 0o700)
	dir, err := state.RoomDir(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	r := &rig{t: t, root: root, dir: dir, acknowledged: make(chan error, 64), completed: map[string]bool{}}
	r.self = publish(t, dir, "api", os.Getpid())
	sleeper := exec.Command("sleep", "120")
	if err := sleeper.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sleeper.Process.Kill(); _, _ = sleeper.Process.Wait() })
	r.web = publish(t, dir, "web", sleeper.Process.Pid)

	r.clock, err = inbox.OpenReadClock(context.Background(), dir, "api", r.self.Epoch())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.clock.Close)
	// Every call's child runs the built binary under the rig's child plan,
	// read when it starts.
	launcher := filepath.Join(root, "child")
	script := fmt.Sprintf("#!/bin/sh\nREWAKE_FAULT=\"$(cat '%s' 2>/dev/null)\"\nexport REWAKE_FAULT\nexec '%s' \"$@\"\n", filepath.Join(root, "child.fault"), binary)
	if err := writeExecutable(launcher, script); err != nil {
		t.Fatal(err)
	}
	cfg := endpoint.Config{
		Dir: dir, Name: "api", Epoch: r.self.Epoch(), Transport: fixture.Transport, Capability: capability,
		Span: 25 * time.Second, Words: cli.ToolWords, Check: cli.ToolCheck, Tools: cli.ToolDescriptors(),
		Executable: launcher, Gate: endpoint.NewGate(r.clock.Snapshot),
		Acknowledge: func(dir, name, epoch, token string, evidence bridge.Exposure, gate bridge.EndGate) error {
			err := cli.AcknowledgeRead(dir, name, epoch, token, evidence, gate)
			r.acknowledged <- err
			return err
		},
		// The child is another binary than the test that plays its
		// wrapper; the build check is the endpoint's own test's.
		SameBuild: func(int) error { return nil },
	}
	for _, apply := range change {
		apply(&cfg)
	}
	r.endpoint, err = endpoint.Listen(state.ContextPath(dir, "api", r.self.Epoch()), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.endpoint.Close)
	r.endpoint.SetPrimary(func() string { return thread })
	r.endpoint.SetReadsOff(func() string {
		if reads, ok := r.current().(interface{ ToolReadsOff() string }); ok {
			return reads.ToolReadsOff()
		}
		return ""
	})
	launch := map[string]string{
		state.DirEnv: root, state.RoomEnv: "default", state.SessionEnv: "api", state.EpochEnv: r.self.Epoch(),
		"PATH": os.Getenv("PATH"), "HOME": root, "LANG": "C.UTF-8",
	}
	r.endpoint.SetChildEnv(func(name string) string { return launch[name] })
	t.Cleanup(r.stop)
	return r
}

func publish(t *testing.T, dir, name string, pid int) registry.Session {
	t.Helper()
	start, err := proc.StartTime(pid)
	if err != nil {
		t.Fatal(err)
	}
	session := registry.Session{
		Name: name, Harness: fixture.ID, ServicePID: pid, ServiceStart: start, Boot: registrytest.Boot(t),
		PIDNamespace: proc.Namespace(), CWD: dir, StartedAt: time.Now(),
		Socket: filepath.Join(dir, "sock", name+".sock"),
	}
	if err := registry.Publish(dir, session); err != nil {
		t.Fatal(err)
	}
	return session
}

// env is the shell's environment as the launch sets it.
func (r *rig) env() []string {
	env := []string{
		state.DirEnv + "=" + r.root, state.RoomEnv + "=default",
		state.SessionEnv + "=api", state.EpochEnv + "=" + r.self.Epoch(),
		"PATH=" + os.Getenv("PATH"), "HOME=" + r.root, "LANG=C.UTF-8",
	}
	if r.fault != "" {
		env = append(env, "REWAKE_FAULT="+r.fault)
	}
	return env
}

func (r *rig) current() harness.Backend {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.backend
}

// stop ends the program running, as a harness that exits does.
func (r *rig) stop() {
	r.mu.Lock()
	backend := r.backend
	r.backend = nil
	r.mu.Unlock()
	if backend != nil {
		backend.Close()
	}
}

// start starts a program under the fixture's adapter with the rig's plan,
// replacing one that runs: a harness restarted.
func (r *rig) start() {
	r.t.Helper()
	r.stop()
	r.starts++
	n := strconv.Itoa(r.starts)
	control := filepath.Join(r.root, "c"+n+".sock")
	self, err := os.Executable()
	if err != nil {
		r.t.Fatal(err)
	}
	vars := append([]string{controlEnv + "=" + control, faultEnv + "=" + r.fault}, r.switches...)
	launcher := filepath.Join(r.root, "fixture"+n)
	script := "#!/bin/sh\nexec env '" + strings.Join(vars, "' '") + "' '" + self + "' \"$@\"\n"
	if err := writeExecutable(launcher, script); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(r.root, "child.fault"), []byte(r.fault), 0o600); err != nil {
		r.t.Fatal(err)
	}
	plan, err := fixture.New().Launch(harness.LaunchRequest{
		Name: "api", Dir: r.dir, Room: "default", Role: role.Write, Epoch: r.self.Epoch(),
		Socket: filepath.Join(r.root, "f"+n+".sock"), Command: launcher,
	})
	if err != nil {
		r.t.Fatal(err)
	}
	offer, ok := plan.Backend.(interface {
		OfferTools([]bridge.ToolDescriptor, string, func(int, uint64))
	})
	if !ok {
		r.t.Fatal("the fixture's backend takes no tools")
	}
	offer.OfferTools(cli.ToolDescriptors(), state.ContextPath(r.dir, "api", r.self.Epoch()), r.endpoint.SetTransport)
	handler := harness.CompletionHandler{
		Tool: r.endpoint, EndCapture: r.endCapture,
		Publish: func(context.Context, harness.Completion) error { return nil },
	}
	if err := plan.Backend.Start(context.Background(), handler, nil); err != nil {
		r.t.Fatalf("the program: %v", err)
	}
	r.mu.Lock()
	r.backend, r.control = plan.Backend, control
	r.mu.Unlock()
}

// pid is the running program's process.
func (r *rig) pid() int {
	if b, ok := r.current().(interface{ ProcessID() int }); ok {
		return b.ProcessID()
	}
	return 0
}

// ask sends the running program one command.
func (r *rig) ask(c command) (reply, error) {
	r.mu.Lock()
	control := r.control
	r.mu.Unlock()
	return controlAsk(control, c)
}

// endCapture is the wrapper's capture of an end through the gate, once per
// turn: an end the rig captured itself is the one the program's end reports.
func (r *rig) endCapture() (*inbox.ReadBoundary, int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.boundary == nil {
		r.boundary, _ = r.endpoint.Gate().Capture()
	}
	return r.boundary, r.endpoint.Gate().Noted()
}

// letter leaves a task from web where an announced one waits in api's
// mailbox, as the run's wrapper leaves it once it told the harness.
func (r *rig) letter(text string) string {
	r.t.Helper()
	message := inbox.Message{ID: inbox.NewID(), From: "web", FromEpoch: r.web.Epoch(), To: "api", ToEpoch: r.self.Epoch(), Kind: inbox.Task, Text: text, CreatedAt: time.Now()}
	if err := inbox.Put(r.dir, message); err != nil {
		r.t.Fatal(err)
	}
	if err := inbox.PutLocal(r.dir, message); err != nil {
		r.t.Fatal(err)
	}
	return message.ID
}

func (r *rig) turnID() string { return "turn-" + strconv.Itoa(r.turn) }

// nextTurn starts a turn: the program reports it with its own time. A rig
// with no program running starts one first, under the rig's plan.
func (r *rig) nextTurn() string {
	r.t.Helper()
	if r.current() == nil {
		r.start()
	}
	r.turn++
	r.mu.Lock()
	r.boundary = nil
	r.mu.Unlock()
	if answer, err := r.ask(command{Op: "turn-started", Turn: r.turnID(), At: boottime.Now()}); err != nil || answer.Error != "" {
		r.t.Fatalf("the turn's start: %v %s", err, answer.Error)
	}
	return r.turnID()
}

// endTurn ends the current turn: the program reports its end, which the
// adapter captures through the gate and hands the endpoint. A program that
// ended reports nothing; the rig, which plays the wrapper, notes the end then.
func (r *rig) endTurn() *inbox.ReadBoundary {
	r.t.Helper()
	answer, err := r.ask(command{Op: "turn-ended", Turn: r.turnID(), End: r.turnID() + "/end"})
	if errors.Is(err, errEnded) {
		boundary, _ := r.endCapture()
		r.endpoint.TurnEnded(thread, r.turnID())
		return boundary
	}
	if err != nil || answer.Error != "" {
		r.t.Fatalf("the turn's end: %v %s", err, answer.Error)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.boundary
}

// writeExecutable writes a script the rig then runs. The parallel tests of
// this binary fork all the time, and a child forked while the file is open
// for writing keeps it open until it execs: running the script then fails
// with "text file busy". Holding the fork lock while the file is written and
// closed lets no fork see it open.
func writeExecutable(path, script string) error {
	syscall.ForkLock.Lock()
	defer syscall.ForkLock.Unlock()
	return os.WriteFile(path, []byte(script), 0o700)
}
