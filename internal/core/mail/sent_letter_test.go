package mail

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// sentLab is web's run sender-epoch sending to api, a live run of this
// process.
type sentLab struct {
	dir string
	api registry.Session
	box Box
}

func newSentLab(t *testing.T) sentLab {
	t.Helper()
	dir := stateDir(t)
	return sentLab{dir: dir, api: publishAPI(t, dir, "write"), box: Open(dir, "web", "sender-epoch")}
}

// send writes a letter of web's to api, as send does.
func (l sentLab) send(t *testing.T, kind Kind, text, addendumTo string) Message {
	t.Helper()
	time.Sleep(time.Microsecond)
	letter := Message{ID: NewID(), From: "web", FromEpoch: "sender-epoch", To: "api", ToEpoch: l.api.Epoch(), Kind: kind, Text: text, AddendumTo: addendumTo, CreatedAt: time.Now()}
	if err := l.box.Send(t.Context(), letter, l.api); err != nil {
		t.Fatal(err)
	}
	return letter
}

// readLocked has api read a letter, as its reader does under the lock the
// caller holds.
func (l sentLab) readLocked(letter Message) error {
	if err := LinkUnread(l.dir, "api", letter.ID); err != nil {
		return err
	}
	return markRead(l.dir, "api", l.api.Epoch(), letter, true)
}

// report is api's report on tasks, as its turn end writes it.
func (l sentLab) report(tasks ...string) error {
	return Put(l.dir, Message{ID: NewID(), From: "api", FromEpoch: l.api.Epoch(), To: "web", ToEpoch: "sender-epoch", Kind: Finished, Text: "done", InReplyTo: tasks, CreatedAt: time.Now()})
}

// meanwhile runs act while the test holds api's mailbox lock, and lands
// between before it once act waits for that lock: what a turn end or a
// reader does in the time between the act's first look and its look under
// the lock.
func (l sentLab) meanwhile(t *testing.T, act func() error, between func() error) error {
	t.Helper()
	waiting := make(chan struct{})
	var once sync.Once
	state.LockWait = func(string) { once.Do(func() { close(waiting) }) }
	t.Cleanup(func() { state.LockWait = nil })
	done := make(chan error, 1)
	if err := withLock(l.dir, "api", func() error {
		go func() { done <- act() }()
		select {
		case <-waiting:
		case <-time.After(5 * time.Second):
			t.Error("the act never waited for the lock")
		}
		return between()
	}); err != nil {
		t.Fatal(err)
	}
	return <-done
}

func reasonOf(err error) Reason {
	var refusal *Refusal
	if errors.As(err, &refusal) {
		return refusal.Reason
	}
	return 0
}

func withdrawnOnDisk(t *testing.T, dir, id string) bool {
	t.Helper()
	status, _, err := ReadStatus(dir, "api", id)
	if err != nil {
		t.Fatal(err)
	}
	return status.Withdrawn
}

// A report that lands after send --to looked at the task is seen by the look
// under the lock: the addendum is refused, not sent to work already reported.
func TestAReportBeforeTheLockRefusesTheAddendum(t *testing.T) {
	lab := newSentLab(t)
	task := lab.send(t, Task, "rerun the smoke", "")
	if err := withLock(lab.dir, "api", func() error { return lab.readLocked(task) }); err != nil {
		t.Fatal(err)
	}
	if _, err := lab.box.TaskToAddTo(task, lab.api); err != nil {
		t.Fatalf("the first look refused: %v", err)
	}
	addendum := Message{ID: NewID(), From: "web", FromEpoch: "sender-epoch", To: "api", ToEpoch: lab.api.Epoch(), Kind: Task, Text: "also the lint", AddendumTo: task.ID, CreatedAt: time.Now()}
	err := lab.meanwhile(t, func() error { return lab.box.Send(context.Background(), addendum, lab.api) }, func() error { return lab.report(task.ID) })
	if reasonOf(err) != Reported {
		t.Fatalf("an addendum across a report answered %v", err)
	}
	if in, _ := IsIn(state.InboxPath(lab.dir, "api"), addendum.ID); in {
		t.Fatal("the refused addendum was written")
	}
}

