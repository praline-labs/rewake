package host

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

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/grant/authority"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
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
	message := mail.Message{ID: "m1", From: "lead", FromEpoch: epoch, To: "worker", ToEpoch: "9.9", Kind: mail.Task, GrantDirs: []string{"/src/lib"}}

	// Nobody listens yet, and main is alive: the message waits.
	if err := confirmGrant(dir, "worker", "9.9", "c1", message); !errors.Is(err, mail.ErrNotYet) {
		t.Fatalf("before main listens: %v", err)
	}

	server, err := authority.Listen(state.AuthorityAddress(dir, epoch), os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	defer server.Close()
	if err := confirmGrant(dir, "worker", "9.9", "c1", message); err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("a grant main never registered: %v", err)
	}
	registered := authority.Grant{ID: "m1", To: "worker", ToEpoch: "9.9", Dirs: []string{"/src/lib"}}
	if err := authority.Register(state.AuthorityAddress(dir, epoch), registered); err != nil {
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
	if err := confirmGrant(dir, "worker", "9.9", "c1", ended); err == nil || errors.Is(err, mail.ErrNotYet) || !strings.Contains(err.Error(), "has ended") {
		t.Fatalf("a main that has ended: %v", err)
	}
	forged := message
	forged.FromEpoch = "not-a-run"
	if err := confirmGrant(dir, "worker", "9.9", "c1", forged); err == nil {
		t.Fatal("a sender with no run was asked")
	}
}

// Main holds the conversation a grant's letter was pinned to, not the one its
// recipient had when it first checked the grant: only that one may take the
// grant again after a resume. This process stands for both wrappers.
func TestMainHoldsTheConversationTheLetterWasPinnedTo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rooms", "default")
	run := ownEpoch(t)
	lib, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	address := state.AuthorityAddress(dir, run)
	server, err := authority.Listen(address, os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	defer server.Close()
	if err := authority.Register(address, authority.Grant{ID: "m1", To: "worker", ToEpoch: run, Dirs: []string{lib}}); err != nil {
		t.Fatal(err)
	}
	message := mail.Message{ID: "m1", From: "lead", FromEpoch: run, To: "worker", ToEpoch: run, Kind: mail.Task, GrantDirs: []string{lib}}
	check := checkGrant(dir, "worker", run, adapter.Protected{})
	if err := check(message); err != nil {
		t.Fatal(err)
	}
	pinGrant(dir, "worker", run)(message, "after-clear")
	expect := authority.Expect{PID: os.Getpid()}
	if expect.Start, err = proc.StartTime(os.Getpid()); err != nil {
		t.Fatal(err)
	}
	if _, err := authority.Reconfirm(address, expect, "m1", "worker", run, "before-clear"); err == nil {
		t.Fatal("the grant would be taken into the conversation it was checked in")
	}
	if _, err := authority.Reconfirm(address, expect, "m1", "worker", run, "after-clear"); err != nil {
		t.Fatalf("not taken into the conversation it was pinned to: %v", err)
	}
}

// Main's wrapper asks the mailbox whether a grant's task is open: one waiting
// for its reader is, one done is not, and its grant goes rather than being
// confirmed again after the report.
func TestMainsAuthorityAsksTheMailboxWhetherATaskIsOpen(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rooms", "default")
	server := listenAuthority(dir, ownEpoch(t), os.Getpid())
	if server == nil {
		t.Fatal("main's wrapper did not bind its address")
	}
	defer server.Close()
	task := mail.Message{ID: "m1", From: "lead", To: "worker", Kind: mail.Task, Text: "go"}
	if err := mail.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	held := authority.Grant{ID: task.ID, To: "worker", ToEpoch: endedEpoch(t)}
	if open, found := server.Open(held); !open || !found {
		t.Fatalf("a waiting task: open %v, found %v", open, found)
	}
	if err := state.EnsureSubdir(state.DonePath(dir, "worker")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(state.InboxPath(dir, "worker"), task.ID+".json"), filepath.Join(state.DonePath(dir, "worker"), task.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if open, found := server.Open(held); open || !found {
		t.Fatalf("a done task: open %v, found %v", open, found)
	}
}
