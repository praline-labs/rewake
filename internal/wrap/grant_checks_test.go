package wrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/grantauth"
	"github.com/praline-labs/rewake/internal/grantauth/grantauthtest"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/state"
)

// A directory swapped for a link after the grant was sent fails at delivery,
// before main is asked: the check with this session's view comes first, and
// a main that has not answered yet would otherwise only hold the task back.
func TestAGrantWhoseDirectoryBecameALinkIsRefusedBeforeMainIsAsked(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rooms", "default")
	target, root := realDir(t), realDir(t)
	swapped := filepath.Join(root, "lib")
	if err := os.Symlink(target, swapped); err != nil {
		t.Fatal(err)
	}
	// Nobody listens for main, and main runs: asked, it would say "not yet".
	message := inbox.Message{ID: "m1", From: "lead", FromEpoch: ownEpoch(t), To: "worker", ToEpoch: "9.9", Kind: inbox.Task, GrantDirs: []string{swapped}}
	err := checkGrant(dir, "worker", "9.9", nil, nil, nil)(message)
	if err == nil || errors.Is(err, inbox.ErrNotYet) || !strings.Contains(err.Error(), "now leads to") {
		t.Fatalf("a swapped directory: %v", err)
	}
}

// A sender name that is not a session's is not asked about at all: the
// address main's wrapper is found at is built from the letter, which a worker
// can write.
func TestAGrantFromAnInvalidSenderNameIsRefused(t *testing.T) {
	dir := t.TempDir()
	for _, from := range []string{"", "../lead", "lead/x"} {
		message := inbox.Message{ID: "m1", From: from, FromEpoch: ownEpoch(t), To: "worker", ToEpoch: "9.9", Kind: inbox.Task, GrantDirs: []string{"/src/lib"}}
		err := confirmGrant(dir, "worker", "9.9", "c1", message)
		if err == nil || errors.Is(err, inbox.ErrNotYet) || !strings.Contains(err.Error(), "not a session run") {
			t.Errorf("from %q: %v", from, err)
		}
	}
}

// A grant restored before the launch was given to the harness at launch, so
// taking it back once its task settles needs the hook, as a directory the
// hook added does; one restored after the launch was never written in and
// ends at once.
func TestAGrantRestoredAtLaunchIsTakenBackThroughTheHook(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rooms", "default")
	lib := realDir(t)
	restored := []grantauth.Restored{{Hint: grant.Hint{Message: "m1", From: "lead", FromEpoch: "1.1"}, Grant: grantauth.Grant{ID: "m1", To: "worker", Dirs: []string{lib}}}}
	own := grantauth.Expect{PID: os.Getpid()}
	own.Start, _ = proc.StartTime(os.Getpid())
	for added, want := range map[bool]string{true: grant.Revoking, false: grant.Revoked} {
		address := state.KeeperAddress(dir, "worker", ownEpoch(t)) + map[bool]string{true: "/added", false: "/later"}[added]
		keeper, err := grantauth.Keep(address, os.Getpid())
		if err != nil {
			t.Fatal(err)
		}
		keeper.Settled = func(string) bool { return true }
		ctx, cancel := context.WithCancel(context.Background())
		go keeper.Serve(ctx)
		if again := keepResumed(keeper, "s1", restored, added); again {
			t.Fatalf("added %v: asked again", added)
		}
		if _, err := grantauth.Ask(address, own, json.RawMessage(`{}`)); err != nil {
			t.Fatal(err)
		}
		if entries := keeper.Entries(); len(entries) != 1 || entries[0].Outcome != want || entries[0].Thread != "s1" {
			t.Errorf("added %v: kept %+v, want %s", added, entries, want)
		}
		cancel()
		keeper.Close()
	}
}

// resumingHarness takes grants through a hook and names the conversation its
// launch arguments resume: what a launch that restores grants needs.
type resumingHarness struct {
	contextHarness
}

func (h *resumingHarness) DecideGrant(json.RawMessage, []grant.Entry) grantauth.Decision {
	return grantauth.Decision{}
}

func (h *resumingHarness) ResumedConversation(args []string) string {
	if len(args) == 2 && args[0] == "--resume" {
		return args[1]
	}
	return ""
}

// A launch that resumes a conversation starts the harness with the
// directories main confirmed again for it, and no others.
func TestAResumedLaunchStartsTheHarnessWithTheGrantsConfirmedAgain(t *testing.T) {
	dir := stateDir(t)
	main := ownEpoch(t)
	// A session is named with its harness.
	name := "worker-fake"
	lib := realDir(t)
	address := state.AuthorityAddress(dir, main)
	authority, err := grantauth.Listen(address, os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	authority.Open = func(grantauth.Grant) (bool, bool) { return true, true }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go authority.Serve(ctx)
	defer authority.Close()
	self := grantauth.Expect{PID: os.Getpid()}
	self.Start, _ = proc.StartTime(os.Getpid())
	grants := map[string][]string{"m1": {lib}}
	previous := grantauthtest.EndedRun(t, func(run string) {
		if err := grantauth.Register(address, grantauth.Grant{ID: "m1", To: name, ToEpoch: run, Dirs: []string{lib}}); err != nil {
			t.Fatal(err)
		}
	}, grantauthtest.Delivery{Dir: dir, Address: address, MainPID: self.PID, MainStart: self.Start, From: "lead", FromEpoch: main, To: name, Grants: grants, Threads: []string{"s1"}})
	if err := grant.Save(dir, name, previous, []grant.Entry{{Path: lib, Message: "m1", Outcome: grant.Granted, Thread: "s1", From: "lead", FromEpoch: main}}); err != nil {
		t.Fatal(err)
	}
	for args, want := range map[string][]string{"--resume s1": {lib}, "--resume s2": nil} {
		fake := &resumingHarness{contextHarness: contextHarness{fakeHarness: fakeHarness{script: "exit 0"}}}
		if _, err := Run(context.Background(), Request{Dir: dir, Name: "worker", Harness: fake, Args: strings.Fields(args)}); err != nil {
			t.Fatal(err)
		}
		if fake.launch.Name != name {
			t.Fatalf("launched as %s", fake.launch.Name)
		}
		if !slices.Equal(fake.launch.GrantDirs, want) {
			t.Errorf("%s: started with %v, want %v", args, fake.launch.GrantDirs, want)
		}
	}
}
