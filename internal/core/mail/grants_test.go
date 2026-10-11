package mail

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

func grantPending(t *testing.T, dir string, grants ...bool) []Message {
	t.Helper()
	var result []Message
	start := time.Now().Add(-time.Minute)
	for index, granted := range grants {
		m := message(fmt.Sprintf("task %d", index))
		m.Kind, m.FromEpoch, m.ToEpoch = Task, "sender-epoch", "receiver-epoch"
		m.CreatedAt = start.Add(time.Duration(index) * time.Second)
		if granted {
			m.GrantDirs = []string{"/work/lib"}
		}
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
		result = append(result, m)
	}
	return result
}

func TestSettledFollowsTheTaskToItsReport(t *testing.T) {
	dir := stateDir(t)
	m := grantPending(t, dir, true)[0]
	if Settled(dir, "api", m.ID) {
		t.Fatal("a task waiting in the inbox is settled")
	}
	if err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
		if err := LinkUnread(dir, "api", m.ID); err != nil {
			return err
		}
		return MarkRead(dir, "api", "receiver-epoch", m, true)
	}); err != nil {
		t.Fatal(err)
	}
	if Settled(dir, "api", m.ID) {
		t.Fatal("a task read and not reported on is settled")
	}
	for _, waiter := range Waiters(dir, "api", "receiver-epoch") {
		if err := ClearAwaiting(dir, "api", "receiver-epoch", waiter); err != nil {
			t.Fatal(err)
		}
	}
	if !Settled(dir, "api", m.ID) {
		t.Fatal("a task reported on is not settled")
	}

	// Swept: no status and no copy ahead of the reader.
	other := grantPending(t, dir, true)[0]
	if err := os.Remove(filepath.Join(state.InboxPath(dir, "api"), other.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if !Settled(dir, "api", other.ID) {
		t.Fatal("a swept task is not settled")
	}
}