// And so is an edit of an addendum: its replacement would add to a task
// reported on, so the addendum stays as it was.
func TestAReportBeforeTheLockRefusesTheAddendumsEdit(t *testing.T) {
	lab := newSentLab(t)
	task := lab.send(t, Task, "rerun the smoke", "")
	addendum := lab.send(t, Task, "also the lint", task.ID)
	if err := withLock(lab.dir, "api", func() error { return lab.readLocked(task) }); err != nil {
		t.Fatal(err)
	}
	replacement := addendum
	replacement.ID, replacement.Text = NewID(), "also the vet"
	edit := Edit{Named: addendum, Current: addendum, Target: lab.api, Replacement: replacement}
	err := lab.meanwhile(t, func() error {
		_, err := lab.box.Replace(context.Background(), edit)
		return err
	}, func() error { return lab.report(task.ID) })
	if reasonOf(err) != Reported || withdrawnOnDisk(t, lab.dir, addendum.ID) {
		t.Fatalf("an edit across a report answered %v, the addendum withdrawn %v", err, withdrawnOnDisk(t, lab.dir, addendum.ID))
	}
}

// A read that lands before the withdrawal's lock is final: the letter is
// refused as read, and nothing of it is withdrawn.
func TestAReadBeforeTheLockRefusesTheWithdrawal(t *testing.T) {
	lab := newSentLab(t)
	task := lab.send(t, Task, "rerun the smoke", "")
	err := lab.meanwhile(t, func() error {
		_, err := lab.box.WithdrawSent(context.Background(), task)
		return err
	}, func() error { return lab.readLocked(task) })
	if stepOf(err) != StepWithdraw || !errors.Is(err, ErrAlreadyRead) || withdrawnOnDisk(t, lab.dir, task.ID) {
		t.Fatalf("a withdrawal across a read answered %v", err)
	}
}

