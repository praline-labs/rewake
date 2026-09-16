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
		if err := linkUnread(dir, sent.To, sent.ID); err != nil {
			t.Fatalf("linkUnread: %v", err)
		}
		settle(dir, sent.To, sent.ID, Delivered)
	}
}

// readAll does what rewake inbox does: look, then mark what was looked at.
func readAll(t *testing.T, dir, epoch string) []Message {
	t.Helper()
	messages, err := PeekUnread(dir, "api", epoch)
	if err != nil {
		t.Fatalf("PeekUnread: %v", err)
	}
	read := make([]Message, 0, len(messages))
	for _, m := range messages {
		moved, err := MarkRead(dir, "api", epoch, m)
		if err != nil {
			t.Fatalf("MarkRead: %v", err)
		}
		if moved {
			read = append(read, m)
		}
	}
	return read
}

func TestReadingHandsEachMessageOnce(t *testing.T) {
	dir := stateDir(t)
	sent := message("pull and rerun the smoke")
	sent.ToEpoch = "5.5"
	unread(t, dir, sent)

	if taken := readAll(t, dir, "5.5"); len(taken) != 1 || taken[0].Text != sent.Text {
		t.Fatalf("first read = %v, want the one message", taken)
	}
	if again := readAll(t, dir, "5.5"); len(again) != 0 {
		t.Fatalf("second read = %v, want nothing: a message is read once", again)
	}
	if moved, _ := MarkRead(dir, "api", "5.5", sent); moved {
		t.Error("a second reader was told it moved a message already read")
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

	if taken := readAll(t, dir, "100.1"); len(taken) != 1 || taken[0].Text != "mine" {
		t.Fatalf("read = %v, want only this epoch's message", taken)
	}
	if n := countUnread(dir, "api", "100.1"); n != 0 {
		t.Errorf("countUnread = %d for this epoch, want 0", n)
	}
}

// Reading a message is what makes its sender wait for the end of the turn. A
// finished notice or a reply is an answer, not a request: waiting on it would
// have two sessions wake each other for nothing.
func TestReadingRecordsWhoWaits(t *testing.T) {
	dir := stateDir(t)
	note := message("please look")
	note.FromEpoch = "40.4"
	done := message("all green")
	done.From, done.FromEpoch, done.Kind = "ops", "41.4", Finished
	answer := message("42")
	answer.From, answer.FromEpoch, answer.Reply = "dev", "42.4", true
	note.ToEpoch, done.ToEpoch, answer.ToEpoch = "5.5", "5.5", "5.5"
	unread(t, dir, note, done, answer)

	readAll(t, dir, "5.5")
	waiting := Waiters(dir, "api", "5.5")
	if len(waiting) != 1 || waiting[0] != (Waiter{Name: "web", Epoch: "40.4"}) {
		t.Fatalf("waiting = %v, want only the run of web that sent the note", waiting)
	}
	if other := Waiters(dir, "api", "6.6"); len(other) != 0 {
		t.Errorf("another run of api sees waiters %v", other)
	}

	ClearAwaiting(dir, "api", "5.5", "web")
	if waiting := Waiters(dir, "api", "5.5"); len(waiting) != 0 {
		t.Errorf("waiting = %v after clearing, want nobody", waiting)
	}
}

// A new run of the name forgets what the previous run was waited on for.
func TestANewRunForgetsTheOldWaits(t *testing.T) {
	dir := stateDir(t)
	if err := markAwaiting(dir, "api", "5.5", "web", "40.4"); err != nil {
		t.Fatalf("markAwaiting: %v", err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "6.6", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered}
	}}
	serveUntil(t, server, func() bool { return len(Waiters(dir, "api", "5.5")) == 0 })
}

// The agent is told a message is waiting and runs rewake inbox straight away.
// By then the message has to be there.
func TestNoticeGoesOutOnceTheMessageIsReadable(t *testing.T) {
	dir := stateDir(t)
	sent := message("pull and rerun the smoke")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}

	readable := make(chan bool, 1)
	server := &Server{Dir: dir, Name: "api", Deliver: func(_ context.Context, m Message) Result {
		_, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), m.ID+".json"))
		select {
		case readable <- err == nil:
		default:
		}
		return Result{State: Delivered, Via: "socket"}
	}}
	var seen bool
	serveUntil(t, server, func() bool {
		select {
		case seen = <-readable:
			return true
		default:
			return false
		}
	})
	if !seen {
		t.Error("the notice went out before the message could be read")
	}
}
