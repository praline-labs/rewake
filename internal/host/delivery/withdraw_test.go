package delivery

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// tombstone is the copy the reader finds in unread/, if any.
func tombstone(t *testing.T, dir string, id string) *mail.Message {
	t.Helper()
	messages, err := mail.ListIn(state.UnreadPath(dir, "api"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range messages {
		if m.ID == id {
			return &m
		}
	}
	return nil
}

// stillWithdrawn checks what every late word must leave alone: the status,
// the tombstone the reader is to find, and the sender left untold.
func (f *heldFixture) stillWithdrawn(t *testing.T) {
	t.Helper()
	time.Sleep(150 * time.Millisecond)
	status, _, _ := mail.ReadStatus(f.dir, "api", f.task.ID)
	if status.State != mail.Failed || !status.Withdrawn || status.Detail != "withdrawn by web" {
		t.Fatalf("a withdrawn task became %+v", status)
	}
	if stone := tombstone(t, f.dir, f.task.ID); stone == nil || stone.Withdrawn == nil || mail.KindOf(*stone) != mail.Note {
		t.Fatalf("the tombstone is gone or wrong: %+v", stone)
	}
	if f.undelivered(t) != nil {
		t.Fatal("the sender of a withdrawn task was told it was not delivered")
	}
	if must(mail.IsIn(state.InboxPath(f.dir, "api"), f.task.ID)) {
		t.Fatal("the waiting copy of a withdrawn task stayed to be delivered again")
	}
}

// A hold that ends after the withdrawal — released, expired, or with the
// session — changes nothing: withdrawn is final like read.
func TestAHoldEndingAfterAWithdrawalLeavesItWithdrawn(t *testing.T) {
	for _, end := range []string{"released", "expired", "session ended"} {
		t.Run(end, func(t *testing.T) {
			f := startHeld(t, mail.Task, false)
			id := f.announced(t)
			f.until(t, mail.Held)
			if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != mail.WithdrawnHeld {
				t.Fatalf("withdraw: %v %v", result, err)
			}
			switch end {
			case "released":
				f.receipts <- mail.Receipt{ID: id, Result: mail.Result{State: mail.Delivered, Via: "socket"}}
			case "expired":
				f.receipts <- mail.Receipt{ID: id, Result: mail.Result{State: mail.Failed, Via: "socket", Detail: "expired unreleased"}}
			default:
				f.stop()
			}
			f.stillWithdrawn(t)
		})
	}
}

// A late word about a delivered notice cannot undo a withdrawal either.
func TestALateWordAfterAWithdrawalLeavesItWithdrawn(t *testing.T) {
	for _, word := range []mail.State{mail.Held, mail.Failed} {
		t.Run(string(word), func(t *testing.T) {
			f := startDelivering(t, mail.Task)
			id := f.announced(t)
			f.until(t, mail.Delivered)
			if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != mail.WithdrawnAnnounced {
				t.Fatalf("withdraw: %v %v", result, err)
			}
			f.receipts <- mail.Receipt{ID: id, Result: mail.Result{State: word, Via: "socket", Detail: "late"}}
			f.stillWithdrawn(t)
		})
	}
}

// Withdrawn while its notice is on its way: whatever the harness answers —
// delivered, or not yet — the status stays withdrawn and the message is not
// announced again.
func TestAWithdrawalDuringTheNoticeIsFinal(t *testing.T) {
	for _, answer := range []mail.State{mail.Delivered, mail.Pending} {
		t.Run(string(answer), func(t *testing.T) {
			dir := stateDir(t)
			task := message("rerun the smoke")
			task.Kind, task.FromEpoch, task.ToEpoch = mail.Task, "web-epoch", "api-epoch"
			if err := mail.Put(dir, task); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := &Server{Dir: dir, Name: "api", Epoch: "api-epoch", Deliver: func(context.Context, mail.Message) mail.Result {
				if calls.Add(1) == 1 {
					if _, err := withdrawNow(t, dir, task, nil); err != nil {
						t.Error(err)
					}
				}
				return mail.Result{State: answer, Via: "socket", Detail: "the harness answered"}
			}}
			f := &heldFixture{dir: dir, task: task}
			// Past one retry interval: a pending answer would be tried again.
			serveUntil(t, server, func() bool {
				return calls.Load() > 0 && time.Now().After(task.CreatedAt.Add(retryInterval+300*time.Millisecond))
			})
			if n := calls.Load(); n != 1 {
				t.Fatalf("the withdrawn task was announced %d times", n)
			}
			f.stillWithdrawn(t)
		})
	}
}

// Before its notice, a withdrawn message goes without a trace for the
// recipient: nothing readable, nothing announced once it could be.
func TestAWithdrawalBeforeTheNoticeLeavesNothingToRead(t *testing.T) {
	f := startHeld(t, mail.Task, true)
	f.until(t, mail.Pending)
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != mail.WithdrawnUnseen {
		t.Fatalf("withdraw: %v %v", result, err)
	}
	close(f.opened)
	time.Sleep(300 * time.Millisecond)
	select {
	case m := <-f.calls:
		t.Fatalf("a withdrawn task was announced: %+v", m)
	default:
	}
	if tombstone(t, f.dir, f.task.ID) != nil {
		t.Fatal("a message withdrawn before its notice became readable")
	}
	if status, _, _ := mail.ReadStatus(f.dir, "api", f.task.ID); !status.Withdrawn || status.State != mail.Failed {
		t.Fatalf("status %+v", status)
	}
	if !must(mail.IsIn(state.DonePath(f.dir, "api"), f.task.ID)) {
		t.Fatal("the withdrawn message was not kept for diagnosis")
	}
}

// Reading a tombstone owes nothing and keeps the withdrawal on record.
func TestReadingATombstoneKeepsItWithdrawn(t *testing.T) {
	f := startDelivering(t, mail.Task)
	f.announced(t)
	f.until(t, mail.Delivered)
	if _, err := withdrawNow(t, f.dir, f.task, nil); err != nil {
		t.Fatal(err)
	}
	read := readAll(t, f.dir, "api-epoch")
	if len(read) != 1 || read[0].Withdrawn == nil || !strings.Contains(read[0].Text, "web withdrew its task of") {
		t.Fatalf("read %+v", read)
	}
	if status, _, _ := mail.ReadStatus(f.dir, "api", f.task.ID); !status.Withdrawn {
		t.Fatalf("reading the tombstone turned it into %+v", status)
	}
	if len(mail.Open(f.dir, "api", "api-epoch").Waiters()) != 0 {
		t.Fatal("a tombstone owed a report")
	}
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != mail.AlreadyWithdrawn {
		t.Fatalf("second withdraw: %v %v", result, err)
	}
}

func TestWithdrawRefusesWhatIsFinal(t *testing.T) {
	f := startDelivering(t, mail.Task)
	f.announced(t)
	f.until(t, mail.Delivered)
	readAll(t, f.dir, "api-epoch")
	if _, err := withdrawNow(t, f.dir, f.task, nil); !errors.Is(err, mail.ErrAlreadyRead) {
		t.Fatalf("a read task: %v", err)
	}

	dir := stateDir(t)
	failed := message("never delivered")
	failed.FromEpoch, failed.ToEpoch = "web-epoch", "api-epoch"
	if err := mail.Put(dir, failed); err != nil {
		t.Fatal(err)
	}
	if err := mail.WriteStatus(dir, "api", failed.ID, mail.Result{State: mail.Failed, Detail: "the session ended"}); err != nil {
		t.Fatal(err)
	}
	_ = mail.Open(dir, "api", "").WithLock(t.Context(), func(l mail.Locked) error { l.Settle(failed.ID, mail.Failed); return nil })
	if _, err := withdrawNow(t, dir, failed, nil); !errors.Is(err, mail.ErrNotDelivered) {
		t.Fatalf("a failed task: %v", err)
	}
	gone := message("swept")
	if _, err := withdrawNow(t, dir, gone, nil); !errors.Is(err, mail.ErrNoLongerKept) {
		t.Fatalf("a swept task: %v", err)
	}
}

// An edit is a withdrawal and a new letter at once: the tombstone names the
// replacement, which waits to be announced with its own preview.
func TestAnEditReplacesUnderOneLock(t *testing.T) {
	f := startDelivering(t, mail.Task)
	f.announced(t)
	f.until(t, mail.Delivered)
	replacement := message("rerun the smoke on staging")
	replacement.Kind, replacement.FromEpoch, replacement.ToEpoch, replacement.Replaces = mail.Task, "web-epoch", "api-epoch", f.task.ID
	if result, err := withdrawNow(t, f.dir, f.task, &replacement); err != nil || result != mail.WithdrawnAnnounced {
		t.Fatalf("edit: %v %v", result, err)
	}
	if next := f.announced(t); next != replacement.ID {
		t.Fatalf("announced %s, want the replacement", next)
	}
	stone := tombstone(t, f.dir, f.task.ID)
	if stone == nil || stone.Withdrawn.ReplacedBy != replacement.ID || !strings.Contains(stone.Text, replacement.ID) {
		t.Fatalf("tombstone %+v", stone)
	}
	if status, _, _ := mail.ReadStatus(f.dir, "api", f.task.ID); !strings.Contains(status.Detail, "replaced by "+replacement.ID) {
		t.Fatalf("status %+v", status)
	}
	another := message("again")
	another.To = "api"
	if _, err := withdrawNow(t, f.dir, f.task, &another); !errors.Is(err, mail.ErrAlreadyWithdrawn) {
		t.Fatalf("editing a withdrawn message: %v", err)
	}
}

// checkedReservation withdraws the message right before the native commit,
// as a sender racing the notice would, and commits only what still checks.
type checkedReservation struct {
	withdraw  func()
	announced *atomic.Int32
}

func (checkedReservation) Prepare(fn func(string) error) error { return fn("") }

func (checkedReservation) Deliver(context.Context, mail.Message) mail.Result {
	return mail.Result{State: mail.Delivered}
}

func (checkedReservation) Close() {}

func (r checkedReservation) DeliverChecked(_ context.Context, _ mail.Message, valid func() bool) mail.Result {
	r.withdraw()
	if !valid() {
		return mail.Result{State: mail.Pending, Detail: "changed"}
	}
	r.announced.Add(1)
	return mail.Result{State: mail.Delivered, Via: "socket"}
}

// The last check before the notice commits sees a withdrawal made after the
// message became readable: no notice goes out for it.
func TestTheLastCheckBeforeANoticeSeesAWithdrawal(t *testing.T) {
	dir := stateDir(t)
	task := message("rerun the smoke")
	task.Kind, task.FromEpoch, task.ToEpoch = mail.Task, "web-epoch", "api-epoch"
	if err := mail.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	var announced atomic.Int32
	var once atomic.Bool
	reservation := checkedReservation{announced: &announced, withdraw: func() {
		if once.CompareAndSwap(false, true) {
			if _, err := withdrawNow(t, dir, task, nil); err != nil {
				t.Error(err)
			}
		}
	}}
	server := &Server{
		Dir: dir, Name: "api", Epoch: "api-epoch",
		Deliver: func(context.Context, mail.Message) mail.Result { return mail.Result{State: mail.Delivered} },
		Reserve: func(context.Context, mail.Message) (mail.Reservation, error) { return reservation, nil },
	}
	serveUntil(t, server, once.Load)
	if announced.Load() != 0 {
		t.Fatal("a notice went out for a message withdrawn before its commit")
	}
	(&heldFixture{dir: dir, task: task}).stillWithdrawn(t)
}
