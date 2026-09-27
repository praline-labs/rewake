package wrap

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/state"
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
	if err := confirmGrant(dir, "worker", "9.9", message); !errors.Is(err, inbox.ErrNotYet) {
		t.Fatalf("before main listens: %v", err)
	}

	authority, err := grantauth.Listen(state.AuthorityAddress(dir, "lead", epoch), os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go authority.Serve(ctx)
	defer authority.Close()
	if err := confirmGrant(dir, "worker", "9.9", message); err == nil || !strings.Contains(err.Error(), "did not confirm") {
		t.Fatalf("a grant main never registered: %v", err)
	}
	registered := grantauth.Grant{ID: "m1", To: "worker", ToEpoch: "9.9", Dirs: []string{"/src/lib"}}
	if err := grantauth.Register(state.AuthorityAddress(dir, "lead", epoch), registered); err != nil {
		t.Fatal(err)
	}
	if err := confirmGrant(dir, "worker", "9.9", message); err != nil {
		t.Fatalf("a registered grant: %v", err)
	}
	wider := message
	wider.GrantDirs = []string{"/src/lib", "/src/other"}
	if err := confirmGrant(dir, "worker", "9.9", wider); err == nil || !strings.Contains(err.Error(), "another grant") {
		t.Fatalf("a message carrying more than main registered: %v", err)
	}
	if err := confirmGrant(dir, "worker", "8.8", message); err == nil {
		t.Fatal("confirmed for another run of the reader")
	}

	ended := message
	ended.FromEpoch = endedEpoch(t)
	if err := confirmGrant(dir, "worker", "9.9", ended); err == nil || errors.Is(err, inbox.ErrNotYet) || !strings.Contains(err.Error(), "has ended") {
		t.Fatalf("a main that has ended: %v", err)
	}
	forged := message
	forged.FromEpoch = "not-a-run"
	if err := confirmGrant(dir, "worker", "9.9", forged); err == nil {
		t.Fatal("a sender with no run was asked")
	}
}
