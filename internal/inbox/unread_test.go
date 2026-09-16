package inbox

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

// readAll does what rewake inbox does, under the same lock: look, then mark
// what was looked at. Taking the real lock is the point — a reader that skips
// it passes where rewake inbox would hang.
func readAll(t *testing.T, dir, epoch string) []Message {
	t.Helper()
	var messages []Message
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := state.WithMailboxLock(ctx, dir, "api", func() error {
		var err error
		if messages, err = PeekUnread(dir, "api", epoch); err != nil {
			return err
		}
		for _, m := range messages {
			if err := MarkRead(dir, "api", epoch, m); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading under the mailbox lock: %v", err)
	}
	return messages
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
// finished notice is an answer, not a request: waiting on it would have two
// sessions report their turns to each other forever. An ordinary answer is not
// exempt: it may carry a new request, and its sender is owed the report.
func TestReadingRecordsWhoWaits(t *testing.T) {
	dir := stateDir(t)
	note := message("please look")
	note.FromEpoch = "40.4"
	done := message("all green")
	done.From, done.FromEpoch, done.Kind = "ops", "41.4", Finished
	answer := message("42")
	answer.From, answer.FromEpoch = "dev", "42.4"
	note.ToEpoch, done.ToEpoch, answer.ToEpoch = "5.5", "5.5", "5.5"
	unread(t, dir, note, done, answer)

	readAll(t, dir, "5.5")
	waiting := Waiters(dir, "api", "5.5")
	want := []Waiter{{Name: "dev", Epoch: "42.4"}, {Name: "web", Epoch: "40.4"}}
	same := func(a, b Waiter) bool { return a.Name == b.Name && a.Epoch == b.Epoch && a.Since != 0 }
	if len(waiting) != 2 || !same(waiting[0], want[0]) || !same(waiting[1], want[1]) {
		t.Fatalf("waiting = %v, want %v with the time each began, and nobody for the finished notice", waiting, want)
	}
	if other := Waiters(dir, "api", "6.6"); len(other) != 0 {
		t.Errorf("another run of api sees waiters %v", other)
	}

	for _, waiter := range waiting {
		ClearAwaiting(dir, "api", "5.5", waiter)
	}
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

// A message linked for an earlier attempt can be read before the next notice
// goes out. The read stands: the task is not announced, or handed out, again.
func TestAReadDuringAPendingNoticeStands(t *testing.T) {
	dir := stateDir(t)
	sent := message("execute once")
	sent.ToEpoch = "5.5"
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}
	reads, notices := 0, 0
	server := &Server{Dir: dir, Name: "api", Epoch: "5.5", attempts: map[string]time.Time{}, outcomes: map[string]Result{}}
	server.Deliver = func(context.Context, Message) Result {
		notices++
		reads += len(readAll(t, dir, "5.5"))
		return Result{State: Pending, Detail: "queue not ready"}
	}

	server.drain(context.Background())
	server.attempts[sent.ID] = time.Now().Add(-2 * retryInterval)
	server.drain(context.Background())

	if reads != 1 || notices != 1 {
		t.Errorf("reads = %d, notices = %d; want the task read once and announced once", reads, notices)
	}
	if status, _ := ReadStatus(dir, "api", sent.ID); status.State != Read {
		t.Errorf("status = %s, want read", status.State)
	}
}

// Whatever the harness says after the agent has read the message — even that
// the notice failed — the message was read, and its sender must hear that.
func TestAReadIsNotUndoneByTheNoticeResult(t *testing.T) {
	for _, outcome := range []State{Failed, Delivered} {
		t.Run(string(outcome), func(t *testing.T) {
			dir := stateDir(t)
			sent := message("already acted on")
			if err := Put(dir, sent); err != nil {
				t.Fatalf("Put: %v", err)
			}
			server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}}
			server.Deliver = func(context.Context, Message) Result {
				readAll(t, dir, "")
				return Result{State: outcome, Detail: "said after the read"}
			}
			server.drain(context.Background())
			if status, _ := ReadStatus(dir, "api", sent.ID); status.State != Read {
				t.Errorf("status = %s, want read", status.State)
			}
		})
	}
}

// Clearing the report owed to an ended run of a name must not clear the one
// owed to the run that wrote since.
func TestClearingAnOldRunKeepsTheNewOne(t *testing.T) {
	dir := stateDir(t)
	if err := markAwaiting(dir, "api", "5.5", "web", "40.4"); err != nil {
		t.Fatalf("markAwaiting: %v", err)
	}
	old := Waiters(dir, "api", "5.5")[0]
	if err := markAwaiting(dir, "api", "5.5", "web", "41.4"); err != nil {
		t.Fatalf("markAwaiting: %v", err)
	}

	ClearAwaiting(dir, "api", "5.5", old)
	if remaining := Waiters(dir, "api", "5.5"); len(remaining) != 1 || remaining[0].Epoch != "41.4" {
		t.Errorf("waiting = %v, want the newer run of web still owed", remaining)
	}
}

// A reader stuck on its output holds the mailbox. The server must still end
// when its session does: waiting on the lock without end kept the wrapper alive
// after its harness had gone.
func TestAStuckReaderDoesNotHoldTheServer(t *testing.T) {
	dir := stateDir(t)
	held := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_ = state.WithMailboxLock(context.Background(), dir, "api", func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	defer close(release)

	sent := message("waits for the reader")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		server.Serve(ctx)
	}()
	time.Sleep(1500 * time.Millisecond)
	cancel()

	select {
	case <-finished:
	case <-time.After(shutdownLockWait + 2*time.Second):
		t.Fatal("the server did not end while a reader held the mailbox")
	}
}

// A lock nobody can take does not stop delivery: the server is then the only
// writer, and the sender hears what happened instead of a silent pending.
func TestAnUnusableLockDoesNotStopDelivery(t *testing.T) {
	dir := stateDir(t)
	if err := os.MkdirAll(filepath.Join(state.InboxPath(dir, "api"), ".lock"), 0o700); err != nil {
		t.Fatalf("block: %v", err)
	}
	sent := message("still delivered")
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}
	server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
		return Result{State: Delivered, Via: "socket"}
	}}
	serveUntil(t, server, func() bool {
		status, ok := ReadStatus(dir, "api", sent.ID)
		return ok && status.State == Delivered
	})
}

