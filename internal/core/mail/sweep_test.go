package mail

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Answered messages are kept for a while and then let go: a machine that runs
// for weeks should not collect a mailbox full of last month's conversations.
func TestFinishedMessagesAreSweptByAge(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")

	done := state.DonePath(dir, "api")
	if err := state.EnsureSubdir(done); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mailbox := state.InboxPath(dir, "api")
	unread := state.UnreadPath(dir, "api")
	if err := state.EnsureSubdir(unread); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	old := time.Now().Add(-keepFinished - time.Hour)
	recent := time.Now().Add(-time.Minute)
	files := map[string]time.Time{
		filepath.Join(done, "old.json"):        old,
		filepath.Join(done, "recent.json"):     recent,
		filepath.Join(mailbox, "old.status"):   old,
		filepath.Join(mailbox, "fresh.status"): recent,
		filepath.Join(unread, "old.json"):      old,
		filepath.Join(unread, "recent.json"):   recent,
	}
	for path, when := range files {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	// A message still waiting is answered by the TTL, not by the sweep.
	waiting := message("still waiting")
	if err := Put(dir, waiting); err != nil {
		t.Fatalf("put: %v", err)
	}
	stale := filepath.Join(mailbox, waiting.ID+".json")
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	sweepAsLiveRun(t, dir, "api", run)

	for path, when := range files {
		_, err := os.Stat(path)
		if when.Equal(old) && !os.IsNotExist(err) {
			t.Errorf("%s survived the sweep", filepath.Base(path))
		}
		if when.Equal(recent) && err != nil {
			t.Errorf("%s was swept too early: %v", filepath.Base(path), err)
		}
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("a message that is still waiting was swept: %v", err)
	}
}

// A task read long ago and still owed keeps its text: rewake inbox --owed has
// to be able to show it again. What is no longer owed goes by age as before.
func TestAnOwedMessageOutlivesTheSweep(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	owedTask, reported := message("still being worked on"), message("reported long ago")
	for _, m := range []*Message{&owedTask, &reported} {
		m.Kind, m.FromEpoch, m.ToEpoch = Task, "web-epoch", run
		if err := Put(dir, *m); err != nil {
			t.Fatal(err)
		}
		if err := LinkUnread(dir, "api", m.ID); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(state.InboxPath(dir, "api"), m.ID+".json")); err != nil {
			t.Fatal(err)
		}
		if err := markRead(dir, "api", run, *m, true); err != nil {
			t.Fatal(err)
		}
	}
	for _, waiter := range waitersOf(dir, "api", run) {
		waiter.Messages = []string{reported.ID}
		if err := clearAwaiting(dir, "api", run, waiter); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * keepFinished)
	for _, m := range []Message{owedTask, reported} {
		if err := os.Chtimes(filepath.Join(state.DonePath(dir, "api"), m.ID+".json"), old, old); err != nil {
			t.Fatal(err)
		}
	}
	sweepAsLiveRun(t, dir, "api", run)
	if _, err := os.Stat(filepath.Join(state.DonePath(dir, "api"), owedTask.ID+".json")); err != nil {
		t.Fatalf("an owed task was swept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.DonePath(dir, "api"), reported.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("a reported task outlived its age: %v", err)
	}
	if got := must(owedMessages(dir, "api", run)); len(got) != 1 || got[0].ID != owedTask.ID || !got[0].Kept {
		t.Fatalf("owed %+v", got)
	}
}

// A read whose last step failed left the text in unread/; while it is owed,
// the age sweep leaves it there too.
func TestAnOwedMessageLeftUnreadOutlivesTheSweep(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	task := message("read, but the move to done/ failed")
	task.Kind, task.FromEpoch, task.ToEpoch = Task, "web-epoch", run
	if err := Put(dir, task); err != nil {
		t.Fatal(err)
	}
	if err := LinkUnread(dir, "api", task.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(state.InboxPath(dir, "api"), task.ID+".json")); err != nil {
		t.Fatal(err)
	}
	if err := markAwaiting(dir, "api", run, "web", "web-epoch", task.ID); err != nil {
		t.Fatal(err)
	}
	unread := filepath.Join(state.UnreadPath(dir, "api"), task.ID+".json")
	old := time.Now().Add(-2 * keepFinished)
	if err := os.Chtimes(unread, old, old); err != nil {
		t.Fatal(err)
	}
	sweepAsLiveRun(t, dir, "api", run)
	if _, err := os.Stat(unread); err != nil {
		t.Fatalf("an owed task left in unread/ was swept: %v", err)
	}
	if got := must(owedMessages(dir, "api", run)); len(got) != 1 || !got[0].Kept || got[0].Text != task.Text {
		t.Fatalf("owed %+v", got)
	}
}
