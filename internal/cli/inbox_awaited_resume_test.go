package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// pinned records the conversation a task was delivered into, as the delivery
// does before the task becomes readable.
func pinned(t *testing.T, dir, name, id, thread string) {
	t.Helper()
	threads := filepath.Join(state.InboxPath(dir, name), "threads")
	if err := os.MkdirAll(threads, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threads, id), []byte(thread), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A task read by a run that ended owing it, in a conversation a resume may
// continue, is not reported lost: main would send it again, and the resume
// would do the work a second time. That holds while no new run has started,
// and while a new one has not yet learned its conversation and swept the
// wait; an addition to it is refused, saying so.
func TestAwaitedDoesNotCallAResumableTaskLost(t *testing.T) {
	room := newAwaitedRoom(t)
	ended := otherRun(t, room.dir, "ended")
	replaced := otherRun(t, room.dir, "replaced")
	first := room.put(ended, "unread", "delivered", "", map[string]any{"text": "long job"})
	pinned(t, room.dir, ended.Name, first, "c1")
	room.read(ended)
	second := room.put(replaced, "unread", "delivered", "", map[string]any{"text": "another job"})
	pinned(t, room.dir, replaced.Name, second, "c2")
	room.read(replaced)
	if err := os.Remove(state.SessionPath(room.dir, ended.Name)); err != nil {
		t.Fatal(err)
	}
	otherRun(t, room.dir, "replaced")

	views := byID(awaitedJSON(t))
	for _, id := range []string{first, second} {
		if !views[id].Resumable || views[id].Gone != "" {
			t.Errorf("%s: %+v", id, views[id])
		}
	}
	code, out, _ := run("inbox", "--awaited")
	if code != ExitOK || strings.Contains(out, "no report coming") || strings.Contains(out, "will not come") ||
		!strings.Contains(out, " · ended ended; a resume of ended in its conversation may still report\nlong job\n") ||
		!strings.Contains(out, " · replaced ended; a resume of replaced in its conversation may still report\nanother job\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	code, _, errOut := run("send", "replaced", "also this", "--to", second)
	if code != ExitFailed || !strings.Contains(errOut, "may still report on it") {
		t.Fatalf("an addition to a resumable task: exit %d %s", code, errOut)
	}
}
