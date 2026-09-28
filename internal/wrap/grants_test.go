package wrap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/grantauth"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/state"
)

// ownEpoch is this process's run, standing for main's wrapper.
func ownEpoch(t *testing.T) string {
	t.Helper()
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	return strconv.Itoa(os.Getpid()) + "." + strconv.FormatUint(start, 10)
}

// endedEpoch is the run of a process that has exited.
func endedEpoch(t *testing.T) string {
	t.Helper()
	child := exec.Command("true")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	pid := child.Process.Pid
	start, err := proc.StartTime(pid)
	_ = child.Wait()
	if err != nil {
		// Gone before its start was read: any start time names a run that
		// is not there.
		start = 1
	}
	return strconv.Itoa(pid) + "." + strconv.FormatUint(start, 10)
}

// A grant reaches its reader only when the wrapper of the main that sent it
// confirms the same grant; one it cannot ask yet waits, and one whose main
// has ended, or that main never registered, fails.
func TestAGrantIsConfirmedWithTheMainThatSentIt(t *testing.T) {
	dir := t.TempDir()
	epoch := ownEpoch(t)
	message := inbox.Message{ID: "m1", From: "lead", FromEpoch: epoch, To: "worker", ToEpoch: "9.9", Kind: inbox.Task, GrantDirs: []string{"/src/lib"}}

	// Nobody listens yet, and main is alive: the message waits.
	if err := confirmGrant(dir, "worker", "9.9", "c1", message); !errors.Is(err, inbox.ErrNotYet) {
		t.Fatalf("before main listens: %v", err)
	}

	authority, err := grantauth.Listen(state.AuthorityAddress(dir, epoch), os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go authority.Serve(ctx)
	defer authority.Close()
	if err := confirmGrant(dir, "worker", "9.9", "c1", message); err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("a grant main never registered: %v", err)
	}
	registered := grantauth.Grant{ID: "m1", To: "worker", ToEpoch: "9.9", Dirs: []string{"/src/lib"}}
	if err := grantauth.Register(state.AuthorityAddress(dir, epoch), registered); err != nil {
		t.Fatal(err)
	}
	if err := confirmGrant(dir, "worker", "9.9", "c1", message); err != nil {
		t.Fatalf("a registered grant: %v", err)
	}
	wider := message
	wider.GrantDirs = []string{"/src/lib", "/src/other"}
	if err := confirmGrant(dir, "worker", "9.9", "c1", wider); err == nil || !strings.Contains(err.Error(), "another grant") {
		t.Fatalf("a message carrying more than main registered: %v", err)
	}
	if err := confirmGrant(dir, "worker", "8.8", "c1", message); err == nil {
		t.Fatal("confirmed for another run of the reader")
	}

	ended := message
	ended.FromEpoch = endedEpoch(t)
	if err := confirmGrant(dir, "worker", "9.9", "c1", ended); err == nil || errors.Is(err, inbox.ErrNotYet) || !strings.Contains(err.Error(), "has ended") {
		t.Fatalf("a main that has ended: %v", err)
	}
	forged := message
	forged.FromEpoch = "not-a-run"
	if err := confirmGrant(dir, "worker", "9.9", "c1", forged); err == nil {
		t.Fatal("a sender with no run was asked")
	}
}

// For a harness that takes a grant through its own hook, a confirmed grant
// waits while the session works, then goes into the keeper its hook asks.
func TestAGrantForAHookWaitsForIdleAndIsKept(t *testing.T) {
	// A room below a state directory of its own: the grant lies beside it.
	dir := filepath.Join(t.TempDir(), "rooms", "default")
	epoch := ownEpoch(t)
	lib, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authority, err := grantauth.Listen(state.AuthorityAddress(dir, epoch), os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go authority.Serve(ctx)
	defer authority.Close()
	if err := grantauth.Register(state.AuthorityAddress(dir, epoch), grantauth.Grant{ID: "m1", To: "worker", ToEpoch: "9.9", Dirs: []string{lib}}); err != nil {
		t.Fatal(err)
	}
	keeper, err := grantauth.Keep(state.KeeperAddress(dir, "worker", "9.9"), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	defer keeper.Close()
	message := inbox.Message{ID: "m1", From: "lead", FromEpoch: epoch, To: "worker", ToEpoch: "9.9", Kind: inbox.Task, GrantDirs: []string{lib}}

	busy := true
	conversation := func() (string, error) { return "s9", nil }
	check := checkGrant(dir, "worker", "9.9", keeper, func() bool { return busy }, conversation)
	if err := check(message); !errors.Is(err, inbox.ErrNotYet) || len(keeper.Entries()) != 0 {
		t.Fatalf("while the session works: %v, kept %v", err, keeper.Entries())
	}
	busy = false
	if err := check(message); err != nil {
		t.Fatal(err)
	}
	// Kept with the conversation it went into, for a resume to ask by.
	if entries := keeper.Entries(); len(entries) != 1 || entries[0].Path != lib || !entries[0].Live() || entries[0].Thread != "s9" {
		t.Fatalf("kept %+v", entries)
	}
	// A grant main never registered is not kept.
	unregistered := message
	unregistered.ID = "m2"
	if err := check(unregistered); err == nil || len(keeper.Entries()) != 1 {
		t.Fatalf("an unconfirmed grant: %v, kept %v", err, keeper.Entries())
	}
}
