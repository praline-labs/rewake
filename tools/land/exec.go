package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// command is one program to run: where, with what added to the environment,
// and for how long at most.
type command struct {
	dir   string
	env   []string
	name  string
	args  []string
	limit time.Duration
}

func (c command) String() string {
	return strings.Join(append(append(append([]string{}, c.env...), c.name), c.args...), " ")
}

// result is what a command did. err is set only when it did not run to an
// exit of its own — not found, killed at its time limit — and code is then -1.
type result struct {
	stdout, stderr string
	code           int
	err            error
}

func (r result) ok() bool { return r.err == nil && r.code == 0 }

// why is the shortest account of a failure: the error, else the end of what
// the command said.
func (r result) why() string {
	if r.err != nil {
		return r.err.Error()
	}
	text := strings.TrimSpace(r.stderr)
	if text == "" {
		text = strings.TrimSpace(r.stdout)
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 4 {
		lines = lines[len(lines)-4:]
	}
	return fmt.Sprintf("exit %d: %s", r.code, strings.Join(lines, "\n  "))
}

// runner runs one command, writing what it prints to log as well; the tests
// replace it with a fake git, go and bun.
type runner func(ctx context.Context, c command, log io.Writer) result

// execute is the real runner.
func execute(ctx context.Context, c command, log io.Writer) result {
	ctx, cancel := context.WithTimeout(ctx, c.limit)
	defer cancel()
	_, _ = fmt.Fprintf(log, "\n$ %s\n", c)
	cmd := exec.CommandContext(ctx, c.name, c.args...)
	cmd.Dir = c.dir
	cmd.Env = append(childEnv(), c.env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = io.MultiWriter(&stdout, log), io.MultiWriter(&stderr, log)
	err := cmd.Run()
	out := result{stdout: stdout.String(), stderr: stderr.String()}
	var exited *exec.ExitError
	switch {
	case err == nil:
	case ctx.Err() != nil:
		out.code, out.err = -1, errors.New("no answer within "+c.limit.String())
	case errors.As(err, &exited):
		out.code = exited.ExitCode()
	default:
		out.code, out.err = -1, err
	}
	_, _ = fmt.Fprintf(log, "[exit %d]\n", out.code)
	return out
}

// childEnv is this process's environment without the four variables that
// name a session (AGENTS.md, "Checks"): go test would otherwise write into
// the owner's live state or read the running session as its own.
func childEnv() []string {
	var env []string
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "REWAKE_SESSION", "REWAKE_EPOCH", "REWAKE_DIR", "REWAKE_ROOM":
			continue
		}
		env = append(env, entry)
	}
	return env
}
