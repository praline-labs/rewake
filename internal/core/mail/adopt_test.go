package mail

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// readIn has run read a task delivered into a conversation.
func readIn(t *testing.T, dir, run, thread string, m Message) {
	t.Helper()
	if err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
		if err := mailtest.WriteDeliveryThread(dir, "api", m.ID, thread); err != nil {
			return err
		}
		if err := LinkUnread(dir, "api", m.ID); err != nil {
			return err
		}
		return MarkRead(dir, "api", run, m, true)
	}); err != nil {
		t.Fatal(err)
	}
}

// Whether a task is open, as main's wrapper asks it of a grant it holds.
func TestTaskOpenFollowsTheTask(t *testing.T) {
	dir := stateDir(t)
	tasks := grantPending(t, dir, true, true, true)
	if open, found := TaskOpen(dir, "api", tasks[0].ID); !open || !found {
		t.Error("a task on its way is not open")
	}
	if err := WriteStatus(dir, "api", tasks[1].ID, Result{State: Failed}); err != nil {
		t.Fatal(err)
	}
	if open, found := TaskOpen(dir, "api", tasks[1].ID); open || !found {
		t.Error("a failed task is open")
	}
	if err := WriteStatus(dir, "api", tasks[2].ID, Result{State: Delivered, Withdrawn: true}); err != nil {
		t.Fatal(err)
	}
	if open, found := TaskOpen(dir, "api", tasks[2].ID); open || !found {
		t.Error("a task taken back is open")
	}
	if _, found := TaskOpen(dir, "api", NewID()); found {
		t.Error("a message never written was found")
	}
}
