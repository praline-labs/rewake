package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// result is what a command did. err is set only when it did not run to an
// exit of its own — not found, killed at its time limit — and code is then -1.
type result struct {
	stdout, stderr string
	code           int
	err            error
}

// ok is a command that ran and exited 0.
func (r result) ok() bool { return r.err == nil && r.code == 0 }

// why is the shortest account of a failure: the error, else the end of what
// the command said on standard error.
func (r result) why() string {
	if r.err != nil {
		return r.err.Error()
	}
	text := strings.TrimSpace(r.stderr)
	if text == "" {
		text = strings.TrimSpace(r.stdout)
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	return "exit " + strconv.Itoa(r.code) + ": " + strings.Join(lines, "\n  ")
}

// runner runs one command in dir with a time limit; the tests replace it with
// a fake npm and git.
type runner func(ctx context.Context, limit time.Duration, dir string, name string, args ...string) result

// execute is the real runner. Every child gets GIT_OPTIONAL_LOCKS=0: the gate
// only reads the repository, and a git status or a VCS-stamping go build
// would otherwise refresh the index behind it.
func execute(ctx context.Context, limit time.Duration, dir, name string, args ...string) result {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.Env = childEnv()
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	out := result{stdout: stdout.String(), stderr: stderr.String()}
	var exited *exec.ExitError
	switch {
	case err == nil:
	case ctx.Err() != nil:
		out.code, out.err = -1, errors.New("no answer within "+limit.String())
	case errors.As(err, &exited):
		out.code = exited.ExitCode()
	default:
		out.code, out.err = -1, err
	}
	return out
}

// childEnv is this process's environment without rewake's session variables,
// which a test or a built rewake would otherwise take for its own session.
func childEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "REWAKE_SESSION", "REWAKE_EPOCH", "REWAKE_DIR", "REWAKE_ROOM", "GIT_OPTIONAL_LOCKS":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "GIT_OPTIONAL_LOCKS=0")
}
