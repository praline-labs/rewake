package inbox

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// withdrawNow withdraws under the recipient's mailbox lock, as the command does.
func withdrawNow(t *testing.T, dir string, message Message, replacement *Message) (Withdrawal, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var result Withdrawal
	err := state.WithMailboxLock(ctx, dir, message.To, func() error {
		var err error
		result, err = Withdraw(dir, message, replacement)
		return err
	})
	return result, err
}

// tombstone is the copy the reader finds in unread/, if any.
func tombstone(t *testing.T, dir string, id string) *Message {
	t.Helper()
	messages, err := listIn(state.UnreadPath(dir, "api"))
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
	status, _, _ := ReadStatus(f.dir, "api", f.task.ID)
	if status.State != Failed || !status.Withdrawn || status.Detail != "withdrawn by web" {
		t.Fatalf("a withdrawn task became %+v", status)
	}
	if stone := tombstone(t, f.dir, f.task.ID); stone == nil || stone.Withdrawn == nil || KindOf(*stone) != Note {
		t.Fatalf("the tombstone is gone or wrong: %+v", stone)
	}
	if f.undelivered(t) != nil {
		t.Fatal("the sender of a withdrawn task was told it was not delivered")
	}
	if must(isIn(state.InboxPath(f.dir, "api"), f.task.ID)) {
		t.Fatal("the waiting copy of a withdrawn task stayed to be delivered again")
	}
}

// A hold that ends after the withdrawal — released, expired, or with the
// session — changes nothing: withdrawn is final like read.
func TestAHoldEndingAfterAWithdrawalLeavesItWithdrawn(t *testing.T) {
	for _, end := range []string{"released", "expired", "session ended"} {
		t.Run(end, func(t *testing.T) {
			f := startHeld(t, Task, false)
			id := f.announced(t)
			f.until(t, Held)
			if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != WithdrawnHeld {
				t.Fatalf("withdraw: %v %v", result, err)
			}
			switch end {
			case "released":
				f.receipts <- Receipt{ID: id, Result: Result{State: Delivered, Via: "socket"}}
			case "expired":
				f.receipts <- Receipt{ID: id, Result: Result{State: Failed, Via: "socket", Detail: "expired unreleased"}}
			default:
				f.stop()
			}
			f.stillWithdrawn(t)
		})
	}
}

// A late word about a delivered notice cannot undo a withdrawal either.
func TestALateWordAfterAWithdrawalLeavesItWithdrawn(t *testing.T) {
	for _, word := range []State{Held, Failed} {
		t.Run(string(word), func(t *testing.T) {
			f := startDelivering(t, Task)
			id := f.announced(t)
			f.until(t, Delivered)
			if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != WithdrawnAnnounced {
				t.Fatalf("withdraw: %v %v", result, err)
			}
			f.receipts <- Receipt{ID: id, Result: Result{State: word, Via: "socket", Detail: "late"}}
			f.stillWithdrawn(t)
		})
	}
}

