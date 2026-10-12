package worktree

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// hookWait bounds how long land waits for a git call that runs the
// repository's hooks. A hook is the repository's code, and one that waits for
// something would hang the agent that called land, with no line saying why.
// A minute: finish holds the repository's lock through its landing, and a
// launch waits for that lock no longer (serial.go).
var hookWait = time.Minute

// Test hooks for land: beforeLook runs before the checkout holding the target
// is asked which branch it has out, beforeMerge between that look and the
// merge.
var (
	beforeLook  = func(string) {}
	beforeMerge = func(string) {}
)

// HookWaitError is a git call that ran the repository's hooks and had not
// ended when land stopped waiting. git is not stopped: a merge cut short
// between moving the files and moving the branch would leave the checkout
// changed under a branch that did not move. It goes on in a session of its
// own, and what it does may still land.
type HookWaitError struct {
	What string
	// Hook names what git was running when land stopped waiting: the hook,
	// or the command line of whatever it started, "" when nothing was seen.
	Hook   string
	PID    int
	Waited time.Duration
}

func (e *HookWaitError) Error() string {
	running := "git itself"
	if e.Hook != "" {
		running = e.Hook
	}
	return fmt.Sprintf("%s had not finished after %s, and %s was still running; rewake stopped waiting, and git (process %d) goes on by itself, so what it does may still land: when it has ended, run land again to see", e.What, e.Waited, running, e.PID)
}

// runWithHooks runs git as a person would in their own checkout, hooks
// included: land moves the source's branch, and the hooks the repository keeps
// for a merge or a ref update are the person's to run there. what names the
// call for a refusal. Its output goes to files, not pipes: a git left running
// after the wait must not meet a closed pipe once this process has gone, which
// could fail a hook half way through the merge it follows.
func runWithHooks(what string, command *exec.Cmd) (string, error) {
	prepare(command, true)
	stdout, err := outputFile()
	if err != nil {
		return "", err
	}
	defer func() { _ = stdout.Close() }()
	stderr, err := outputFile()
	if err != nil {
		return "", err
	}
	defer func() { _ = stderr.Close() }()
	command.Stdout, command.Stderr = stdout, stderr
	if err := command.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	timer := time.NewTimer(hookWait)
	defer timer.Stop()
	select {
	case err := <-done:
		return finished(err, written(stdout), written(stderr))
	case <-timer.C:
		pid := command.Process.Pid
		return "", &HookWaitError{What: what, Hook: runningHook(pid), PID: pid, Waited: hookWait}
	}
}

// outputFile is a file for one git call's output, unlinked at once: the
// descriptors keep it for git and for the read after it, and nothing is left
// behind whether or not git outlives this process.
func outputFile() (*os.File, error) {
	file, err := os.CreateTemp("", "rewake-git-*")
	if err != nil {
		return nil, fmt.Errorf("cannot make a file for git's output: %w", err)
	}
	_ = os.Remove(file.Name())
	return file, nil
}

// written reads back what git wrote to one of its output files, through the
// descriptor, as the name is gone.
func written(file *os.File) string {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	text, _ := io.ReadAll(file)
	return string(text)
}

// runningHook names what git, which leads a session of its own, is running
// below it: the hook, by the name git ran it under, when an argument is a path
// in a hooks directory; else the command line of a process git started; else
// "". Read from /proc, best effort.
func runningHook(session int) string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return ""
	}
	fallback := ""
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == session {
			continue
		}
		stat, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		// pid (comm) state ppid pgrp session ...; comm may hold spaces and
		// parentheses, so the fields are counted after its last ")".
		at := strings.LastIndexByte(string(stat), ')')
		fields := strings.Fields(string(stat[at+1:]))
		if at < 0 || len(fields) < 4 || fields[3] != strconv.Itoa(session) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(strings.TrimRight(string(raw), "\x00"), "\x00")
		for _, arg := range args {
			if strings.Contains("/"+arg, "/hooks/") {
				return "the repository's " + filepath.Base(arg) + " hook"
			}
		}
		if fallback == "" && fields[1] == strconv.Itoa(session) {
			fallback = "what git started, " + strings.Join(args, " ")
		}
	}
	return fallback
}
