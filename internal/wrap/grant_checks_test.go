package wrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/grant"
	"github.com/praline-labs/rewake/internal/core/grant/authority"
	"github.com/praline-labs/rewake/internal/core/grant/authority/authoritytest"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
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
	message := mail.Message{ID: "m1", From: "lead", FromEpoch: ownEpoch(t), To: "worker", ToEpoch: "9.9", Kind: mail.Task, GrantDirs: []string{swapped}}
	err := checkGrant(dir, "worker", "9.9", adapter.Protected{})(message)
	if err == nil || errors.Is(err, mail.ErrNotYet) || !strings.Contains(err.Error(), "now leads to") {
		t.Fatalf("a swapped directory: %v", err)
	}
}

// A sender name that is not a session's is not asked about at all: the
// address main's wrapper is found at is built from the letter, which a worker
// can write.
func TestAGrantFromAnInvalidSenderNameIsRefused(t *testing.T) {
	dir := t.TempDir()
	for _, from := range []string{"", "../lead", "lead/x"} {
		message := mail.Message{ID: "m1", From: from, FromEpoch: ownEpoch(t), To: "worker", ToEpoch: "9.9", Kind: mail.Task, GrantDirs: []string{"/src/lib"}}
		err := confirmGrant(dir, "worker", "9.9", "c1", message)
		if err == nil || errors.Is(err, mail.ErrNotYet) || !strings.Contains(err.Error(), "not a session run") {
			t.Errorf("from %q: %v", from, err)
		}
	}
}

// resumingHarness takes directory grants and names the conversation its
// launch arguments resume: what a launch that restores grants needs. No
// product adapter is both from S9 until stage 6 (grant_resume.go).
type resumingHarness struct {
	contextHarness
}

func (h *resumingHarness) Offers() adapter.Offer {
	return adapter.Offer{Wake: true, Permissions: adapter.PermissionsOffer{Dir: true}}
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
	server, err := authority.Listen(address, os.Getpid(), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	server.Open = func(authority.Grant) (bool, bool) { return true, true }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go server.Serve(ctx)
	defer server.Close()
	self := authority.Expect{PID: os.Getpid()}
	self.Start, _ = proc.StartTime(os.Getpid())
	grants := map[string][]string{"m1": {lib}}
	previous := authoritytest.EndedRun(t, func(run string) {
		if err := authority.Register(address, authority.Grant{ID: "m1", To: name, ToEpoch: run, Dirs: []string{lib}}); err != nil {
			t.Fatal(err)
		}
	}, authoritytest.Delivery{Dir: dir, Address: address, MainPID: self.PID, MainStart: self.Start, From: "lead", FromEpoch: main, To: name, Grants: grants, Threads: []string{"s1"}})
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

// Step 3 of the grant scheme, through the whole check a delivery makes: while
// the main that sent a granted task runs and does not answer, the task waits
// to be asked again; once that main has ended, nobody can confirm the grant,
// and the task fails naming that main, to be sent again from the current one
// (docs/grants-authority.md#the-scheme).
func TestAGrantWhoseMainHasEndedFailsNamingThatMain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rooms", "default")
	lib := realDir(t)
	message := mail.Message{ID: "m1", From: "lead", FromEpoch: ownEpoch(t), To: "worker", ToEpoch: "9.9", Kind: mail.Task, GrantDirs: []string{lib}}
	check := checkGrant(dir, "worker", "9.9", adapter.Protected{})

	if err := check(message); !errors.Is(err, mail.ErrNotYet) || !strings.Contains(err.Error(), "lead") {
		t.Fatalf("a quiet main that runs: %v, want the task to wait for it", err)
	}
	message.FromEpoch = endedEpoch(t)
	err := check(message)
	if err == nil || errors.Is(err, mail.ErrNotYet) {
		t.Fatalf("a main that has ended: %v, want the task to fail", err)
	}
	for _, said := range []string{"lead, which sent it, has ended", "send the task again from the current main"} {
		if !strings.Contains(err.Error(), said) {
			t.Errorf("the failure %q does not say %q", err, said)
		}
	}
}
