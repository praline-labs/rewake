package server

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
	"github.com/praline-labs/rewake/internal/receipt"
	"github.com/praline-labs/rewake/internal/state"
)

// Running the child (docs/mail-bridge-server.md#running-the-child). Every
// effect of a call happens in it, under its receipt; the server only starts
// it, bounds it, and reads what it left. It never repeats, finishes or undoes
// what the child did.

// hardBound is how long past its deadline a child may live.
const hardBound = 5 * time.Second

// reapWait is how long, once stdin ended, the server waits for a killed
// child to be reaped: past it the call is answered as unknown and the server
// exits without it.
const reapWait = time.Second

// outputCap bounds what is kept of a child's stdout and stderr. A child bounds
// its own answer to one result; more than this is one that broke its bound,
// and its answer is replaced whole either way.
const outputCap = 64 << 10

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
// what the server can say of a child that left none.
func (s *Server) runChild(ticket bridge.Ticket, words []string) answer {
	reader, writer, err := os.Pipe()
	if err != nil {
		s.cannotStart()
		return substitute(startFailed)
	}
	encoded, _ := json.Marshal(ticket)
	command := exec.Command(s.cfg.Executable, words...)
	command.Env = append(append([]string(nil), s.cfg.Env...), bridge.TicketEnv+"=3")
	command.ExtraFiles = []*os.File{reader}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr capped
	command.Stdout, command.Stderr = &stdout, &stderr
	state.Step("start")
	err = command.Start()
	_ = reader.Close()
	if err != nil {
		_ = writer.Close()
		s.cannotStart()
		return substitute(startFailed)
	}
	// The ticket fits the pipe's buffer, and the child reads it to its end
	// within a second; the server never waits on it.
	_, _ = writer.Write(encoded)
	_ = writer.Close()

	exited := make(chan error, 1)
	go func() { exited <- command.Wait() }()
	bound := time.NewTimer(max(0, time.Duration(ticket.DeadlineBoot-boottime.Now())) + hardBound)
	defer bound.Stop()
	select {
	case err = <-exited:
	case <-bound.C:
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		// A child that does not die, or something of it that holds its
		// output, keeps its slot while the server serves. Once stdin ended
		// it is waited for only reapWait more.
		select {
		case err = <-exited:
		case <-s.ending:
			reap := time.NewTimer(reapWait)
			defer reap.Stop()
			select {
			case err = <-exited:
			case <-reap.C:
				return s.outlived(ticket)
			}
		}
	}
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return s.afterDeath(ticket)
	}
	if exit != nil && !exit.Exited() {
		return s.afterDeath(ticket)
	}
	if stdout.over || stderr.over {
		return substitute(overBound)
	}
	return answer{stdout: stdout.String(), stderr: stderr.String(), code: command.ProcessState.ExitCode()}
}

// cannotStart tells the run's wrapper the call's command could not start
// (docs/mail-bridge-channel.md#the-tool-observation): the one failure only the
// server sees. It waits for nothing; the call answers its model either way.
func (s *Server) cannotStart() {
	if client := s.connect(); client != nil {
		client.Report()
	}
}

// startFailed is what a call answers when its child could not start.
const startFailed = "Rewake: the tool could not start its command, so nothing ran; run the same words in the shell.\n"

// afterDeath says what a child that died without an answer left, from its
// binding: the operation the call held, proof that it held none, or nothing
// known.
func (s *Server) afterDeath(ticket bridge.Ticket) answer {
	token, err := receipt.Bound(s.cfg.Dir, s.cfg.Name, s.cfg.Epoch, bridge.CallKey(ticket.Transport, ticket.Conversation, ticket.CallID))
	switch {
	case err == nil:
		return substitute(fmt.Sprintf("Rewake: the call ended before it answered, holding operation %s; its outcome is unknown. Run: rewake retry %s\n", token, token))
	case errors.Is(err, receipt.ErrUnbound):
		return substitute("Rewake: the call ended before it began any operation, so nothing ran; run the same words again, here or in the shell.\n")
	default:
		return substitute("Rewake: the call ended before it answered, and what it began cannot be read; its outcome is unknown. Run the same words again in this turn: they join whatever it began.\n")
	}
}

// outlived says what a call whose child outlived its kill left, at the end of
// stdin. The child may still be running, so no binding missing yet proves
// that nothing ran: the outcome is unknown either way, and the journal
// decides it.
func (s *Server) outlived(ticket bridge.Ticket) answer {
	token, err := receipt.Bound(s.cfg.Dir, s.cfg.Name, s.cfg.Epoch, bridge.CallKey(ticket.Transport, ticket.Conversation, ticket.CallID))
	if err == nil {
		return substitute(fmt.Sprintf("Rewake: the call's command outlived its kill, holding operation %s; its outcome is unknown. Run: rewake retry %s\n", token, token))
	}
	return substitute("Rewake: the call's command outlived its kill, so its outcome is unknown. Run the same words again in this turn: they join whatever it began.\n")
}

// childEnv is the environment of every child: the launch values, and PATH,
// HOME and LANG. Nothing else of the harness's environment reaches it.
func childEnv(getenv func(string) string) []string {
	var env []string
	for _, name := range []string{state.DirEnv, state.RoomEnv, state.SessionEnv, state.EpochEnv, "PATH", "HOME", "LANG"} {
		if value := getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return append(env, state.FaultEnv()...)
}
