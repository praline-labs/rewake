package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/grantauth"
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
	w := newGrantWorld(t, "main", "codex")
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
	peer.Role, peer.Harness = "write", "codex"
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
func TestACodexMainCannotGrant(t *testing.T) {
	w := newGrantWorld(t, "main", "codex")
	self, err := registry.Load(w.dir, os.Getenv(state.SessionEnv))
	if err != nil {
		t.Fatal(err)
	}
	self.Harness = "codex"
	if err := registry.Update(w.dir, self); err != nil {
		t.Fatal(err)
	}
	for _, grant := range [][]string{{"--grant-dir", filepath.Join(w.home, "work", "lib")}, {"--grant-git"}} {
		args := append([]string{"send", w.peer.Name, "Write there", "--wait=0"}, grant...)
		code, out, stderr := run(args...)
		if code != ExitFailed || !strings.Contains(stderr, "cannot reach its wrapper") {
			t.Fatalf("%v: got %d %s %s", grant, code, out, stderr)
		}
	}
	if messages := w.sent(t); len(messages) != 0 {
		t.Fatalf("a Codex main sent %d grants", len(messages))
	}
}
