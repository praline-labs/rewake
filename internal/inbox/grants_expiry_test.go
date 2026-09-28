package inbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// A grant whose message outlived its wait is refused before main is asked,
// and its sender is told: a grant main let go after its lifetime would be
// refused anyway, and asking a busy main only delays the answer.
func TestAnExpiredGrantIsRefusedWithoutAskingMain(t *testing.T) {
	dir := stateDir(t)
	task := grantPending(t, dir, true)[0]
	s := batchServer(dir)
	s.TTL = time.Second
	asked := false
	s.CheckGrant = func(Message) error { asked = true; return nil }
	s.Deliver = func(context.Context, Message) Result {
		t.Fatal("an expired grant was delivered")
		return Result{}
	}
	s.drain(context.Background())
	if asked {
		t.Error("main was asked about an expired grant")
	}
	status, ok := ReadStatus(dir, "api", task.ID)
	if !ok || status.State != Failed || !strings.Contains(status.Detail, "expired") {
		t.Fatalf("status = %+v", status)
	}
	told, err := list(dir, "web")
	if err != nil || len(told) != 1 || told[0].Undelivered == nil || told[0].Undelivered.ID != task.ID {
		t.Fatalf("sender was not told: %+v %v", told, err)
	}
}

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
