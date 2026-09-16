package inbox

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// The notice says how many messages wait, so the second one delivered before the
// agent read the first must say two.
func TestNoticeCountsTheWaitingMail(t *testing.T) {
	dir := stateDir(t)
	first, second := message("one"), message("two")
	for _, sent := range []Message{first, second} {
		if err := Put(dir, sent); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	var mu sync.Mutex
	counts := map[string]int{}
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m Message) Result {
		mu.Lock()
		defer mu.Unlock()
		counts[m.ID] = m.Unread
		return Result{State: Delivered, Via: "socket"}
	}}
	serveUntil(t, server, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(counts) == 2
	})

	if counts[first.ID] != 1 || counts[second.ID] != 2 {
		t.Errorf("counts = %v, want 1 for the first and 2 for the second", counts)
	}
}

func unread(t *testing.T, dir string, messages ...Message) {
	t.Helper()
	for _, sent := range messages {
		if err := Put(dir, sent); err != nil {
			t.Fatalf("Put: %v", err)
		}
		if err := markUnread(dir, sent.To, sent.ID); err != nil {
			t.Fatalf("markUnread: %v", err)
		}
	}
}

func TestReadingHandsEachMessageOnce(t *testing.T) {
	dir := stateDir(t)
	sent := message("pull and rerun the smoke")
	unread(t, dir, sent)

	taken, err := TakeUnread(dir, "api", "")
	if err != nil || len(taken) != 1 || taken[0].Text != sent.Text {
		t.Fatalf("TakeUnread = %v, %v; want the one message", taken, err)
	}
	again, err := TakeUnread(dir, "api", "")
	if err != nil || len(again) != 0 {
		t.Fatalf("second TakeUnread = %v, %v; want nothing: a message is read once", again, err)
	}

	status, ok := ReadStatus(dir, "api", sent.ID)
	if !ok || status.State != Read {
		t.Errorf("status = %+v, want read", status)
	}
	if _, err := os.Stat(filepath.Join(state.DonePath(dir, "api"), sent.ID+".json")); err != nil {
		t.Errorf("a read message was not archived: %v", err)
	}
}

// A name can be reused. Mail left unread by an earlier session with this name
// is not the current session's to read.
func TestReadingKeepsToItsOwnEpoch(t *testing.T) {
	dir := stateDir(t)
	mine, theirs := message("mine"), message("theirs")
	mine.ToEpoch, theirs.ToEpoch = "100.1", "99.1"
	unread(t, dir, mine, theirs)

	taken, err := TakeUnread(dir, "api", "100.1")
	if err != nil || len(taken) != 1 || taken[0].Text != "mine" {
		t.Fatalf("TakeUnread = %v, %v; want only this epoch's message", taken, err)
	}
	if n := countUnread(dir, "api", "100.1"); n != 0 {
		t.Errorf("countUnread = %d for this epoch, want 0", n)
	}
}

// Reading a message is what makes its sender wait for the end of the turn. A
// finished notice is an answer, not a request: waiting on it would have two
// sessions report their turns to each other forever.
func TestReadingRecordsWhoWaits(t *testing.T) {
	dir := stateDir(t)
	note := message("please look")
	done := message("all green")
	done.From, done.Kind = "ops", Finished
	unread(t, dir, note, done)

	if _, err := TakeUnread(dir, "api", ""); err != nil {
		t.Fatalf("TakeUnread: %v", err)
	}
	waiting := TakeAwaiting(dir, "api")
	if len(waiting) != 1 || waiting[0] != "web" {
		t.Fatalf("waiting = %v, want only the sender of the note", waiting)
	}
	if again := TakeAwaiting(dir, "api"); len(again) != 0 {
		t.Errorf("waiting after taking = %v, want each told once", again)
	}
}

func TestAnsweringClearsTheWait(t *testing.T) {
	dir := stateDir(t)
	unread(t, dir, message("please look"))
	if _, err := TakeUnread(dir, "api", ""); err != nil {
		t.Fatalf("TakeUnread: %v", err)
	}

	if !ClearAwaiting(dir, "api", "web") {
		t.Error("ClearAwaiting says web was not waiting")
	}
	if waiting := TakeAwaiting(dir, "api"); len(waiting) != 0 {
		t.Errorf("waiting = %v, want nobody after a direct answer", waiting)
	}
}

// An answer does not ask for a report back, or every exchange would end with
// the first session woken for a notice about its own answer being read.
func TestReadingAReplyAsksForNothing(t *testing.T) {
	dir := stateDir(t)
	answer := message("42")
	answer.Reply = true
	unread(t, dir, answer)

	if _, err := TakeUnread(dir, "api", ""); err != nil {
		t.Fatalf("TakeUnread: %v", err)
	}
	if waiting := TakeAwaiting(dir, "api"); len(waiting) != 0 {
		t.Errorf("waiting = %v, want nobody after reading a reply", waiting)
	}
}
