package workflow

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestFixtureWrapperKilled is a fixture wrapper killed outright: the session
// half it started ends with it. The backend asks for no parent-death signal,
// which the exec of a wrapper handing its run over would fire, so the
// fixture's program must see its wrapper go by itself (found in review,
// November 2, 2026: the session half outlived its wrapper by its 85 seconds'
// stay). The foreground half, which runs without a terminal here and had no
// such signal before either, is not judged: the scenario ends it.
func TestFixtureWrapperKilled(t *testing.T) {
	binary := enterScenario(t, "fixture-wrapper-killed")
	c := Start(t, Spec{Name: "fixture-wrapper-killed", Harness: fixtureColumn.harness, Observations: []string{obsKilledWrapperChildren}, Deadline: 60 * time.Second})
	iso := Isolate(t, c, binary)
	s, command := sessionLaunch(t, c, iso, "", fixtureColumn.harness, "lead", "--main", nil, nil, staysUp(iso, "lead"))
	release := c.captureStderr(command, iso.Home, "lead")
	// Expected to fail: the scenario kills it.
	process, err := c.start(command, true)
	release()
	if err != nil {
		t.Fatal(err)
	}
	s.home, s.process = iso.Home, process
	if !waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(s.ready); return err == nil }) {
		c.Contradicted(obsKilledWrapperChildren, "the session never became ready")
		stopSession(t, c, s)
		return
	}
	record := reexecRecordOf(iso, s.name)
	wrapper := record.ServicePID
	children := childrenOf(wrapper)
	// Whatever still runs goes before the case's own cleanup looks; so do the
	// run's socket and record, which a wrapper killed outright cannot remove:
	// the next launch of the name replaces the record, and the socket's path
	// is the run's, so no later run meets it.
	defer func() {
		_ = os.Remove(record.Socket)
		_ = os.Remove(record.path)
		for _, pid := range children {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
		waitFor(c, 5*time.Second, func() bool {
			for _, pid := range children {
				if alive(pid) {
					return false
				}
			}
			return true
		})
	}()
	backend := 0
	for _, pid := range children {
		if raw, _ := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid)); strings.Contains(string(raw), "--connect") {
			backend = pid
		}
	}
	if backend == 0 {
		c.Contradicted(obsKilledWrapperChildren, "no session half among the wrapper's children %v", children)
		return
	}
	if err := syscall.Kill(wrapper, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if !waitFor(c, 3*time.Second, func() bool { return !alive(backend) }) {
		c.Contradicted(obsKilledWrapperChildren, "wrapper %d killed; its session half %d still runs 3 s later", wrapper, backend)
		return
	}
	c.Observed(obsKilledWrapperChildren, fmt.Sprintf("wrapper %d killed; its session half %d ended", wrapper, backend))
}

const obsKilledWrapperChildren = "a fixture wrapper killed outright: the session half it started ends within three seconds"

// endWithWrapper ends the session half once the wrapper that started it is
// gone, as a harness's program ends with its wrapper: the backend asks for no
// parent-death signal, which the exec of a wrapper handing its run over would
// fire, so the half watches its parent's pid, which that exec keeps.
func endWithWrapper() {
	parent := os.Getppid()
	for range time.Tick(200 * time.Millisecond) {
		if os.Getppid() != parent {
			os.Exit(0)
		}
	}
}
