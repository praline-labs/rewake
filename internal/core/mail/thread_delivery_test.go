package mail

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func TestDeliveryThreadIsRecordedBeforeAFastRead(t *testing.T) {
	dir := stateDir(t)
	task := message("work")
	task.FromEpoch = "2.2"
	task.ToEpoch = "1.1"
	if err := Put(dir, task); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "1.1", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Thread: func() (string, error) { return "original", nil }}
	server.Deliver = func(_ context.Context, delivered Message) Result {
		if delivered.DeliveryThread != "original" || must(mailtest.DeliveryThread(dir, "api", task.ID)) != "original" {
			t.Error("delivery context was not durable before announcement")
		}
		err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
			messages, err := PeekUnread(dir, "api", "1.1")
			if err != nil {
				return err
			}
			if len(messages) != 1 {
				t.Fatalf("readable messages=%v", messages)
			}
			if err := MarkRead(dir, "api", "1.1", messages[0], true); err != nil {
				return err
			}
			if !ReportThreadChanged(dir, "api", []string{task.ID}, "new") {
				t.Error("fast report lost the original thread")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return Result{State: Delivered}
	}
	server.drain(context.Background())
	status, _, _ := ReadStatus(dir, "api", task.ID)
	if status.State != Read || must(mailtest.DeliveryThread(dir, "api", task.ID)) != "original" {
		t.Fatalf("read discarded delivery context: %+v", status)
	}
}

func TestAnUnknownDeliveryThreadWaitsBeforeReadability(t *testing.T) {
	dir := stateDir(t)
	task := message("work")
	task.FromEpoch = "2.2"
	if err := Put(dir, task); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Thread: func() (string, error) { return "", errors.New("thread not ready") }, Deliver: func(context.Context, Message) Result { t.Fatal("delivered without context"); return Result{} }}
	server.drain(context.Background())
	unread, _ := PeekUnread(dir, "api", "")
	if len(unread) != 0 {
		t.Fatal("task became readable before thread identity")
	}
}

func TestUnavailableThreadFailsWithoutBecomingReadable(t *testing.T) {
	dir := stateDir(t)
	task := message("work")
	task.FromEpoch = "2.2"
	task.ToEpoch = "1.1"
	if err := Put(dir, task); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "1.1", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Thread: func() (string, error) { return "", ErrThreadUnavailable }, Deliver: func(context.Context, Message) Result { t.Fatal("unknown thread was delivered"); return Result{} }}
	server.drain(context.Background())
	status, _, _ := ReadStatus(dir, "api", task.ID)
	if status.State != Failed {
		t.Fatalf("unavailable conversation left status %s", status.State)
	}
	if unread, _ := PeekUnread(dir, "api", "1.1"); len(unread) != 0 {
		t.Fatal("task was readable without a delivery thread")
	}
}
