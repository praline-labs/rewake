package wrap

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/grant"
	"github.com/iiiokojiadbi/rewake/internal/grantauth"
	"github.com/iiiokojiadbi/rewake/internal/harness/claude"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// realDir is a directory as the rules see it, with no link on its path.
func realDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// A run that resumes a conversation keeps what the main that sent each grant
// confirms again, and nothing a copy only claims: a grant main never made, one
// whose task is closed, one whose main has ended.
func TestAResumedRunKeepsOnlyWhatMainConfirmsAgain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rooms", "default")
	main := ownEpoch(t)
	previous := endedEpoch(t)
	lib, doc, forged := realDir(t), realDir(t), realDir(t)

	authority, err := grantauth.Listen(state.AuthorityAddress(dir, main), os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var closed atomic.Value
	closed.Store("")
	authority.Open = func(held grantauth.Grant) (bool, bool) { return held.ID != closed.Load(), true }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go authority.Serve(ctx)
	defer authority.Close()
	address := state.AuthorityAddress(dir, main)
	pid, start, _ := registry.ParseEpoch(main)
	expect := grantauth.Expect{PID: pid, Start: start}
	for id, directory := range map[string]string{"m1": lib, "m2": doc} {
		if err := grantauth.Register(address, grantauth.Grant{ID: id, To: "worker", ToEpoch: previous, Dirs: []string{directory}}); err != nil {
			t.Fatal(err)
		}
		if _, err := grantauth.Confirm(address, expect, id, "worker", previous); err != nil {
			t.Fatal(err)
		}
	}
	closed.Store("m2")
	copied := []grant.Entry{
		{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: "c1", From: "lead", FromEpoch: main},
		{Path: doc, Message: "m2", Outcome: grant.Granted, Thread: "c1", From: "lead", FromEpoch: main},
		{Path: forged, Message: "m3", Outcome: grant.Granted, Thread: "c1", From: "lead", FromEpoch: main},
		{Path: forged, Message: "m4", Outcome: grant.Granted, Thread: "c1", From: "lead", FromEpoch: endedEpoch(t)},
	}
	if err := grant.Save(dir, "worker", previous, copied); err != nil {
		t.Fatal(err)
	}

	restored := resumeGrants(dir, "worker", main, claude.New(), []string{"--resume", "c1"})
	if restored.thread != "c1" || !slices.Equal(restored.dirs(), []string{lib}) {
		t.Fatalf("restored %v for %q", restored.dirs(), restored.thread)
	}
	notes := strings.Join(restored.notes, "\n")
	for _, want := range []string{"restored after the resume", "m2 is not restored", "m3 is not restored", "m4 is not restored", "has ended"} {
		if !strings.Contains(notes, want) {
			t.Errorf("the notes do not say %q:\n%s", want, notes)
		}
	}

	keeper, err := grantauth.Keep(state.KeeperAddress(dir, "worker", main), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	defer keeper.Close()
	if again := keepResumed(keeper, restored.thread, restored.grants, true); again {
		t.Error("nothing is left to ask, and the conversation is asked again")
	}
	entries := keeper.Entries()
	if len(entries) != 1 || entries[0].Path != lib || entries[0].Thread != "c1" || entries[0].FromEpoch != main || !entries[0].Live() {
		t.Fatalf("kept %+v", entries)
	}

	// A launch that resumes nothing, or a fork, restores nothing.
	for _, args := range [][]string{nil, {"--resume", "c1", "--fork-session"}} {
		if got := resumeGrants(dir, "worker", main, claude.New(), args); len(got.grants) != 0 {
			t.Errorf("%v restored %+v", args, got.grants)
		}
	}
}

// A main that runs and does not answer is asked again later: the conversation
// is not done with while its grant may still be confirmed.
func TestAQuietMainIsAskedAgain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rooms", "default")
	keeper, err := grantauth.Keep(state.KeeperAddress(dir, "worker", ownEpoch(t)), os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	defer keeper.Close()
	quiet := []grantauth.Restored{{Hint: grant.Hint{Message: "m1"}, Err: grantauth.ErrUnreachable}}
	if !keepResumed(keeper, "c1", quiet, true) || len(keeper.Entries()) != 0 {
		t.Fatalf("a grant not confirmed yet: kept %+v", keeper.Entries())
	}
}
