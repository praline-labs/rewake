package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// The server writes into the mailbox itself — statuses, temporary files — and
// reacting to its own writes turned one unarchivable message into a loop: write,
// event, pass, write again, hundreds of times a second.
func TestOwnWritesDoNotFeedTheWatch(t *testing.T) {
	dir := stateDir(t)
	sent := message("hello")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	// A file where done/ belongs: archiving can only fail, so the message stays
	// visible and every pass writes its status again.
	if err := os.WriteFile(filepath.Join(state.InboxPath(dir, "api"), "done"), nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered, Via: "socket"}
	}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	time.Sleep(2 * time.Second)
	cancel()
	<-finished

	// Every rewrite of the status touches the file, so its own history is the
	// count: at one retry every two seconds there can only be a couple.
	writes := countStatusWrites(t, dir, sent.ID)
	if writes > 4 {
		t.Fatalf("the status was rewritten %d times in two seconds: the server is answering its own events", writes)
	}
}

// countStatusWrites re-reads the status file repeatedly and counts how often its
// modification time changes.
func countStatusWrites(t *testing.T, dir, id string) int {
	t.Helper()
	path := statusPath(dir, "api", id)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("no status was written at all: %v", err)
	}
	// One sample is enough to tell a loop from a retry: a loop leaves the file
	// changing while nothing else does.
	first := info.ModTime()
	time.Sleep(600 * time.Millisecond)
	again, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if again.ModTime().Equal(first) {
		return 1
	}
	return 10
}

// A mailbox that is removed and recreated leaves the watch pointing at nothing.
// Without asking for a new one, delivery quietly falls back to the poll forever.
func TestWatchSurvivesAReplacedMailbox(t *testing.T) {
	dir := stateDir(t)
	mailbox := state.InboxPath(dir, "api")
	if err := state.EnsureSubdir(mailbox); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := watchMailbox(ctx, dir, "api")
	if changed == nil {
		t.Skip("no watch on this system")
	}

	// Replace the directory the watch was set on.
	if err := os.RemoveAll(mailbox); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := state.EnsureSubdir(mailbox); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Drain whatever the removal itself produced.
	deadline := time.After(time.Second)
drain:
	for {
		select {
		case <-changed:
		case <-deadline:
			break drain
		}
	}

	if err := Put(dir, message("after the mailbox was replaced")); err != nil {
		t.Fatalf("put: %v", err)
	}
	select {
	case <-changed:
	case <-time.After(3 * time.Second):
		t.Fatal("the watch never came back after the mailbox was replaced")
	}
}

// A message answered long ago is swept away with its status. A sender that comes
// back after that must not be told its message is still on its way.
func TestASweptAnswerIsNotReportedAsPending(t *testing.T) {
	dir := stateDir(t)
	sent := message("answered a long time ago")

	// What the sweep leaves behind: no message, no status.
	if err := state.EnsureSubdir(state.DonePath(dir, "api")); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if Answered(dir, "api", sent.ID) != true {
		t.Error("a message that is nowhere to be found was reported as still waiting")
	}

	// And while it is waiting, it is waiting.
	if err := Put(dir, sent); err != nil {
		t.Fatalf("put: %v", err)
	}
	if Answered(dir, "api", sent.ID) {
		t.Error("a message still in the mailbox was reported as answered")
	}
}

// The age that matters is the age of the answer. A rename carries the original
// time along, and an old message refused at startup was swept in the same breath.
func TestArchivingRefreshesTheAge(t *testing.T) {
	dir := stateDir(t)
	old := message("written long ago")
	if err := Put(dir, old); err != nil {
		t.Fatalf("put: %v", err)
	}
	ancient := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(filepath.Join(state.InboxPath(dir, "api"), old.ID+".json"), ancient, ancient); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	if err := archive(dir, "api", old.ID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	info, err := os.Stat(filepath.Join(state.DonePath(dir, "api"), old.ID+".json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if time.Since(info.ModTime()) > time.Minute {
		t.Errorf("the archived message kept the age it was written at: %v", info.ModTime())
	}
}