// A reader running inside delivery — the agent reads the moment it is told —
// takes the real lock, and must get it: delivery does not hold the mailbox.
func TestAReaderDuringDeliveryGetsTheLock(t *testing.T) {
	dir := stateDir(t)
	sent := message("task")
	sent.ToEpoch = "5.5"
	if err := Put(dir, sent); err != nil {
		t.Fatalf("Put: %v", err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "5.5", attempts: map[string]time.Time{}, outcomes: map[string]Result{}}
	server.Deliver = func(context.Context, Message) Result {
		readAll(t, dir, "5.5")
		return Result{State: Pending}
	}
	done := make(chan struct{})
	go func() {
		server.drain(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a reader could not get the mailbox while a notice was on its way")
	}
	if status, _ := ReadStatus(dir, "api", sent.ID); status.State != Read {
		t.Errorf("status = %s, want read", status.State)
	}
}

// A new request from the same run, read after the report was written, is a new
// wait. Clearing the old one must not take it along.
func TestClearingAnOldWaitKeepsANewOneOfTheSameRun(t *testing.T) {
	dir := stateDir(t)
	if err := markAwaiting(dir, "api", "5.5", "web", "40.4"); err != nil {
		t.Fatalf("markAwaiting: %v", err)
	}
	old := Waiters(dir, "api", "5.5")[0]
	time.Sleep(time.Millisecond)
	if err := markAwaiting(dir, "api", "5.5", "web", "40.4"); err != nil {
		t.Fatalf("markAwaiting: %v", err)
	}

	ClearAwaiting(dir, "api", "5.5", old)
	remaining := Waiters(dir, "api", "5.5")
	if len(remaining) != 1 || remaining[0].Since == old.Since {
		t.Fatalf("waiting = %v, want the newer wait still owed", remaining)
	}
	if ReportID("api", "5.5", old) == ReportID("api", "5.5", remaining[0]) {
		t.Error("two waits of one run share a report id, so the second report would be skipped")
	}
}
