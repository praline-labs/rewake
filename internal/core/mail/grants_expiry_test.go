package mail

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A task whose status was swept leaves its done copy: that is enough to know
// the task was there and is closed, so main lets its grant go at once rather
// than holding it for the lifetime of a letter it cannot find. A wait that
// still stands keeps it open all the same.
func TestATaskFoundOnlyInDoneIsClosed(t *testing.T) {
	dir := stateDir(t)
	tasks := grantPending(t, dir, true, true)
	readIn(t, dir, "receiver-epoch", "thread-a", tasks[1])
	if err := state.EnsureSubdir(state.DonePath(dir, "api")); err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if err := os.Rename(filepath.Join(state.InboxPath(dir, "api"), task.ID+".json"), filepath.Join(state.DonePath(dir, "api"), task.ID+".json")); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{filepath.Join(state.UnreadPath(dir, "api"), task.ID+".json"), statusPath(dir, "api", task.ID)} {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
	if open, found := TaskOpen(dir, "api", tasks[0].ID); open || !found {
		t.Errorf("done: open %v found %v", open, found)
	}
	if !Settled(dir, "api", tasks[0].ID) {
		t.Error("done: not settled")
	}
	if open, found := TaskOpen(dir, "api", tasks[1].ID); !open || !found {
		t.Errorf("done and owed: open %v found %v", open, found)
	}
	if Settled(dir, "api", tasks[1].ID) {
		t.Error("done and owed: settled")
	}
}
