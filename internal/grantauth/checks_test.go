package grantauth

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/proc"
)

// realStranger is a process that is not this one, as it started: a pid and
// start time that pass every liveness check, so only the pid comparison can
// refuse it.
func realStranger(t *testing.T) Expect {
	t.Helper()
	pid := stranger(t)
	start, err := proc.StartTime(pid)
	if err != nil {
		t.Fatal(err)
	}
	return Expect{PID: pid, Start: start}
}

// An answer from this process, when the sender's record names another one
// that is alive as it started, is somebody else's: the listener's pid is the
// check, not whether the named process runs.
func TestAnAnswerFromAnotherLiveProcessIsRefused(t *testing.T) {
	_, path := listen(t, os.Getpid(), time.Minute)
	if err := Register(path, lib); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(path, realStranger(t), lib.ID, lib.To, lib.ToEpoch, "c1"); !errors.Is(err, ErrNotConfirmed) || !strings.Contains(err.Error(), "not the sending session's wrapper") {
		t.Fatalf("confirmed by another process: %v", err)
	}
}

// The hook believes only its own wrapper, even when the run it names is a
// live process: the keeper answering is not that process.
func TestAHookRefusesAnAnswerFromAnotherLiveProcess(t *testing.T) {
	_, path, _ := keep(t, os.Getpid(), nil, nil)
	if output, err := Ask(path, realStranger(t), json.RawMessage(`"add"`)); !errors.Is(err, ErrNotConfirmed) || output != nil {
		t.Fatalf("asked another process: %s %v", output, err)
	}
}

// Two grants differ when one is broad where the other is not, or gives Git
// where the other does not: a message carrying either difference is not the
// grant main registered.
func TestGrantsDifferingOnlyInBreadthOrGitAreNotTheSame(t *testing.T) {
	narrower := lib
	narrower.Broad = nil
	withGit := lib
	withGit.Git = true
	reordered := lib
	reordered.Dirs = []string{lib.Dirs[1], lib.Dirs[0]}
	for name, other := range map[string]Grant{"not broad": narrower, "with Git": withGit} {
		if lib.Same(other) || other.Same(lib) {
			t.Errorf("%s: the same", name)
		}
	}
	if !lib.Same(reordered) {
		t.Error("the same directories in another order differ")
	}
}

// A wrapper in a sandbox of its own names a run and is not it: neither a
// confirm nor a hand-over from it counts as the run's wrapper asking.
func TestARunInASandboxOfItsOwnIsNotItsWrapper(t *testing.T) {
	run := ownRun(t)
	t.Run("a confirm delivers nothing", func(t *testing.T) {
		authority, path := listen(t, os.Getpid(), time.Minute)
		authority.Open = func(Grant) (bool, bool) { return true, true }
		granted := lib
		granted.ToEpoch = run
		if err := Register(path, granted); err != nil {
			t.Fatal(err)
		}
		restore := sandboxed(t)
		answer := raw(t, path, request{Op: opConfirm, Grant: Grant{ID: granted.ID, To: granted.To, ToEpoch: run}, Thread: "c1"})
		restore()
		if answer.Error != "" || answer.Grant == nil {
			t.Fatalf("the confirm: %+v", answer)
		}
		if _, err := Reconfirm(path, ownExpect(t), lib.ID, lib.To, run, "c1"); !errors.Is(err, ErrNotConfirmed) || !strings.Contains(err.Error(), "never delivered") {
			t.Fatalf("a sandboxed confirm delivered the grant: %v", err)
		}
	})
	t.Run("a hand-over is refused", func(t *testing.T) {
		path, _ := delivered(t, time.Minute)
		restore := sandboxed(t)
		answer := raw(t, path, request{Op: opReconfirm, Grant: Grant{ID: lib.ID, To: lib.To, ToEpoch: run}, Thread: "c1"})
		restore()
		if answer.Grant != nil || !strings.Contains(answer.Error, "sandbox of its own") {
			t.Fatalf("handed over to a sandbox: %+v", answer)
		}
		if _, err := Reconfirm(path, ownExpect(t), lib.ID, lib.To, run, "c1"); err != nil {
			t.Fatalf("the run outside the sandbox: %v", err)
		}
	})
}

// raw sends one request as it is, past the client's own checks.
func raw(t *testing.T, path string, asked request) response {
	t.Helper()
	conn, err := dial(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	answer, err := exchange(conn, asked)
	if err != nil {
		t.Fatal(err)
	}
	return answer
}

// answerers numbers the listeners, each at an address of its own.
var answerers int

// answering listens as this process and gives every request the same answer:
// a wrapper that passes every check on who it is, and says what it likes.
func answering(t *testing.T, answer string) string {
	t.Helper()
	answerers++
	path := "@rewake-test/answering/" + t.Name() + "/" + strconv.Itoa(os.Getpid()) + "/" + strconv.Itoa(answerers)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			_, _ = bufio.NewReader(io.LimitReader(conn, maxLine)).ReadBytes('\n')
			_, _ = fmt.Fprintln(conn, answer)
			_ = conn.Close()
		}
	}()
	return path
}

// What the wrapper answers is checked too: a hand-over naming another grant
// than the one asked for, or none, is not taken, and neither is a confirm
// naming none.
func TestAnAnswerNamingAnotherGrantIsNotTaken(t *testing.T) {
	run := ownRun(t)
	for name, answer := range map[string]string{
		"another message":   `{"grant":{"id":"m2","to":"worker","toEpoch":"` + run + `"}}`,
		"another recipient": `{"grant":{"id":"m1","to":"other","toEpoch":"` + run + `"}}`,
		"another run":       `{"grant":{"id":"m1","to":"worker","toEpoch":"1.1"}}`,
		"no grant":          `{}`,
	} {
		path := answering(t, answer)
		if got, err := Reconfirm(path, ownExpect(t), "m1", "worker", run, "c1"); !errors.Is(err, ErrNotConfirmed) {
			t.Errorf("%s: handed over %+v, %v", name, got, err)
		}
	}
	if got, err := Confirm(answering(t, `{}`), ownExpect(t), "m1", "worker", run, "c1"); !errors.Is(err, ErrNotConfirmed) || !strings.Contains(err.Error(), "named no grant") {
		t.Errorf("a confirm naming no grant: %+v, %v", got, err)
	}
}
