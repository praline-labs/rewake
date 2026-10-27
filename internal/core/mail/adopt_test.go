package mail

import (
	"context"
	"fmt"
	"testing"
	"time"

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
		return markRead(dir, "api", run, m, true)
	}); err != nil {
		t.Fatal(err)
	}
}

// pendingTasks writes count tasks from one sender's run, a second apart, that
// wait in the inbox.
func pendingTasks(t *testing.T, dir string, count int) []Message {
	t.Helper()
	var result []Message
	start := time.Now().Add(-time.Minute)
	for index := range count {
		m := message(fmt.Sprintf("task %d", index))
		m.Kind, m.FromEpoch, m.ToEpoch = Task, "sender-epoch", "receiver-epoch"
		m.CreatedAt = start.Add(time.Duration(index) * time.Second)
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
		result = append(result, m)
	}
	return result
}
