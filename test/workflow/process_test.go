package workflow

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// Processes belong to the case, not to the call that started them, and what
// the case owns is a process *group*.
//
// exec.CommandContext is not enough, for three separate reasons. It kills only
// the immediate child, so a descendant keeps the pipe open and decides when
// the call returns — a 30 ms deadline was seen returning after 252 ms. It
// kills with SIGKILL, which for the rewake wrapper skips the signal forwarding
// and teardown the Codex path exists to do. And a parent that exits leaves its
// descendants running: waiting on the parent says nothing about the group.

// groupPoll is how often a dying group is checked. A group has no channel to
// wait on: its members may not be this process's children at all once their
// parent has exited, so the only way to observe it is to ask.
const groupPoll = 10 * time.Millisecond

// owned is one process group the case is responsible for.
//
// done is closed rather than sent to: the group is waited on from the call
// that started it and again when the case ends, and a channel carrying one
// value would deliver it to the first reader and block the second for ever.
type owned struct {
	name string
	cmd  *exec.Cmd
	pgid int
	done chan struct{}
	err  error
	// label is the owner label the process was started under, which its
	// descendants carry too; see owner_labels_test.go.
	label string

	// failureExpected marks a process whose non-zero exit is the point of the
	// scenario. Without it a negative case would have to choose between
	// failing itself and having its exit status ignored.
	failureExpected bool
	// endedByCase marks a process the case killed itself. Its exit status is
	// then the case's own doing and says nothing: reporting "signal:
	// terminated" as a failure would bury the reason it was killed for.
	endedByCase bool
}

// wait blocks until the immediate process has been reaped, and may be called
// any number of times. It says nothing about the rest of the group.
func (process *owned) wait() error {
	<-process.done
	return process.err
}

func (process *owned) finished() bool {
	select {
	case <-process.done:
		return true
	default:
		return false
	}
}

// groupAlive reports whether any member of the group is still running. It is
// the question that matters: the parent exiting is not the group ending.
func (process *owned) groupAlive() bool {
	if process.pgid <= 0 {
		return false
	}
	alive, err := groupHasLiveMember(process.pgid)
	// A failure to look is not an absence: report the group as alive so the
	// loop keeps its read error rather than calling an unchecked group clean.
	return alive || err != nil
}

func (process *owned) signalGroup(signal syscall.Signal) {
	if process.pgid > 0 {
		_ = syscall.Kill(-process.pgid, signal)
	}
}

// reapGroup collects any member of the group that has died and been left to
// this process to bury. A zombie answers kill(2) as though it were alive, so
// without this the group looks immortal and the budget is spent waiting for a
// process that is already dead.
//
// It waits on the group specifically, never on -1: reaping any child at all
// would steal the children of whatever else the test process is running.
func (process *owned) reapGroup() {
	if process.pgid <= 0 {
		return
	}
	for {
		var status syscall.WaitStatus
		pid, err := syscall.Wait4(-process.pgid, &status, syscall.WNOHANG, nil)
		if err != nil || pid <= 0 {
			return
		}
	}
}

// startGroup starts cmd in a process group of its own, under the owner label,
// and begins reaping it.
func startGroup(name, label string, cmd *exec.Cmd) (*owned, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.Env = withLabel(cmd.Env, label)
	if err := startLeader(cmd); err != nil {
		return nil, fmt.Errorf("starting %s: %w", name, err)
	}
	pid := cmd.Process.Pid
	process := &owned{name: name, cmd: cmd, pgid: pid, done: make(chan struct{}), label: label}
	go func() {
		process.err = cmd.Wait()
		forgetLeader(pid)
		close(process.done)
	}()
	return process, nil
}

// outputBounded runs cmd to completion under ctx and returns its combined
// output. It is the preparation-time counterpart of Case.Output: the suite's
// own setup — building the binary, running the self-check child — has to be
// bounded the same way, or a descendant of the build can hold the run open
// past its context.
func outputBounded(ctx context.Context, name string, cmd *exec.Cmd) ([]byte, error) {
	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined
	// A label of its own: this runs beside the cases, and its sweep must take
	// what it started and nothing of theirs.
	process, err := startGroup(name, newOwnerLabel(), cmd)
	if err != nil {
		return nil, err
	}
	var outcome error
	select {
	case <-process.done:
		outcome = process.err
	case <-ctx.Done():
		outcome = fmt.Errorf("%s did not finish in time: %w", name, ctx.Err())
		if left := terminate(process, process.label, nil); len(left) > 0 {
			outcome = fmt.Errorf("%s left work running: %s", name, joinStrings(left, "; "))
		}
	}
	// Asked on every path, not only on the timeout: a parent that exits
	// normally can still leave something running, and preparation that
	// returned nil while a descendant lived is preparation that lied.
	if left := process.descendantsLeft(); len(left) > 0 {
		return combined.Bytes(), fmt.Errorf("%s left work running: %s", name, joinStrings(left, "; "))
	}
	return combined.Bytes(), outcome
}