// Withdrawn while its notice is on its way: whatever the harness answers —
// delivered, or not yet — the status stays withdrawn and the message is not
// announced again.
func TestAWithdrawalDuringTheNoticeIsFinal(t *testing.T) {
	for _, answer := range []State{Delivered, Pending} {
		t.Run(string(answer), func(t *testing.T) {
			dir := stateDir(t)
			task := message("rerun the smoke")
			task.Kind, task.FromEpoch, task.ToEpoch = Task, "web-epoch", "api-epoch"
			if err := Put(dir, task); err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			server := &Server{Dir: dir, Name: "api", Epoch: "api-epoch", Deliver: func(context.Context, Message) Result {
				if calls.Add(1) == 1 {
					if _, err := withdrawNow(t, dir, task, nil); err != nil {
						t.Error(err)
					}
				}
				return Result{State: answer, Via: "socket", Detail: "the harness answered"}
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
	f := startHeld(t, Task, true)
	f.until(t, Pending)
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != WithdrawnUnseen {
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
	if status, _, _ := ReadStatus(f.dir, "api", f.task.ID); !status.Withdrawn || status.State != Failed {
		t.Fatalf("status %+v", status)
	}
	if !must(isIn(state.DonePath(f.dir, "api"), f.task.ID)) {
		t.Fatal("the withdrawn message was not kept for diagnosis")
	}
}

// Reading a tombstone owes nothing and keeps the withdrawal on record.
func TestReadingATombstoneKeepsItWithdrawn(t *testing.T) {
	f := startDelivering(t, Task)
	f.announced(t)
	f.until(t, Delivered)
	if _, err := withdrawNow(t, f.dir, f.task, nil); err != nil {
		t.Fatal(err)
	}
	read := readAll(t, f.dir, "api-epoch")
	if len(read) != 1 || read[0].Withdrawn == nil || !strings.Contains(read[0].Text, "web withdrew its task of") {
		t.Fatalf("read %+v", read)
	}
	if status, _, _ := ReadStatus(f.dir, "api", f.task.ID); !status.Withdrawn {
		t.Fatalf("reading the tombstone turned it into %+v", status)
	}
	if len(Waiters(f.dir, "api", "api-epoch")) != 0 {
		t.Fatal("a tombstone owed a report")
	}
	if result, err := withdrawNow(t, f.dir, f.task, nil); err != nil || result != AlreadyWithdrawn {
		t.Fatalf("second withdraw: %v %v", result, err)
	}
}

func TestWithdrawRefusesWhatIsFinal(t *testing.T) {
	f := startDelivering(t, Task)
	f.announced(t)
	f.until(t, Delivered)
	readAll(t, f.dir, "api-epoch")
	if _, err := withdrawNow(t, f.dir, f.task, nil); !errors.Is(err, ErrAlreadyRead) {
		t.Fatalf("a read task: %v", err)
	}

	dir := stateDir(t)
	failed := message("never delivered")
	failed.FromEpoch, failed.ToEpoch = "web-epoch", "api-epoch"
	if err := Put(dir, failed); err != nil {
		t.Fatal(err)
	}
	if err := writeStatus(dir, "api", failed.ID, Result{State: Failed, Detail: "the session ended"}); err != nil {
		t.Fatal(err)
	}
	settle(dir, "api", failed.ID, Failed)
	if _, err := withdrawNow(t, dir, failed, nil); !errors.Is(err, ErrNotDelivered) {
		t.Fatalf("a failed task: %v", err)
	}
	gone := message("swept")
	if _, err := withdrawNow(t, dir, gone, nil); !errors.Is(err, ErrNoLongerKept) {
		t.Fatalf("a swept task: %v", err)
	}
}

// An edit is a withdrawal and a new letter at once: the tombstone names the
// replacement, which waits to be announced with its own preview.
func TestAnEditReplacesUnderOneLock(t *testing.T) {
	f := startDelivering(t, Task)
	f.announced(t)
	f.until(t, Delivered)
	replacement := message("rerun the smoke on staging")
	replacement.Kind, replacement.FromEpoch, replacement.ToEpoch, replacement.Replaces = Task, "web-epoch", "api-epoch", f.task.ID
	if result, err := withdrawNow(t, f.dir, f.task, &replacement); err != nil || result != WithdrawnAnnounced {
		t.Fatalf("edit: %v %v", result, err)
	}
	if next := f.announced(t); next != replacement.ID {
		t.Fatalf("announced %s, want the replacement", next)
	}
	stone := tombstone(t, f.dir, f.task.ID)
	if stone == nil || stone.Withdrawn.ReplacedBy != replacement.ID || !strings.Contains(stone.Text, replacement.ID) {
		t.Fatalf("tombstone %+v", stone)
	}
	if status, _, _ := ReadStatus(f.dir, "api", f.task.ID); !strings.Contains(status.Detail, "replaced by "+replacement.ID) {
		t.Fatalf("status %+v", status)
	}
	another := message("again")
	another.To = "api"
	if _, err := withdrawNow(t, f.dir, f.task, &another); !errors.Is(err, ErrAlreadyWithdrawn) {
		t.Fatalf("editing a withdrawn message: %v", err)
	}
}

func TestSentMatchingTakesUniquePrefixesOfThisRunOnly(t *testing.T) {
	dir := stateDir(t)
	put := func(id string, from, epoch string, kind Kind) Message {
		m := Message{ID: id, From: from, FromEpoch: epoch, To: "api", ToEpoch: "api-epoch", Kind: kind, Text: "x", CreatedAt: time.Now()}
		if err := Put(dir, m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	put("1790000000000000001-aaaa11112222", "web", "web-epoch", Task)
	put("1790000000000000002-aaaa33334444", "web", "web-epoch", Note)
	put("1790000000000000003-bbbb55556666", "web", "old-epoch", Task)
	put("1790000000000000004-cccc77778888", "web", "web-epoch", Finished)
	cases := map[string]int{
		"1790000000000000001-aaaa11112222": 1,
		"aaaa1":                            1,
		"aaaa":                             2,
		"1790000000000000002":              1,
		"bbbb":                             0, // an earlier run's
		"cccc":                             0, // a report
		"179":                              0, // too short
	}
	for reference, want := range cases {
		if got := must(SentMatching(dir, "web", "web-epoch", reference)); len(got) != want {
			t.Errorf("%s: %d matches, want %d", reference, len(got), want)
		}
	}
}

// checkedReservation withdraws the message right before the native commit,
// as a sender racing the notice would, and commits only what still checks.
type checkedReservation struct {
	withdraw  func()
	announced *atomic.Int32
}

func (checkedReservation) Prepare(fn func(string) error) error { return fn("") }
func (checkedReservation) Deliver(context.Context, Message) Result {
	return Result{State: Delivered}
}
func (checkedReservation) Close() {}
func (r checkedReservation) DeliverChecked(_ context.Context, _ Message, valid func() bool) Result {
	r.withdraw()
	if !valid() {
		return Result{State: Pending, Detail: "changed"}
	}
	r.announced.Add(1)
	return Result{State: Delivered, Via: "codex"}
}

// The last check before the notice commits sees a withdrawal made after the
// message became readable: no notice goes out for it.
func TestTheLastCheckBeforeANoticeSeesAWithdrawal(t *testing.T) {
	dir := stateDir(t)
	task := message("rerun the smoke")
	task.Kind, task.FromEpoch, task.ToEpoch = Task, "web-epoch", "api-epoch"
	if err := Put(dir, task); err != nil {
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
		Deliver: func(context.Context, Message) Result { return Result{State: Delivered} },
		Reserve: func(context.Context, Message) (Reservation, error) { return reservation, nil },
	}
	serveUntil(t, server, once.Load)
	if announced.Load() != 0 {
		t.Fatal("a notice went out for a message withdrawn before its commit")
	}
	(&heldFixture{dir: dir, task: task}).stillWithdrawn(t)
}
