package harness

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// A probe runs a person's program the way the harness itself would, before
// the run is claimed, to learn something only that program can answer — its
// version (version.go). Each runs in a session of its own under a holder
// process, bounded, with its output captured to a bound and never shown;
// whatever it started is ended and reaped on every outcome, and a process
// that cannot be ended is an outcome of its own, which refuses the launch.

// ProbeSpec is one probe to run.
type ProbeSpec struct {
	// Label is the probe's fixed name in a diagnostic: "--version".
	Label string
	// Program, Args, Env and Dir start it, as the harness would be started.
	Program string
	Args    []string
	Env     []string
	Dir     string
	// Bound is how long the probe may take, from its start to its end.
	Bound time.Duration
	// Capture keeps the output to read; false discards it.
	Capture bool
}

// The outcome classes a diagnostic may name: never the program's output,
// which may carry what the person keeps private.
const (
	OutcomeBound        = "no answer within its bound"
	OutcomeUnrecognized = "an answer not recognized"
	OutcomeNoStart      = "could not start"
	OutcomeNotEnded     = "could not be ended"
)

// OutcomeExit is the class of a probe that exited with a code the reading
// did not expect.
func OutcomeExit(code int) string { return "exit code " + strconv.Itoa(code) }

// maxProbeOutput bounds what a probe's output keeps; past it the output is
// read and dropped, so the program never blocks on a full pipe.
const maxProbeOutput = 64 << 10

// endGrace is how long a probe's group has between SIGTERM and SIGKILL.
const endGrace = time.Second

// reapBound bounds the sweep for processes the probe left behind.
const reapBound = 3 * time.Second

// holderBound bounds what the holder itself takes: starting the program,
// and ending once its tree is ended.
const holderBound = 5 * time.Second

// Probe is a started probe.
type Probe struct {
	holder   *exec.Cmd
	pid      int
	held     chan struct{}
	control  *os.File
	deadline time.Time
	output   *boundedBuffer
	reader   *os.File
	copied   chan struct{}
	exited   chan struct{}
	status   syscall.WaitStatus
	lost     bool
	result   chan string
	ended    sync.Once
	endErr   error
}

// StartProbe starts a probe under a holder of its own: a process that
// starts nothing but the probe and is the subreaper of its tree, so whatever
// the probe leaves, in any group or session, is the holder's to end — and
// nothing of this process's own is ever taken for it.
func StartProbe(spec ProbeSpec) (*Probe, error) {
	p := &Probe{
		output: &boundedBuffer{limit: maxProbeOutput},
		held:   make(chan struct{}), copied: make(chan struct{}), exited: make(chan struct{}), result: make(chan string, 1),
	}
	order, err := json.Marshal(holderOrder{Program: spec.Program, Args: spec.Args, Env: spec.Env, Dir: spec.Dir})
	if err != nil {
		return nil, err
	}
	var opened []*os.File
	pipe := func() (*os.File, *os.File) {
		r, w, perr := os.Pipe()
		if perr != nil && err == nil {
			err = perr
		}
		opened = append(opened, r, w)
		return r, w
	}
	reader, writer := pipe()
	controlR, controlW := pipe()
	reportR, reportW := pipe()
	closeAll := func() {
		for _, f := range opened {
			if f != nil {
				_ = f.Close()
			}
		}
	}
	if err != nil {
		closeAll()
		return nil, err
	}
	holder := &exec.Cmd{
		Path: "/proc/self/exe", Args: []string{holderName},
		Stdout: writer, Stderr: writer, ExtraFiles: []*os.File{controlR, reportW},
		// A group of its own: a signal to this process's group is not one
		// to end the probe, and the holder ends it only when told or orphaned.
		SysProcAttr: &syscall.SysProcAttr{Setpgid: true},
	}
	started := time.Now()
	if err := holder.Start(); err != nil {
		closeAll()
		return nil, err
	}
	_ = writer.Close()
	_ = controlR.Close()
	_ = reportW.Close()
	p.holder, p.control, p.reader = holder, controlW, reader
	p.deadline = started.Add(spec.Bound)
	go func() {
		defer close(p.held)
		_ = holder.Wait()
	}()
	go func() {
		defer close(p.copied)
		if spec.Capture {
			_, _ = io.Copy(p.output, reader)
			return
		}
		_, _ = io.Copy(io.Discard, reader)
	}()
	answer := make(chan bool, 1)
	go p.readReports(reportR, answer)
	if _, err := controlW.Write(append(order, '\n')); err == nil {
		select {
		case ok := <-answer:
			if ok {
				return p, nil
			}
		case <-time.After(holderBound):
		}
	}
	if p.End() != nil {
		return nil, ErrProbeNotEnded
	}
	return nil, errors.New("the probe could not be started")
}

