package mail

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// readLab is api's run 5.5 with letters announced to it.
func readLab(t *testing.T, messages ...Message) (string, Box) {
	t.Helper()
	dir := stateDir(t)
	for index := range messages {
		if messages[index].ToEpoch == "" {
			messages[index].ToEpoch = "5.5"
		}
	}
	unread(t, dir, messages...)
	return dir, Open(dir, "api", "5.5")
}

// task is a task from web's run 40.4.
func task(text string) Message {
	sent := message(text)
	sent.FromEpoch, sent.Kind = "40.4", Task
	return sent
}

func stepOf(err error) Step {
	var step *StepError
	if errors.As(err, &step) {
		return step.Step
	}
	return 0
}

func unreadCount(t *testing.T, box Box) int {
	t.Helper()
	messages, err := box.PeekUnread()
	if err != nil {
		t.Fatal(err)
	}
	return len(messages)
}

func showNothing([]Message) error { return nil }

// A read whose showing failed counts nothing as read: the letters stay
// unread and nobody is owed a report yet (E5).
func TestAReadThatCouldNotShowLeavesTheLettersUnread(t *testing.T) {
	_, box := readLab(t, task("keep me"), task("and me"))
	_, err := box.Read(t.Context(), ReadRequest{Reports: true}, func([]Message) error { return errors.New("no space left on device") })
	if stepOf(err) != StepShow {
		t.Fatalf("a failed showing answered %v", err)
	}
	if unreadCount(t, box) != 2 || len(box.Waiters()) != 0 {
		t.Fatalf("after a failed showing: %d unread, %v waiting", unreadCount(t, box), box.Waiters())
	}
}

// A read whose record failed after the showing leaves the letter unread; the
// next read shows it again and owes its sender one report.
func TestAReadWhoseRecordFailedIsReadAgainOwingOnce(t *testing.T) {
	sent := task("report back")
	dir, box := readLab(t, sent)
	blocked := state.AwaitingPath(dir, "api")
	if err := os.MkdirAll(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
	if _, err := box.Read(t.Context(), ReadRequest{Reports: true}, showNothing); stepOf(err) != StepMark {
		t.Fatalf("a failed record answered %v", err)
	}
	if err := os.Chmod(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	read, err := box.Read(t.Context(), ReadRequest{Reports: true}, showNothing)
	if err != nil || len(read) != 1 || read[0].ID != sent.ID {
		t.Fatalf("the retry read %v, %v", read, err)
	}
	if waiters := box.Waiters(); len(waiters) != 1 || len(waiters[0].Messages) != 1 {
		t.Fatalf("after the retry api owes %+v, want one report", waiters)
	}
}

// Two reads at once — parallel tool calls, a command run again while the
// first still prints — do not both take the same task.
func TestTwoReadsAtOnceTakeATaskOnce(t *testing.T) {
	dir, box := readLab(t, task("execute once"))
	waiting := make(chan struct{})
	var once sync.Once
	state.LockWait = func(string) { once.Do(func() { close(waiting) }) }
	t.Cleanup(func() { state.LockWait = nil })

	var shown sync.Map
	count := func(name string) func([]Message) error {
		return func(messages []Message) error {
			shown.Store(name, len(messages))
			return nil
		}
	}
	second := make(chan error, 1)
	_, err := box.Read(t.Context(), ReadRequest{Reports: true}, func(messages []Message) error {
		go func() {
			_, err := Open(dir, "api", "5.5").Read(context.Background(), ReadRequest{Reports: true}, count("second"))
			second <- err
		}()
		// The second read is let through only once it waits for the lock.
		select {
		case <-waiting:
		case <-time.After(5 * time.Second):
			t.Error("the second read never waited for the lock")
		}
		return count("first")(messages)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	first, _ := shown.Load("first")
	other, _ := shown.Load("second")
	if first != 1 || other != 0 {
		t.Fatalf("the first read was shown %v letters, the second %v; want the task once", first, other)
	}
}

// A read whose mailbox lock stays held takes nothing, and says the lock
// stopped it.
func TestABusyMailboxIsReadLater(t *testing.T) {
	dir, box := readLab(t, task("later"))
	short, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := withLock(dir, "api", func() error {
		_, err := box.Read(short, ReadRequest{Reports: true}, showNothing)
		return err
	})
	if stepOf(err) != StepLock || !errors.Is(err, state.ErrMailboxBusy) {
		t.Fatalf("a held lock answered %v", err)
	}
	if unreadCount(t, box) != 1 {
		t.Fatal("a read that took no lock took the letter")
	}
}

// A selected letter is read alone, and one that is not available — unknown,
// or a report held back for the question waiting on it — is refused.
func TestASelectedLetterIsReadAlone(t *testing.T) {
	held := message("the answer")
	held.Kind, held.FromEpoch, held.InReplyTo = Finished, "40.4", []string{"question"}
	first, second := task("first"), task("second")
	_, box := readLab(t, first, second, held)
	release, err := box.ReserveAnswer("question")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, id := range []string{"unknown", held.ID} {
		if _, err := box.Read(t.Context(), ReadRequest{Selected: id, Reports: true}, showNothing); stepOf(err) != StepSelect {
			t.Fatalf("selecting %s answered %v", id, err)
		}
	}
	read, err := box.Read(t.Context(), ReadRequest{Selected: second.ID, Reports: true}, showNothing)
	if err != nil || len(read) != 1 || read[0].ID != second.ID {
		t.Fatalf("the selected read took %v, %v", read, err)
	}
	if unreadCount(t, box) != 2 {
		t.Fatal("the selected read took its neighbors")
	}
}

// A peek shows what is available and counts nothing as read.
func TestAPeekMarksNothing(t *testing.T) {
	_, box := readLab(t, task("look"), task("only"))
	read, err := box.Read(t.Context(), ReadRequest{Peek: true, Reports: true}, showNothing)
	if err != nil || len(read) != 2 {
		t.Fatalf("the peek showed %v, %v", read, err)
	}
	if unreadCount(t, box) != 2 || len(box.Waiters()) != 0 {
		t.Fatal("the peek counted a letter read")
	}
}

// A task written for an earlier run of the name is read by the run now, and
// owes nothing: its sender counts it lost. A reader whose role reports
// nothing owes nothing either.
func TestOnlyTheReadersOwnRunsTasksAreOwed(t *testing.T) {
	earlier := task("from before")
	earlier.ToEpoch = "4.4"
	_, box := readLab(t, earlier, task("now"))
	read, err := box.Read(t.Context(), ReadRequest{Reports: true}, showNothing)
	if err != nil || len(read) != 2 {
		t.Fatalf("read %v, %v", read, err)
	}
	if waiters := box.Waiters(); len(waiters) != 1 || len(waiters[0].Messages) != 1 || waiters[0].Messages[0] == earlier.ID {
		t.Fatalf("api owes %+v, want only the task of its own run", waiters)
	}

	_, main := readLab(t, task("for main"))
	if _, err := main.Read(t.Context(), ReadRequest{}, showNothing); err != nil {
		t.Fatal(err)
	}
	if len(main.Waiters()) != 0 {
		t.Fatal("a reader that reports nothing was made to owe a report")
	}
}
