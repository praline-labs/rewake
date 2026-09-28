package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// A session writing to itself is refused before anything is written: a task
// would owe a report to the very turn that sent it, and a question would wait
// for an answer only its own blocked turn could give.
func TestSendToItselfIsRefused(t *testing.T) {
	for _, kind := range []string{"", "--question", "--notify"} {
		t.Run("kind "+kind, func(t *testing.T) {
			dir, self, _ := stateCaller(t, "general")
			args := []string{"send", self.Name, "do it", "--wait=0"}
			if kind != "" {
				args = append(args, kind)
			}
			code, _, stderr := run(args...)
			if code != ExitUsage || !strings.Contains(stderr, "cannot send to itself") {
				t.Fatalf("got %d %s", code, stderr)
			}
			if files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, self.Name), "*.json")); len(files) != 0 {
				t.Fatalf("a refused send wrote %v", files)
			}
		})
	}
}

// A flag that is wrong whoever the message is for is named as such, not
// hidden behind a session that happens to be missing.
func TestSendChecksFlagsBeforeTheSession(t *testing.T) {
	liveSession(t, "caller")
	t.Setenv(state.SessionEnv, "")
	for flags, says := range map[string]string{
		"--wait abc":            "--wait takes a number of seconds",
		"--question --notify":   "exclude each other",
		"--notify --question":   "exclude each other",
		"--wait=-1 --notify":    "--wait takes a number of seconds",
		"--wait 1 --notify --q": "does not take --q",
	} {
		code, _, stderr := run(append([]string{"send", "nobody", "text"}, strings.Fields(flags)...)...)
		if code != ExitUsage || !strings.Contains(stderr, says) {
			t.Errorf("%s: got %d %s, want %q", flags, code, stderr, says)
		}
	}
}

// A task or a heads-up waits for its delivery as long as --wait says: it once
// stopped at five seconds and said it had waited the whole time asked.
func TestSendWaitsForTheDeliveryAsLongAsAsked(t *testing.T) {
	for _, c := range []struct {
		kind string
		want time.Duration
	}{{"", 12 * time.Second}, {"--notify", 12 * time.Second}} {
		t.Run("kind "+c.kind, func(t *testing.T) {
			_, _, peer := stateCaller(t, "main")
			var waited time.Duration
			previous := awaitStatus
			awaitStatus = func(_, _, _ string, wait time.Duration, _ func() bool) (inbox.Status, bool) {
				waited = wait
				return inbox.Status{State: inbox.Delivered}, true
			}
			t.Cleanup(func() { awaitStatus = previous })
			args := []string{"send", peer.Name, "do it", "--wait", "12"}
			if c.kind != "" {
				args = append(args, c.kind)
			}
			_, _, _ = run(args...)
			if waited != c.want {
				t.Fatalf("waited %s for the delivery, want %s", waited, c.want)
			}
		})
	}
}

// A long --wait ends with the recipient: a run that ended writes no status, and
// the wait for one once lasted as long as asked — an hour for --wait=3600.
func TestALongWaitEndsWithTheRecipient(t *testing.T) {
	dir, _, peer := stateCaller(t, "main")
	go func() {
		time.Sleep(300 * time.Millisecond)
		_ = os.Remove(state.SessionPath(dir, peer.Name))
	}()
	started := time.Now()
	code, out, stderr := run("send", peer.Name, "do it", "--wait", "30")
	if took := time.Since(started); took > 5*time.Second {
		t.Fatalf("waited %s for a recipient that ended at 0.3 s", took)
	}
	if code != ExitFailed || !strings.Contains(out+stderr, "ended before the message was delivered") {
		t.Fatalf("got %d %s %s", code, out, stderr)
	}
}

// A task to itself written before send refused one is not sent again by an
// edit; taking it back stays possible.
func TestAnEditDoesNotSendToItself(t *testing.T) {
	dir, self, _ := stateCaller(t, "general")
	old := inbox.Message{ID: inbox.NewID(), From: self.Name, FromEpoch: self.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Task, Text: "old", CreatedAt: time.Now()}
	if err := inbox.Put(dir, old); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := run("edit", old.ID, "new text", "--wait=0")
	if code != ExitUsage || !strings.Contains(stderr, "cannot send to itself") || !strings.Contains(stderr, "rewake withdraw") {
		t.Fatalf("got %d %s", code, stderr)
	}
	if files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, self.Name), "*.json")); len(files) != 1 {
		t.Fatalf("a refused edit left %v", files)
	}
	if code, out, stderr := run("withdraw", old.ID); code != ExitOK {
		t.Fatalf("withdraw: %d %s %s", code, out, stderr)
	}
}
