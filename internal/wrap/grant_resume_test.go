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

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/grantauth"
	"github.com/praline-labs/rewake/internal/grantauth/grantauthtest"
	"github.com/praline-labs/rewake/internal/harness/claude"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/state"
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
	grants := map[string][]string{"m1": {lib}, "m2": {doc}}
	// The run before confirmed them at delivery, naming its conversation,
	// and ended.
	self := grantauth.Expect{PID: os.Getpid()}
	self.Start, _ = proc.StartTime(os.Getpid())
	previous := grantauthtest.EndedRun(t, func(run string) {
		for id, directories := range grants {
			if err := grantauth.Register(address, grantauth.Grant{ID: id, To: "worker", ToEpoch: run, Dirs: directories}); err != nil {
				t.Fatal(err)
			}
		}
	}, grantauthtest.Delivery{Dir: dir, Address: address, MainPID: self.PID, MainStart: self.Start, From: "lead", FromEpoch: main, To: "worker", Grants: grants, Threads: []string{"s1"}})
	closed.Store("m2")
	copied := []grant.Entry{
		{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: "s1", From: "lead", FromEpoch: main},
		{Path: doc, Message: "m2", Outcome: grant.Granted, Thread: "s1", From: "lead", FromEpoch: main},
		{Path: forged, Message: "m3", Outcome: grant.Granted, Thread: "s1", From: "lead", FromEpoch: main},
		{Path: forged, Message: "m4", Outcome: grant.Granted, Thread: "s1", From: "lead", FromEpoch: endedEpoch(t)},
	}
	if err := grant.Save(dir, "worker", previous, copied); err != nil {
		t.Fatal(err)
	}

	// A copy a worker wrote for a conversation of its own, naming the grant
	// delivered into c1, and a resume of that conversation while main still
	// holds the grant: the grant does not follow it there.
	own := []grant.Entry{{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: "c0", From: "lead", FromEpoch: main}}
	if err := grant.Save(dir, "worker", endedEpoch(t), own); err != nil {
		t.Fatal(err)
	}
	if elsewhere := resumeGrants(dir, "worker", main, claude.New(), []string{"--resume", "c0"}); len(elsewhere.dirs()) != 0 {
		t.Fatalf("a resume of another conversation got %v", elsewhere.dirs())
	}

	restored := resumeGrants(dir, "worker", main, claude.New(), []string{"--resume", "s1"})
	if restored.thread != "s1" || !slices.Equal(restored.dirs(), []string{lib}) {
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
	if len(entries) != 1 || entries[0].Path != lib || entries[0].Thread != "s1" || entries[0].FromEpoch != main || !entries[0].Live() {
		t.Fatalf("kept %+v", entries)
	}

	// A launch that resumes nothing, or a fork, restores nothing.
	for _, args := range [][]string{nil, {"--resume", "s1", "--fork-session"}} {
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