// Two edits of one letter at once: the second finds, under the lock, that the
// first has replaced it, and replaces that replacement instead of refusing
// with advice to send the letter again.
func TestAnEditBesideAnotherReplacesItsReplacement(t *testing.T) {
	lab := newSentLab(t)
	task := lab.send(t, Task, "rerun the smoke", "")
	addendum := lab.send(t, Task, "also the lint", task.ID)
	second, third := addendum, addendum
	second.ID, second.Text, second.Replaces = NewID(), "also the vet", addendum.ID
	third.ID, third.Text = NewID(), "also the tests"
	var edited Edited
	err := lab.meanwhile(t, func() error {
		var err error
		edited, err = lab.box.Replace(context.Background(), Edit{Named: addendum, Current: addendum, Target: lab.api, Replacement: third})
		return err
	}, func() error {
		_, err := Withdraw(lab.dir, addendum, &second)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Replaced.ID != second.ID || edited.Replacement.Replaces != second.ID || edited.Replacement.AddendumTo != task.ID || !withdrawnOnDisk(t, lab.dir, second.ID) {
		t.Fatalf("the edit beside another replaced %s with %+v", edited.Replaced.ID, edited.Replacement)
	}
}

// Withdrawn by the id an edit replaced, the letter that stands now goes, and
// with it every addendum: one unread is withdrawn, one read stays owed.
func TestAWithdrawalTakesWhatStandsNowWithItsAddenda(t *testing.T) {
	lab := newSentLab(t)
	task := lab.send(t, Task, "rerun the smoke", "")
	unseen := lab.send(t, Task, "also the lint", task.ID)
	read := lab.send(t, Task, "and the vet", task.ID)
	if err := withLock(lab.dir, "api", func() error { return lab.readLocked(read) }); err != nil {
		t.Fatal(err)
	}
	replacement := task
	replacement.ID, replacement.Text, replacement.Replaces = NewID(), "rerun the smoke on staging", task.ID
	if _, err := lab.box.Replace(t.Context(), Edit{Named: task, Current: task, Target: lab.api, Replacement: replacement}); err != nil {
		t.Fatal(err)
	}
	done, err := lab.box.WithdrawSent(t.Context(), task)
	if err != nil || done.Letter.ID != replacement.ID || done.Result != WithdrawnUnseen {
		t.Fatalf("withdrawing by the old id: %+v, %v", done, err)
	}
	results := map[string]error{}
	for _, addendum := range done.Addenda {
		results[addendum.Letter.ID] = addendum.Err
	}
	if len(results) != 2 || results[unseen.ID] != nil || !errors.Is(results[read.ID], ErrAlreadyRead) {
		t.Fatalf("the addenda went %+v", done.Addenda)
	}
	if !withdrawnOnDisk(t, lab.dir, unseen.ID) || withdrawnOnDisk(t, lab.dir, read.ID) {
		t.Fatal("the unread addendum stayed, or the read one went")
	}
}

// A mailbox whose lock stays held is acted on not at all, and the failure
// says the lock stopped it.
func TestABusyMailboxActsOnNothing(t *testing.T) {
	lab := newSentLab(t)
	task := lab.send(t, Task, "rerun the smoke", "")
	short, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := withLock(lab.dir, "api", func() error {
		_, err := lab.box.WithdrawSent(short, task)
		return err
	})
	if stepOf(err) != StepLock || !errors.Is(err, state.ErrMailboxBusy) || withdrawnOnDisk(t, lab.dir, task.ID) {
		t.Fatalf("a held lock answered %v", err)
	}
}

// Nothing is added where no report can come, each refusal naming its reason.
func TestAnAdditionIsRefusedWhereNoReportCanCome(t *testing.T) {
	lab := newSentLab(t)
	other := lab.api
	other.Name = "db"
	note := lab.send(t, Note, "the build is red", "")
	withdrawn := lab.send(t, Task, "withdrawn", "")
	if _, err := lab.box.WithdrawSent(t.Context(), withdrawn); err != nil {
		t.Fatal(err)
	}
	withdrawn = must(lab.box.SentMatching(withdrawn.ID))[0]
	failed := lab.send(t, Task, "never delivered", "")
	if err := WriteStatus(lab.dir, "api", failed.ID, Result{State: Failed, Detail: "refused"}); err != nil {
		t.Fatal(err)
	}
	earlier := lab.send(t, Task, "for the run that ended", "")
	reported := lab.send(t, Task, "reported", "")
	if err := lab.report(reported.ID); err != nil {
		t.Fatal(err)
	}
	ended := lab.api
	ended.ServiceStart++
	for _, c := range []struct {
		letter Message
		target registry.Session
		want   Reason
	}{
		{reported, other, OtherRecipient},
		{withdrawn, lab.api, TaskWithdrawn},
		{note, lab.api, OwesNothing},
		{reported, lab.api, Reported},
		{earlier, ended, RecipientGone},
		{failed, lab.api, NotDelivered},
	} {
		if _, err := lab.box.TaskToAddTo(c.letter, c.target); reasonOf(err) != c.want {
			t.Errorf("adding to %q answered %v, want reason %d", c.letter.Text, err, c.want)
		}
	}
}

// An id an edit replaced leads to the replacement; once that is no longer
// kept, the act is refused rather than taken on the tombstone.
func TestALostReplacementIsRefused(t *testing.T) {
	lab := newSentLab(t)
	task := lab.send(t, Task, "rerun the smoke", "")
	replacement := task
	replacement.ID, replacement.Replaces = NewID(), task.ID
	if _, err := lab.box.Replace(t.Context(), Edit{Named: task, Current: task, Target: lab.api, Replacement: replacement}); err != nil {
		t.Fatal(err)
	}
	if current, err := lab.box.CurrentSent(task); err != nil || current.ID != replacement.ID {
		t.Fatalf("the old id led to %s, %v", current.ID, err)
	}
	if err := state.Remove(filepath.Join(state.InboxPath(lab.dir, "api"), replacement.ID+".json")); err != nil {
		t.Fatal(err)
	}
	_, err := lab.box.CurrentSent(task)
	var refusal *Refusal
	if !errors.As(err, &refusal) || refusal.Reason != ReplacementLost || refusal.Leads != replacement.ID {
		t.Fatalf("a lost replacement answered %v", err)
	}
	if _, err := lab.box.WithdrawSent(t.Context(), task); reasonOf(err) != ReplacementLost {
		t.Fatalf("withdrawing by the old id answered %v", err)
	}
}
