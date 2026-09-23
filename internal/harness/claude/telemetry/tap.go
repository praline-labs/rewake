package telemetry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// Shell is what Claude Code runs a status-line command with: `/bin/sh -c`,
// in the project directory, with the status JSON on stdin (seen in a
// container on 2.1.280, docs/research.md). The tap is itself run that way, so
// running the person's command the same way from inside it changes nothing
// they could notice.
const Shell = "/bin/sh"

// RunTap is the status-line command rewake installs in front of the person's
// own. It tells the wrapper what the status line was handed, finds the
// person's command (statusline.go), then becomes it: same shell, directory,
// environment and stdin, its output and exit code unchanged, under the
// harness's own timeout. With no command configured it prints nothing, which
// is what the harness shows without one.
//
// Nothing of rewake's may stand in the way of the person's display. Parsing
// that fails, a wrapper that is gone and a format that changed all end the
// same way: the command runs as if rewake were not there.
func RunTap(socket, sources, caller string) int {
	payload := readStatus(os.Stdin)
	report(socket, payload)
	command := ownerCommand(sources, caller)
	if command == "" {
		return 0
	}
	if err := becomeShell(payload, command); err != nil {
		// The exec did not happen — the payload outgrew a pipe, or the
		// kernel refused — so the command runs as a child instead.
		return runShell(payload, command, os.Stdout, os.Stderr)
	}
	return 0
}

// ownerCommand finds the person's status line from the harness's environment
// and directory, as they are now.
func ownerCommand(sources, caller string) (command string) {
	defer func() {
		if recover() != nil {
			command = ""
		}
	}()
	env := os.Environ()
	return ResolveStatusCommand(env, ProjectDir(env), sources, json.RawMessage(caller))
}

// report sends what the status line says, and survives anything doing so
// might throw: the person's command comes next whatever happened here.
func report(socket string, payload []byte) {
	defer func() { _ = recover() }()
	if event, ok := DecodeStatus(payload); ok {
		Send(socket, event)
	}
}

// readStatus reads everything the harness wrote, because all of it goes on to
// the person's command. Never from a terminal: run by hand, the tap would
// otherwise wait for a person.
func readStatus(input *os.File) []byte {
	if info, err := input.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
		return nil
	}
	payload, _ := io.ReadAll(input)
	return payload
}

// becomeShell replaces this process with the shell, its stdin a pipe already
// holding the payload. Replacing rather than starting a child keeps the
// process the harness spawned the one that runs: a timeout or an abort that
// signals it reaches the person's command, not a wrapper around it.
//
// A pipe holds only so much before a write waits for a reader, and after the
// exec there is nobody left to write the rest; so a payload larger than the
// pipe can be made to hold is refused here and handed to runShell.
func becomeShell(payload []byte, command string) error {
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	if !fitsPipe(writer, len(payload)) {
		_ = writer.Close()
		return errors.New("the status payload does not fit a pipe")
	}
	if _, err := writer.Write(payload); err != nil {
		_ = writer.Close()
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := syscall.Dup3(int(reader.Fd()), 0, 0); err != nil {
		return err
	}
	return syscall.Exec(Shell, []string{Shell, "-c", command}, os.Environ())
}

// maxTapPipe is the payload becomeShell takes. A test lowers it to reach the
// other path; the kernel's own limit is checked beside it.
var maxTapPipe = 1 << 20

// fitsPipe asks the kernel to make the pipe large enough, and answers whether
// it is. A status payload is a few kilobytes; the default of 64 KiB is
// already plenty, and growing it is for a format that grew.
func fitsPipe(pipe *os.File, size int) bool {
	if size > maxTapPipe {
		return false
	}
	const setSize, getSize = 1031, 1032 // F_SETPIPE_SZ, F_GETPIPE_SZ
	fd := pipe.Fd()
	capacity, _, errno := syscall.Syscall(syscall.SYS_FCNTL, fd, getSize, 0)
	if errno != 0 {
		return false
	}
	if int(capacity) >= size {
		return true
	}
	capacity, _, errno = syscall.Syscall(syscall.SYS_FCNTL, fd, setSize, uintptr(size))
	return errno == 0 && int(capacity) >= size
}

// runShell runs the command as a child and answers its exit code the way a
// shell reports it. Termination requests to the tap are passed on, so the
// child does not outlive a status line the harness has given up on.
func runShell(payload []byte, command string, stdout, stderr io.Writer) int {
	child := exec.Command(Shell, "-c", command)
	child.Stdin = bytes.NewReader(payload)
	child.Stdout, child.Stderr = stdout, stderr
	if err := child.Start(); err != nil {
		return 127
	}
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer func() {
		// Stop promises no further sends, so closing ends the forwarder.
		signal.Stop(signals)
		close(signals)
	}()
	go func() {
		for received := range signals {
			_ = child.Process.Signal(received)
		}
	}()
	err := child.Wait()
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		return 1
	}
	if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal())
	}
	return exit.ExitCode()
}
