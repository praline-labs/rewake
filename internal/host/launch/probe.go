package launch

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// A probe runs a person's program the way the harness itself would, before
// the run is claimed, to learn something only that program can answer — its
// version (version.go). It runs in a process group of its own, bounded, with
// its output captured to a bound and never shown; the group is killed on every
// outcome, so nothing it started in that group outlives it.

// The outcome classes a diagnostic may name: never the program's output,
// which may carry what the person keeps private.
const (
	outcomeBound   = "no answer within its bound"
	outcomeNoStart = "could not start"
)

// outcomeExit is the class of a probe that exited with a code the reading
// did not expect.
func outcomeExit(code int) string { return "exit code " + strconv.Itoa(code) }

// maxProbeOutput bounds what a probe's output keeps; past it the output is
// read and dropped, so the program never blocks on a full pipe.
const maxProbeOutput = 64 << 10

// pipeGrace is how long the output may stay open after the program ended or
// was killed: a process that left the group still holding the pipe does not
// keep the launch waiting.
const pipeGrace = time.Second

// probeResult is how a probe went.
type probeResult struct {
	Output []byte
	// Code is the exit code, 128 plus the signal for one a signal ended.
	Code int
	// Started is false when the program could not be started.
	Started bool
	// Answered is false when the bound passed first.
	Answered bool
}

// runProbe runs program with args, env and dir within bound and kills its
// process group when it ends or the bound passes.
func runProbe(program string, args, env []string, dir string, bound time.Duration) probeResult {
	ctx, cancel := context.WithTimeout(context.Background(), bound)
	defer cancel()
	output := &boundedBuffer{limit: maxProbeOutput}
	cmd := exec.CommandContext(ctx, program, args...)
	cmd.Env, cmd.Dir, cmd.Stdout, cmd.Stderr = env, dir, output, output
	// A group of its own: a signal to this process's group is not one to
	// end the probe, and the probe's group is killed without touching ours.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = pipeGrace
	if err := cmd.Start(); err != nil {
		return probeResult{}
	}
	err := cmd.Wait()
	// Whatever it left running in its group goes with it.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	result := probeResult{Output: output.Bytes(), Started: true, Answered: ctx.Err() == nil}
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		status, _ := exit.Sys().(syscall.WaitStatus)
		if status.Signaled() {
			result.Code = 128 + int(status.Signal())
		} else {
			result.Code = exit.ExitCode()
		}
	case err != nil && !errors.Is(err, exec.ErrWaitDelay):
		result.Code = -1
	}
	return result
}

// boundedBuffer keeps the first limit bytes written and drops the rest.
type boundedBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.limit - b.buf.Len(); room > 0 {
		b.buf.Write(data[:min(room, len(data))])
	}
	return len(data), nil
}

func (b *boundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}