// descendantsLeft ends and names whatever this process left behind: its own
// group, and any descendant that moved to a group of its own and outlived it.
func (process *owned) descendantsLeft() []string {
	stillUp := process.groupAlive()
	left := terminate(process, process.label, nil)
	if stillUp {
		left = append(left, fmt.Sprintf("group %d was still running", process.pgid))
	}
	return dedup(left)
}

// Output runs cmd under the case's ownership and returns its standard output.
// A non-zero exit fails the case unless the scenario asked for one with
// OutputAllowingFailure.
func (c *Case) Output(cmd *exec.Cmd) ([]byte, error) {
	return c.output(cmd, false)
}

// OutputAllowingFailure is Output for a scenario whose point is that the
// command refuses. The exit status is returned rather than held against the
// case, and the scenario is expected to observe it.
func (c *Case) OutputAllowingFailure(cmd *exec.Cmd) ([]byte, error) {
	return c.output(cmd, true)
}

func (c *Case) output(cmd *exec.Cmd, failureExpected bool) ([]byte, error) {
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	process, err := c.start(cmd, failureExpected)
	if err != nil {
		return nil, err
	}
	if err := c.wait(process); err != nil {
		return stdout.Bytes(), fmt.Errorf("%w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// start launches cmd in its own process group and registers the group with the
// case, so the case remains responsible for it however the call goes.
func (c *Case) start(cmd *exec.Cmd, failureExpected bool) (*owned, error) {
	process, err := startGroup(cmd.Path, c.label, cmd)
	if err != nil {
		return nil, err
	}
	process.failureExpected = failureExpected
	c.mu.Lock()
	c.processes = append(c.processes, process)
	c.mu.Unlock()
	return process, nil
}

// wait joins one process, ending its group if the case runs out of time.
func (c *Case) wait(process *owned) error {
	select {
	case <-process.done:
		return process.err
	case <-c.ctx.Done():
		c.mu.Lock()
		process.endedByCase = true
		c.mu.Unlock()
		// terminate rather than wait-then-clean: what holds the pipe open may
		// be a descendant that left the group, and waiting for the parent
		// first would wait on exactly what nothing has cleaned up yet.
		terminate(process, c.label, c.livePids())
		return fmt.Errorf("%s ran past the case deadline of %s; last observed: %s",
			process.name, c.spec.Deadline, c.lastProgress())
	}
}

// joinProcesses ends and joins everything the case owns, and reports what went
// wrong: a group that was still running, a group that would not die, or a
// process that failed without the scenario expecting it.
//
// An empty state directory proves nothing about live processes, and neither
// does a parent that has exited — so both questions are asked of the group.
func (c *Case) joinProcesses() error {
	c.mu.Lock()
	processes := c.processes
	c.mu.Unlock()
	var problems []string
	for _, process := range processes {
		if !process.finished() {
			c.mu.Lock()
			process.endedByCase = true
			c.mu.Unlock()
			problems = append(problems, terminate(process, c.label, c.livePids())...)
			problems = append(problems, fmt.Sprintf("%s was still running when the case ended", process.name))
			continue
		}
		// The parent is gone; the group may not be.
		if process.groupAlive() {
			problems = append(problems, terminate(process, c.label, c.livePids())...)
			problems = append(problems, fmt.Sprintf("%s left descendants running after it exited", process.name))
			continue
		}
		if process.err != nil && !process.failureExpected && !process.endedByCase {
			problems = append(problems, fmt.Sprintf("%s failed: %v", process.name, process.err))
		}
	}
	// Anything that left the group it was started in is invisible to every
	// check above, so it is looked for by descent instead.
	problems = append(problems, terminate(nil, c.label, c.livePids())...)
	if problems = dedup(problems); len(problems) > 0 {
		return fmt.Errorf("%s", joinStrings(problems, "; "))
	}
	return nil
}

// livePids are the processes the case still accounts for itself, so the sweep
// does not mistake one of them for an orphan.
func (c *Case) livePids() map[int]bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	live := map[int]bool{}
	for _, process := range c.processes {
		if !process.finished() && process.cmd.Process != nil {
			live[process.cmd.Process.Pid] = true
		}
	}
	return live
}

func joinStrings(parts []string, sep string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += sep
		}
		out += part
	}
	return out
}
