package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// runOf is a process as a run.
func runOf(t *testing.T, pid int) string {
	t.Helper()
	start, err := proc.StartTime(pid)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%d.%d", pid, start)
}

// endedRunOf is the run of a process that has exited.
func endedRunOf(t *testing.T) string {
	t.Helper()
	child := exec.Command("true")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	run := runOf(t, child.Process.Pid)
	_ = child.Wait()
	return run
}

// resumedMain serves, as main's wrapper, grants delivered to a run of the
// writer that has ended, and returns that run. Tasks named in closed are
// closed once delivered.
func resumedMain(t *testing.T, server *serverSession, grants map[string]string, closed ...string) string {
	t.Helper()
	address := state.AuthorityAddress(server.mailbox, server.epoch)
	authority, err := grantauth.Listen(address, os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var delivered atomic.Bool
	authority.Open = func(held grantauth.Grant) (bool, bool) {
		return !delivered.Load() || !slices.Contains(closed, held.ID), true
	}
	ctx, cancel := context.WithCancel(context.Background())
	go authority.Serve(ctx)
	t.Cleanup(func() { cancel(); authority.Close() })
	previous := endedRunOf(t)
	self := grantauth.Expect{PID: os.Getpid()}
	self.Start, _ = proc.StartTime(os.Getpid())
	for id, directory := range grants {
		if err := grantauth.Register(address, grantauth.Grant{ID: id, To: server.name, ToEpoch: previous, Dirs: []string{directory}}); err != nil {
			t.Fatal(err)
		}
		if _, err := grantauth.Confirm(address, self, id, server.name, previous, fixtureRoot); err != nil {
			t.Fatal(err)
		}
	}
	delivered.Store(true)
	return previous
}

// A cold resume leaves the thread with the roots it saved and this run with
// no journal. At the first notice the adapter asks main again: a confirmed
// grant is journaled and a root it lost added back; a root a copy names that
// nobody confirms is taken out, the launch directory excepted. Then the grant
// goes with its report as any other.
func TestAResumedThreadHasItsGrantsConfirmedAgain(t *testing.T) {
	workspace, lib, doc, forged := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	server, captured := gitDeliveryFixture(t, role.General,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, doc, forged}, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, lib}, "idle")},
	)
	dirGrantSession(t, server)
	server.epoch = runOf(t, os.Getpid())
	main := server.epoch
	previous := resumedMain(t, server, map[string]string{"m1": lib, "m2": doc}, "m2")
	waiting(t, server, "m1", true)
	copied := []grant.Entry{
		{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main},
		{Path: doc, Message: "m2", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main},
		{Path: forged, Message: "m3", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main},
		{Path: workspace, Message: "m3", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: main},
	}
	if err := grant.Save(server.mailbox, server.name, previous, copied); err != nil {
		t.Fatal(err)
	}

	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "n1", Kind: inbox.Note})
	if result.State != inbox.Delivered || !slices.Equal(roots, []string{workspace, lib}) {
		t.Fatalf("restore: roots=%q result=%+v", roots, result)
	}
	for _, want := range []string{"restored after the resume, confirmed again by lead: " + lib, "the grant of task m2 not confirmed again: " + doc, "m3 not confirmed again: " + forged} {
		if !strings.Contains(result.Detail, want) {
			t.Errorf("the detail does not say %q: %s", want, result.Detail)
		}
	}
	entries := journal(server)
	if got := outcomes(entries); !slices.Equal(got, []string{lib + "=granted"}) || entries[0].Thread != fixtureRoot || entries[0].FromEpoch != main {
		t.Fatalf("journal = %+v", entries)
	}

	// Looked at once: the next notice reads nothing while the task is open.
	if roots, result = deliverDirs(t, server, captured, inbox.Message{ID: "n2", Kind: inbox.Note}); roots != nil || result.Detail != "" {
		t.Fatalf("looked again: roots=%q result=%+v", roots, result)
	}

	// Reported on: the grant restored is taken back like any other.
	waiting(t, server, "m1", false)
	roots, result = deliverDirs(t, server, captured, inbox.Message{ID: "n3", Kind: inbox.Note})
	if !slices.Equal(roots, []string{workspace}) || !strings.Contains(result.Detail, "taken back, their tasks reported on: "+lib) {
		t.Fatalf("revoke: roots=%q result=%+v", roots, result)
	}
}

// While the main a copy names runs and does not answer, the roots stay as
// they are and the next notice asks again; once that main has ended, nobody
// can confirm the grant, and its root is taken out.
func TestAResumedThreadWaitsForAQuietMain(t *testing.T) {
	workspace, lib := t.TempDir(), t.TempDir()
	server, captured := gitDeliveryFixture(t, role.General,
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, lib}, "idle")},
		gitReadFixture{thread: gitThreadFixture(workspace, []string{workspace, lib}, "idle")},
	)
	dirGrantSession(t, server)
	server.epoch = runOf(t, os.Getpid())
	quiet := exec.Command("sleep", "60")
	if err := quiet.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = quiet.Process.Kill(); _ = quiet.Wait() })
	copied := []grant.Entry{{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: fixtureRoot, From: "lead", FromEpoch: runOf(t, quiet.Process.Pid)}}
	if err := grant.Save(server.mailbox, server.name, endedRunOf(t), copied); err != nil {
		t.Fatal(err)
	}

	roots, result := deliverDirs(t, server, captured, inbox.Message{ID: "n1", Kind: inbox.Note})
	if result.State != inbox.Delivered || roots != nil || len(journal(server)) != 0 {
		t.Fatalf("a quiet main: roots=%q result=%+v", roots, result)
	}
	_ = quiet.Process.Kill()
	_ = quiet.Wait()
	roots, result = deliverDirs(t, server, captured, inbox.Message{ID: "n2", Kind: inbox.Note})
	if !slices.Equal(roots, []string{workspace}) || !strings.Contains(result.Detail, "has ended") {
		t.Fatalf("a main that ended: roots=%q result=%+v", roots, result)
	}
}
