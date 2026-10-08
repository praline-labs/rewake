package endpoint

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// Running a transport's call (docs/mail-bridge-server.md#running-the-child,
// the same steps in the wrapper). Every effect of a call happens in its child,
// under its receipt; the endpoint only starts it, bounds it, and reads what it
// left. It never repeats, finishes or undoes what the child did.

// hardBound is how long past its deadline a child may live.
const hardBound = 5 * time.Second

// reapWait is how long, once the endpoint closes, it waits for a killed child
// to be reaped: past it the call is answered as unknown.
const reapWait = time.Second

// outputCap bounds what is kept of a child's stdout and stderr. A child bounds
// its own answer to one result; more than this is one that broke its bound,
// and its answer is replaced whole either way.
const outputCap = 64 << 10

// maxChildren is how many calls run at once; a fifth is refused as busy.
const maxChildren = 4

// capped keeps the first outputCap bytes and remembers that more came.
type capped struct {
	bytes.Buffer
	over bool
}

func (c *capped) Write(data []byte) (int, error) {
	if room := outputCap - c.Len(); room < len(data) {
		c.over = true
		if room > 0 {
			c.Buffer.Write(data[:room])
		}
		return len(data), nil
	}
	return c.Buffer.Write(data)
}

// runChild runs one call's child under its ticket and returns its answer, or
// what the endpoint can say of a child that left none.
func (e *Endpoint) runChild(ticket bridge.Ticket, words []string) childAnswer {
	reader, writer, err := os.Pipe()
	if err != nil {
		return substitute(startFailed)
	}
	encoded, _ := json.Marshal(ticket)
	executable := e.cfg.Executable
	if executable == "" {
		executable = "/proc/self/exe"
	}
	e.mu.Lock()
	env := append(append([]string(nil), e.childEnv...), bridge.TicketEnv+"=3")
	e.mu.Unlock()
	command := exec.Command(executable, words...)
	command.Env = env
	command.ExtraFiles = []*os.File{reader}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr capped
	command.Stdout, command.Stderr = &stdout, &stderr
	e.step("start")
	err = command.Start()
	_ = reader.Close()
	if err != nil {
		_ = writer.Close()
		return substitute(startFailed)
	}
	pid := command.Process.Pid
	// Known by its start too: its pid is free again once it is reaped.
	start, _ := proc.StartTime(pid)
	e.ownChild(pid, start, true)
	defer e.ownChild(pid, start, false)
	// The ticket fits the pipe's buffer, and the child reads it to its end
	// within a second; the endpoint never waits on it.
	_, _ = writer.Write(encoded)
	_ = writer.Close()

	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	bound := time.NewTimer(max(0, time.Duration(ticket.DeadlineBoot-boottime.Now())) + hardBound)
	defer bound.Stop()
	select {
	case err = <-exited:
	case <-bound.C:
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		// A child that does not die, or something of it that holds its
		// output, keeps its slot while the endpoint serves; once it closes
		// the child is waited for only reapWait more.
		select {
		case err = <-exited:
		case <-e.done:
			reap := time.NewTimer(reapWait)
			defer reap.Stop()
			select {
			case err = <-exited:
			case <-reap.C:
				return e.outlived(ticket)
			}
		}
	}
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return e.afterDeath(ticket)
	}
	if exit != nil && !exit.Exited() {
		return e.afterDeath(ticket)
	}
	if stdout.over || stderr.over {
		return substitute(overBound)
	}
	return childAnswer{stdout: stdout.String(), stderr: stderr.String(), code: command.ProcessState.ExitCode()}
}

// ownChild adds or drops a child the endpoint started: its confirmation is
// taken from it, though it runs below the wrapper rather than the harness.
func (e *Endpoint) ownChild(pid int, start uint64, running bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.children == nil {
		e.children = map[int]uint64{}
	}
	if running {
		e.children[pid] = start
	} else {
		delete(e.children, pid)
	}
}

// isOwnChild says whether pid is a call's child the endpoint runs now, by
// its pid and start time.
func (e *Endpoint) isOwnChild(pid int) bool {
	e.mu.Lock()
	known, ok := e.children[pid]
	e.mu.Unlock()
	if !ok || known == 0 {
		return false
	}
	start, err := proc.StartTime(pid)
	return err == nil && start == known
}

// startFailed is what a call answers when its child could not start.
const startFailed = "Rewake: the tool could not start its command, so nothing ran; run the same words in the shell.\n"

// afterDeath says what a child that died without an answer left, from its
// binding: the operation the call held, proof that it held none, or nothing
// known.
func (e *Endpoint) afterDeath(ticket bridge.Ticket) childAnswer {
	token, err := receipt.Bound(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch, bridge.CallKey(ticket.Transport, ticket.Conversation, ticket.CallID))
	switch {
	case err == nil:
		return substitute(fmt.Sprintf("Rewake: the call ended before it answered, holding operation %s; its outcome is unknown. Run: rewake retry %s\n", token, token))
	case errors.Is(err, receipt.ErrUnbound):
		return substitute("Rewake: the call ended before it began any operation, so nothing ran; run the same words again, here or in the shell.\n")
	default:
		return substitute("Rewake: the call ended before it answered, and what it began cannot be read; its outcome is unknown. Run the same words again in this turn: they join whatever it began.\n")
	}
}

// outlived says what a call whose child outlived its kill left as the
// endpoint closed: the child may still run, so the outcome is unknown either
// way, and the journal decides it.
func (e *Endpoint) outlived(ticket bridge.Ticket) childAnswer {
	token, err := receipt.Bound(e.cfg.Dir, e.cfg.Name, e.cfg.Epoch, bridge.CallKey(ticket.Transport, ticket.Conversation, ticket.CallID))
	if err == nil {
		return substitute(fmt.Sprintf("Rewake: the call's command outlived its kill, holding operation %s; its outcome is unknown. Run: rewake retry %s\n", token, token))
	}
	return substitute("Rewake: the call's command outlived its kill, so its outcome is unknown. Run the same words again in this turn: they join whatever it began.\n")
}

// ChildEnv is the environment of a call's child: the launch values, and PATH,
// HOME and LANG. Nothing else of the harness's environment reaches it.
func ChildEnv(getenv func(string) string) []string {
	var env []string
	for _, name := range []string{state.DirEnv, state.RoomEnv, state.SessionEnv, state.EpochEnv, "PATH", "HOME", "LANG"} {
		if value := getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return append(env, state.FaultEnv()...)
}
