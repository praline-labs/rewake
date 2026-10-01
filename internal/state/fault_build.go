//go:build rewakefault

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

// A binary built with the rewakefault tag takes its faults from REWAKE_FAULT,
// so a test can break a step of a process it does not run itself: the mail
// tool's server and the CLI child it starts
// (docs/mail-bridge-checks.md#testing-without-a-live-harness). A release is
// never built with the tag, and has no such variable.
//
// The value is a list of role:action=argument separated by semicolons. The
// role is child (a process running one tool call), server (bridge-serve) or
// other (any other command, as the shell runs it); the actions:
//
//	log=<file>       append "<role> <op> <path>" for every operation
//	crash=<n>        die by SIGKILL just before the n-th durable step
//	fail=<n>         fail the n-th durable step with an I/O error
//	readfail=<text>  fail every read of a path containing text
//	hold=<n>@<dir>   just before the n-th durable step create <dir>/held and
//	                 wait, at most a minute, until <dir>/go exists: a test
//	                 orders a step of another process against its own
//	holdat=<name>@<dir>  the same, before the first named step <name>
//	orphan=<s>@<dir> the first process of the role to start leaves behind a
//	                 process of its own session, holding its stdout for <s>
//	                 seconds: to whoever reads that stdout, a process that
//	                 outlives SIGKILL to its group
//
// A durable step is a write, a publication, a removal, a rename or a named step of a
// call's path (Step); a step cannot fail, so fail=<n> on one does nothing.

const faultEnv = "REWAKE_FAULT"

// FaultEnv is what a process passes on to the processes it starts, so their
// faults come from the same plan.
func FaultEnv() []string {
	if value, ok := os.LookupEnv(faultEnv); ok {
		return []string{faultEnv + "=" + value}
	}
	return nil
}

func init() {
	spec := os.Getenv(faultEnv)
	if spec == "" {
		return
	}
	role := "other"
	switch {
	case os.Getenv("REWAKE_BRIDGE_TICKET_FD") != "":
		role = "child"
	case len(os.Args) > 1 && os.Args[1] == "bridge-serve":
		role = "server"
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
			plan.holdAt, plan.held, _ = strings.Cut(argument, "@")
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
	holdAt      string
	held        string
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
	if op == OpRead {
		if p.readFail != "" && strings.Contains(path, p.readFail) {
			return &os.PathError{Op: "read", Path: path, Err: syscall.EIO}
		}
		return nil
	}
	p.steps++
	if p.steps == p.hold {
		p.wait()
	}
	if op == OpStep && p.holdAt != "" && path == p.holdAt {
		p.holdAt = ""
		p.wait()
	}
	if p.steps == p.crash {
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {}
	}
	if p.steps == p.fail && op != OpStep {
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

// wait signals that the process is held and waits for leave to go on.
func (p *faultPlan) wait() {
	_ = os.WriteFile(filepath.Join(p.held, "held"), nil, 0o600)
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(p.held, "go")); err == nil {
			return
		}
	}
}
