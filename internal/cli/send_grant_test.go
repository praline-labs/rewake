package cli

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/grantauth"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// grantingMain serves the grant authority of the caller's wrapper, which the
// caller's record names: this test process. Its sends register there, from
// below it; a wrapper elsewhere takes no registration from them.
func grantingMain(t *testing.T, dir string, self registry.Session, wrapper int) {
	t.Helper()
	authority, err := grantauth.Listen(state.AuthorityAddress(dir, self.Epoch()), wrapper, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go authority.Serve(ctx)
	t.Cleanup(func() { cancel(); authority.Close() })
}

// A send that registers its grant leaves it confirmable by the recipient's
// wrapper, as the message carries it.
func TestADirGrantIsRegisteredWithMainsWrapper(t *testing.T) {
	w := newGrantWorld(t, "main", "")
	lib := filepath.Join(w.home, "work", "lib")
	if code, out, stderr := run("send", w.peer.Name, "Write in lib", "--wait=0", "--grant-dir", lib); code != ExitPending {
		t.Fatalf("send: %d %s %s", code, out, stderr)
	}
	messages := w.sent(t)
	if len(messages) != 1 {
		t.Fatalf("sent %d", len(messages))
	}
	message := messages[0]
	self, err := registry.Load(w.dir, message.From)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := grantauth.Confirm(state.AuthorityAddress(w.dir, self.Epoch()), grantauth.Expect{PID: self.ServicePID, Start: self.ServiceStart}, message.ID, w.peer.Name, w.peer.Epoch(), "c1")
	if err != nil || !confirmed.Same(grantauth.Grant{ID: message.ID, To: w.peer.Name, ToEpoch: w.peer.Epoch(), Dirs: []string{lib}}) {
		t.Fatalf("confirmed %+v, %v", confirmed, err)
	}
}

// A process that does not run below main's wrapper — a worker's, with main's
// variables in its environment — registers nothing, and so sends nothing.
func TestAGrantSentFromOutsideMainsWrapperIsRefused(t *testing.T) {
	dir, self, peer := stateCaller(t, "main")
	stranger := exec.Command("sleep", "30")
	if err := stranger.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stranger.Process.Kill(); _ = stranger.Wait() })
	grantingMain(t, dir, self, stranger.Process.Pid)
	peer.Role = "write"
	stubGrants(t, peer.Harness, true, true)
	if err := registry.Update(dir, peer); err != nil {
		t.Fatal(err)
	}
	code, out, stderr := run("send", peer.Name, "Commit it", "--wait=0", "--grant-git")
	if code != ExitFailed || !strings.Contains(stderr, "only a command this session runs") {
		t.Fatalf("got %d %s %s", code, out, stderr)
	}
	if files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, peer.Name), "*.json")); len(files) != 0 {
		t.Fatalf("an unregistered grant was sent: %v", files)
	}
}

// A main whose commands cannot reach its wrapper cannot grant: its grant
// would be refused on delivery, so it is refused before it is sent.
func TestAMainThatCannotReachItsWrapperCannotGrant(t *testing.T) {
	w := newGrantWorld(t, "main", "")
	self, err := registry.Load(w.dir, os.Getenv(state.SessionEnv))
	if err != nil {
		t.Fatal(err)
	}
	stubGrants(t, self.Harness, true, false)
	for _, grant := range [][]string{{"--grant-dir", filepath.Join(w.home, "work", "lib")}, {"--grant-git"}} {
		args := append([]string{"send", w.peer.Name, "Write there", "--wait=0"}, grant...)
		code, out, stderr := run(args...)
		if code != ExitFailed || !strings.Contains(stderr, "cannot reach its wrapper") {
			t.Fatalf("%v: got %d %s %s", grant, code, out, stderr)
		}
	}
	if messages := w.sent(t); len(messages) != 0 {
		t.Fatalf("a main that cannot reach its wrapper sent %d grants", len(messages))
	}
}

