package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
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

// A task read by a run that ended owing it is reported lost even in a
// conversation: no release reopens a worker's conversation, so main gives the
// task to a new worker.
func TestAwaitedCallsATaskOfAnEndedRunLost(t *testing.T) {
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
		if views[id].Gone == "" {
			t.Errorf("%s: %+v", id, views[id])
		}
	}
	code, out, _ := run("inbox", "--awaited")
	if code != ExitOK || !strings.Contains(out, " · no report coming: ended ended; check its saved work and give the task to a new worker\nlong job\n") ||
		!strings.Contains(out, " · no report coming: replaced was replaced by a new run; check its saved work and give the task to a new worker\nanother job\n") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	code, _, errOut := run("send", "replaced", "also this", "--to", second)
	if code != ExitFailed || !strings.Contains(errOut, "no report on it is coming") {
		t.Fatalf("an addition to a lost task: exit %d %s", code, errOut)
	}
}
