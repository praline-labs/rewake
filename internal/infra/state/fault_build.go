//go:build rewaketest

package state

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// A binary built with the rewaketest tag takes its faults from REWAKE_FAULT,
// so a test can break a step of a process it does not run itself: the CLI
// child a tool call starts (docs/testing.md#the-fault-build-inside-the-test-run).
// A release is never built with the tag, and has no such variable.
//
// The value is a list of role:action=argument separated by semicolons. The
// role is child (a process running one tool call) or other (any other
// command, as the shell runs it); the actions:
//
//	log=<file>       append "<role> <op> <path>" for every operation
//	crash=<n>        die by SIGKILL just before the n-th durable step
//	fail=<n>         fail the n-th durable step with an I/O error
//	readfail=<text>  fail every read of a path containing text
//	hold=<n>@<dir>   just before the n-th durable step create <dir>/held and
//	                 wait, at most a minute, until <dir>/go exists: a test
//	                 orders a step of another process against its own
//	holdat=<name>@<dir>  the same, before the first named step <name>, writing
//	                 the name into <dir>/held; with names separated by commas,
//	                 before each it reaches in their order, a name passed by
//	                 dropped, the process removing <dir>/held, then <dir>/go,
//	                 when it goes on to wait for the next
//	failat=<op>:<text>  fail the first durable step of the operation <op> whose
//	                 path contains <text>, with an I/O error
//	orphan=<s>@<dir> the first process of the role to start leaves behind a
//	                 process of its own session, holding its stdout for <s>
//	                 seconds: to whoever reads that stdout, a process that
//	                 outlives SIGKILL to its group
//
// A durable step is a write, a publication, a removal, a rename or a named step of a
// call's path (Step); a step cannot fail, so fail=<n> on one does nothing. A note
// (Note) is logged and never counted.

const faultEnv = "REWAKE_FAULT"

func init() {
	spec := os.Getenv(faultEnv)
	if spec == "" {
		return
	}
	role := "other"
	if os.Getenv("REWAKE_BRIDGE_TICKET_FD") != "" {
		role = "child"
	}
	plan := faultPlan{role: role}
	for _, item := range strings.Split(spec, ";") {
		who, rest, ok := strings.Cut(item, ":")
		if !ok || who != role {
			continue
		}
		action, argument, _ := strings.Cut(rest, "=")
		switch action {
		case "log":
			plan.log = argument
		case "crash":
			plan.crash, _ = strconv.Atoi(argument)
		case "fail":
			plan.fail, _ = strconv.Atoi(argument)
		case "readfail":
			plan.readFail = argument
		case "hold":
			step, held, _ := strings.Cut(argument, "@")
			plan.hold, _ = strconv.Atoi(step)
			plan.held = held
		case "holdat":
			names, held, _ := strings.Cut(argument, "@")
			plan.holdAt, plan.held = strings.Split(names, ","), held
		case "failat":
			plan.failOp, plan.failAt, _ = strings.Cut(argument, ":")
		case "orphan":
			seconds, dir, _ := strings.Cut(argument, "@")
			orphan(seconds, dir)
		}
	}
	Fault = plan.apply
}

type faultPlan struct {
	mu          sync.Mutex
	role        string
	log         string
	crash, fail int
	readFail    string
	steps       int
	hold        int
	holdAt      []string
	held        string
	failOp      string
	failAt      string
}

func (p *faultPlan) apply(op, path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.log != "" {
		if file, err := os.OpenFile(p.log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			_, _ = fmt.Fprintf(file, "%s %s %s\n", p.role, op, path)
			_ = file.Close()
		}
	}
	if op == OpNote {
		return nil
	}
	if op == OpRead {
		if p.readFail != "" && strings.Contains(path, p.readFail) {
			return &os.PathError{Op: "read", Path: path, Err: syscall.EIO}
		}
		return nil
	}
	p.steps++
	if p.steps == p.hold {
		p.wait("")
	}
	if at := p.holdingAt(op, path); at >= 0 {
		p.holdAt = p.holdAt[at+1:]
		p.wait(path)
		if len(p.holdAt) > 0 {
			// held first: a test that sees go gone knows a held it sees
			// next is the next step's.
			_ = os.Remove(filepath.Join(p.held, "held"))
			_ = os.Remove(filepath.Join(p.held, "go"))
		}
	}
	if p.steps == p.crash {
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {}
	}
	if p.steps == p.fail && op != OpStep {
		return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	if p.failOp != "" && op == p.failOp && strings.Contains(path, p.failAt) {
		p.failOp = ""
		return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	return nil
}

// orphan starts the lingering process, once for all processes of the plan:
// the first to create <dir>/orphaned.
func orphan(seconds, dir string) {
	marker, err := os.OpenFile(filepath.Join(dir, "orphaned"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_ = marker.Close()
	lingering := exec.Command("sleep", seconds)
	lingering.Stdout = os.Stdout
	lingering.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	_ = lingering.Start()
}

// holdingAt is the index of a named step in the steps left to hold at, or -1.
func (p *faultPlan) holdingAt(op, path string) int {
	if op != OpStep {
		return -1
	}
	for index, name := range p.holdAt {
		if name == path {
			return index
		}
	}
	return -1
}

// wait signals that the process is held, at the named step if any, and waits
// for leave to go on. The name is published by a rename, so a test reading
// held never sees it half written.
func (p *faultPlan) wait(step string) {
	staged := filepath.Join(p.held, ".held")
	if os.WriteFile(staged, []byte(step), 0o600) == nil {
		_ = os.Rename(staged, filepath.Join(p.held, "held"))
	}
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(p.held, "go")); err == nil {
			return
		}
	}
}