// ErrProbeNotEnded is a probe that could not be started and whose holder
// could not show that everything it began has ended: what a caller that
// reads an unknown outcome as harmless must still refuse on.
var ErrProbeNotEnded = errors.New("the probe could not be started, and what it began could not be shown to have ended")

// readReports follows what the holder says: whether the program started,
// how it exited, and how its tree ended. A holder that is gone says no more:
// the program's exit is then unknown, and so is its tree.
func (p *Probe) readReports(reports *os.File, started chan<- bool) {
	defer func() { _ = reports.Close() }()
	exited := false
	scanner := bufio.NewScanner(reports)
	for scanner.Scan() {
		word, value, _ := strings.Cut(scanner.Text(), " ")
		switch word {
		case reportStarted:
			p.pid, _ = strconv.Atoi(value)
			started <- true
		case reportNoStart:
			started <- false
		case reportExit:
			if status, err := strconv.ParseUint(value, 10, 32); err == nil && !exited {
				p.status, exited = syscall.WaitStatus(status), true
				close(p.exited)
			}
		case reportEnded, reportLeft:
			p.result <- scanner.Text()
		}
	}
	select {
	case started <- false:
	default:
	}
	if !exited {
		p.lost = true
		close(p.exited)
	}
	close(p.result)
}

// Deadline is when the probe's bound passes.
func (p *Probe) Deadline() time.Time { return p.deadline }

// Exited closes when the probe's own process has exited: a program that
// exits before it answers will not answer.
func (p *Probe) Exited() <-chan struct{} { return p.exited }

// Wait waits for the probe's own process to exit within its bound. It
// answers its exit code, or false when the bound passed first.
func (p *Probe) Wait() (int, bool) {
	timer := time.NewTimer(time.Until(p.deadline))
	defer timer.Stop()
	select {
	case <-p.exited:
	case <-timer.C:
		return 0, false
	}
	switch {
	case p.lost:
	case p.status.Exited():
		return p.status.ExitStatus(), true
	case p.status.Signaled():
		return 128 + int(p.status.Signal()), true
	}
	return -1, true
}

// Output is what the probe printed, up to its bound. Read only after End.
func (p *Probe) Output() []byte { return p.output.Bytes() }

// End has the holder end the probe's group — SIGTERM, then SIGKILL a second
// later — and then everything else of its tree, and waits for the holder to
// say so. An error says something could not be ended, or that the holder
// could not tell: the launch is refused, since it would run beside it.
func (p *Probe) End() error {
	p.ended.Do(func() { p.endErr = p.end() })
	return p.endErr
}

func (p *Probe) end() error {
	_, _ = p.control.Write([]byte(orderEnd + "\n"))
	var result string
	select {
	case result = <-p.result:
	case <-time.After(2*endGrace + reapBound + holderBound):
	}
	_ = p.control.Close()
	select {
	case <-p.held:
	case <-time.After(holderBound):
		// A holder that does not end is ended: what it held is then
		// beyond this process's knowledge, which the error says.
		_ = syscall.Kill(-p.holder.Process.Pid, syscall.SIGKILL)
		<-p.held
		result = ""
	}
	// The pipe is closed once nothing writes to it any more; a writer that
	// could not be ended would otherwise hold the copy forever.
	_ = p.reader.Close()
	<-p.copied
	switch {
	case result == reportEnded:
		return nil
	case strings.HasPrefix(result, reportLeft+" "):
		return fmt.Errorf("%s processes it started could not be ended", strings.TrimPrefix(result, reportLeft+" "))
	}
	return errors.New("its holder did not say how its tree ended")
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
