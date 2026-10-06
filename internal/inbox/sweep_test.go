package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// A message arriving while the session runs is delivered at once, not when the
// next poll happens to come round. The ticker is deliberately slow, so a server
// that only polled would fail this.
func TestANewMessageIsNoticedWithoutWaitingForThePoll(t *testing.T) {
	dir := stateDir(t)

	delivered := make(chan string, 1)
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m Message) Result {
		select {
		case delivered <- m.Text:
		default:
		}
		return Result{State: Delivered}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	defer func() {
		cancel()
		<-finished
	}()

	// Let the watch settle, then write a message the way a sender does.
	time.Sleep(200 * time.Millisecond)
	began := time.Now()
	if err := Put(dir, message("pull and rerun the smoke")); err != nil {
		t.Fatalf("put: %v", err)
	}

	select {
	case <-delivered:
		// Well inside the poll: a server that only polled would answer at the
		// tick, and the point of the watch is that it does not.
		if waited := time.Since(began); waited > pollInterval/2 {
			t.Errorf("the message waited %v, which is the poll rather than the watch", waited)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the message was never delivered")
	}
}

// Answered messages are kept for a while and then let go: a machine that runs
// for weeks should not collect a mailbox full of last month's conversations.
func TestFinishedMessagesAreSweptByAge(t *testing.T) {
	dir := stateDir(t)
	run := liveRunOf(t, dir, "api")
	server := &Server{Dir: dir, Name: "api", Epoch: run}

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

	server.sweepFinished()

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
		if err := linkUnread(dir, "api", m.ID); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(state.InboxPath(dir, "api"), m.ID+".json")); err != nil {
			t.Fatal(err)
		}
		if err := MarkRead(dir, "api", run, *m, true); err != nil {
			t.Fatal(err)
		}
	}
	for _, waiter := range Waiters(dir, "api", run) {
		waiter.Messages = []string{reported.ID}
		if err := ClearAwaiting(dir, "api", run, waiter); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * keepFinished)
	for _, m := range []Message{owedTask, reported} {
		if err := os.Chtimes(filepath.Join(state.DonePath(dir, "api"), m.ID+".json"), old, old); err != nil {
			t.Fatal(err)
		}
	}
	(&Server{Dir: dir, Name: "api", Epoch: run}).sweepFinished()
	if _, err := os.Stat(filepath.Join(state.DonePath(dir, "api"), owedTask.ID+".json")); err != nil {
		t.Fatalf("an owed task was swept: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.DonePath(dir, "api"), reported.ID+".json")); !os.IsNotExist(err) {
		t.Fatalf("a reported task outlived its age: %v", err)
	}
	if got := must(OwedMessages(dir, "api", run)); len(got) != 1 || got[0].ID != owedTask.ID || !got[0].Kept {
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
	if err := linkUnread(dir, "api", task.ID); err != nil {
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
	(&Server{Dir: dir, Name: "api", Epoch: run}).sweepFinished()
	if _, err := os.Stat(unread); err != nil {
		t.Fatalf("an owed task left in unread/ was swept: %v", err)
	}
	if got := must(OwedMessages(dir, "api", run)); len(got) != 1 || !got[0].Kept || got[0].Text != task.Text {
		t.Fatalf("owed %+v", got)
	}
}
