package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/state"
)

// rewake inbox --owed only informs, so a grant journal it cannot read is
// folded into the cautious answer: the grant is not ended, shown with the
// caveat a counted grant always carries, never as taken back on a guess
// and never dropped from the view. It is ended again once the journal reads.
func TestAnUnreadableGrantJournalDoesNotEndTheGrant(t *testing.T) {
	w := newGrantWorld(t, "main", dirGrantHarness(t))
	lib := filepath.Join(w.home, "work/lib")
	t.Chdir(filepath.Join(w.home, "work"))
	if code, out, stderr := run("send", w.peer.Name, "Bump the client", "--wait=0", "--grant-dir", lib); code != ExitPending {
		t.Fatalf("send: %d %s %s", code, out, stderr)
	}
	message := w.sent(t)[0]
	if err := state.EnsureSubdir(state.UnreadPath(w.dir, w.peer.Name)); err != nil {
		t.Fatal(err)
	}
	from := filepath.Join(state.InboxPath(w.dir, w.peer.Name), message.ID+".json")
	if err := os.Link(from, filepath.Join(state.UnreadPath(w.dir, w.peer.Name), message.ID+".json")); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, w.peer.Name)
	t.Setenv(state.EpochEnv, w.peer.Epoch())
	if code, out, stderr := run("inbox"); code != ExitOK {
		t.Fatalf("inbox: %d %s %s", code, out, stderr)
	}
	if err := grant.Save(w.dir, w.peer.Name, w.peer.Epoch(), []grant.Entry{{Path: lib, Message: message.ID, Outcome: grant.Dropped}}); err != nil {
		t.Fatal(err)
	}
	journals, err := filepath.Glob(filepath.Join(w.dir, "grants", "*.json"))
	if err != nil || len(journals) != 1 {
		t.Fatalf("the grant journal: %v %v", journals, err)
	}
	if err := os.Chmod(journals[0], 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(journals[0], 0o600) })
	code, out, _ := run("inbox", "--owed")
	if code != ExitOK || !strings.Contains(out, "grant: write "+lib+" — unless a turn typed") || strings.Contains(out, "no longer writable") {
		t.Fatalf("inbox --owed past an unreadable grant journal: %d %s", code, out)
	}
	code, out, _ = run("inbox", "--owed", "--json")
	var model struct {
		Messages []struct {
			ID         string            `json:"id"`
			GrantEnded map[string]string `json:"grantEnded"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(out), &model); code != ExitOK || err != nil || len(model.Messages) != 1 || model.Messages[0].GrantEnded != nil {
		t.Fatalf("inbox --owed --json past an unreadable grant journal: %d %s %v", code, out, err)
	}
	if err := os.Chmod(journals[0], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, out, _ := run("inbox", "--owed"); !strings.Contains(out, "grant: dropped "+lib+" — no longer writable") {
		t.Fatalf("inbox --owed once the journal reads: %s", out)
	}
}

// dirGrantHarness names a harness that takes a directory into a running
// session, found by the capability rather than by name.
func dirGrantHarness(t *testing.T) string {
	t.Helper()
	for _, h := range harness.All() {
		if capable, ok := h.(harness.DirGrantHarness); ok && capable.SupportsDirGrant() {
			return h.ID()
		}
	}
	t.Skip("no harness takes a directory into a running session")
	return ""
}