// forgeLetter rewrites the unread letter of a message in its recipient's
// mailbox, as a sandboxed worker can: the state directory is writable to it.
func (w grantWorld) forgeLetter(t *testing.T, id string, change func(*inbox.Message)) {
	t.Helper()
	path := filepath.Join(state.InboxPath(w.dir, w.peer.Name), id+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var message inbox.Message
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	change(&message)
	if raw, err = json.Marshal(message); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// replacementOf finds the letter an edit wrote in place of id.
func (w grantWorld) replacementOf(t *testing.T, id string) (inbox.Message, bool) {
	t.Helper()
	for _, message := range w.sent(t) {
		if message.Replaces == id {
			return message, true
		}
	}
	return inbox.Message{}, false
}

// An edit carries the grant main's wrapper holds for the task it replaces,
// not the one its letter names: a worker that widened the grant in its
// unread letter gets the directory main gave, and no more.
func TestAnEditCarriesTheGrantMainsWrapperHolds(t *testing.T) {
	w := newGrantWorld(t, "main", "")
	lib, wide := filepath.Join(w.home, "work", "lib"), filepath.Join(w.home, "wide")
	if code, out, stderr := run("send", w.peer.Name, "Write in lib", "--wait=0", "--grant-dir", lib); code != ExitPending {
		t.Fatalf("send: %d %s %s", code, out, stderr)
	}
	original := w.sent(t)[0]
	w.forgeLetter(t, original.ID, func(message *inbox.Message) {
		message.GrantDirs, message.GrantBroad, message.GrantGit = []string{lib, wide}, []string{wide}, true
	})
	if code, out, stderr := run("edit", original.ID, "Write in lib, and test it", "--wait=0"); code != ExitPending {
		t.Fatalf("edit: %d %s %s", code, out, stderr)
	}
	replacement, ok := w.replacementOf(t, original.ID)
	if !ok {
		t.Fatal("the edit wrote no replacement")
	}
	if !slices.Equal(replacement.GrantDirs, []string{lib}) || len(replacement.GrantBroad) != 0 || replacement.GrantGit {
		t.Fatalf("the replacement carries %v broad %v git %v, want only %s", replacement.GrantDirs, replacement.GrantBroad, replacement.GrantGit, lib)
	}
	self, err := registry.Load(w.dir, original.From)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := grantauth.Confirm(state.AuthorityAddress(w.dir, self.Epoch()), grantauth.Expect{PID: self.ServicePID, Start: self.ServiceStart}, replacement.ID, w.peer.Name, w.peer.Epoch(), "c1")
	if err != nil || !confirmed.Same(grantauth.Grant{ID: replacement.ID, To: w.peer.Name, ToEpoch: w.peer.Epoch(), Dirs: []string{lib}}) {
		t.Fatalf("confirmed %+v, %v", confirmed, err)
	}
}

// A letter main sent with no grant, given one by the worker on disk, is not
// edited into a grant main never gave: main's wrapper holds none for it, and
// the edit is refused with the reason.
func TestAnEditOfALetterGivenAForgedGrantIsRefused(t *testing.T) {
	w := newGrantWorld(t, "main", "")
	wide := filepath.Join(w.home, "wide")
	if code, out, stderr := run("send", w.peer.Name, "Look at the logs", "--wait=0"); code != ExitPending && code != ExitOK {
		t.Fatalf("send: %d %s %s", code, out, stderr)
	}
	original := w.sent(t)[0]
	w.forgeLetter(t, original.ID, func(message *inbox.Message) {
		message.GrantDirs, message.GrantBroad, message.GrantGit = []string{wide}, []string{wide}, true
	})
	code, out, stderr := run("edit", original.ID, "Look at the logs of yesterday", "--wait=0")
	if code != ExitFailed || !strings.Contains(stderr, "does not hold") {
		t.Fatalf("edit: %d %s %s", code, out, stderr)
	}
	if replacement, ok := w.replacementOf(t, original.ID); ok {
		t.Fatalf("the edit wrote %+v", replacement)
	}
}
