package launch

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/proc"
)

// script writes a shell program for a probe to run.
func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "program")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// alive says whether a process runs: a zombie was ended, whoever reaps it.
func alive(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return false
	}
	state, err := proc.State(pid)
	return err == nil && state != "Z"
}

// pidFrom reads a pid a program wrote, waiting for it.
func pidFrom(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if pid, perr := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && perr == nil && pid > 0 {
			return pid
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no pid in %s", path)
	return 0
}

func TestAProbeThatExitsGivesItsCodeAndBoundedOutput(t *testing.T) {
	program := script(t, `head -c 200000 /dev/zero | tr '\0' x; echo; exit 3`)
	got := runProbe(program, nil, os.Environ(), t.TempDir(), 5*time.Second)
	if !got.Started || !got.Answered || got.Code != 3 {
		t.Fatalf("%+v", got)
	}
	if len(got.Output) != maxProbeOutput {
		t.Fatalf("kept %d bytes, want the bound %d", len(got.Output), maxProbeOutput)
	}
}

// A probe past its bound is killed with its group: a child it left in the
// group, which ignores SIGTERM, goes too.
func TestAProbePastItsBoundIsEndedWithItsGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	body := `sh -c 'echo $$ > ` + marker + `; trap "" TERM; while :; do sleep 1; done' &
trap '' TERM; while :; do sleep 1; done`
	started := time.Now()
	got := runProbe(script(t, body), nil, os.Environ(), t.TempDir(), 300*time.Millisecond)
	if !got.Started || got.Answered {
		t.Fatalf("a hanging probe answered within its bound: %+v", got)
	}
	if elapsed := time.Since(started); elapsed > 300*time.Millisecond+2*pipeGrace {
		t.Fatalf("the probe took %s", elapsed)
	}
	if child := pidFrom(t, marker); alive(child) {
		_ = syscall.Kill(child, syscall.SIGKILL)
		t.Fatalf("process %d of the probe's group outlived it", child)
	}
}

// A process that left the group still holding the output keeps the probe no
// longer than its grace.
func TestAProbeIsNotHeldByItsOutputPastTheGrace(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pid")
	body := `setsid sh -c 'echo $$ > ` + marker + `; sleep 30' &
echo 1.2.3`
	started := time.Now()
	got := runProbe(script(t, body), nil, os.Environ(), t.TempDir(), 5*time.Second)
	t.Cleanup(func() { _ = syscall.Kill(pidFrom(t, marker), syscall.SIGKILL) })
	if !got.Started || !got.Answered || got.Code != 0 || !strings.Contains(string(got.Output), "1.2.3") {
		t.Fatalf("%+v", got)
	}
	if elapsed := time.Since(started); elapsed > 3*pipeGrace {
		t.Fatalf("the probe waited %s for its output", elapsed)
	}
}

func TestAProbeThatCannotStartSaysSo(t *testing.T) {
	if got := runProbe(filepath.Join(t.TempDir(), "absent"), nil, os.Environ(), t.TempDir(), time.Second); got.Started {
		t.Fatalf("a missing program started: %+v", got)
	}
}
