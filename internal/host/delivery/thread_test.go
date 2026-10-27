package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func TestDeliveryThreadIsRecordedBeforeAFastRead(t *testing.T) {
	dir := stateDir(t)
	task := message("work")
	task.FromEpoch = "2.2"
	task.ToEpoch = "1.1"
	if err := mail.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "1.1", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Thread: func() (string, error) { return "original", nil }}
	server.Deliver = func(_ context.Context, delivered mail.Message) mail.Result {
		if delivered.DeliveryThread != "original" || must(mailtest.DeliveryThread(dir, "api", task.ID)) != "original" {
			t.Error("delivery context was not durable before announcement")
		}
		err := state.WithMailboxLock(context.Background(), dir, "api", func() error {
			messages, err := mail.Open(dir, "api", "1.1").PeekUnread()
			if err != nil {
				return err
			}
			if len(messages) != 1 {
				t.Fatalf("readable messages=%v", messages)
			}
			if err := mail.Open(dir, "api", "1.1").MarkRead(messages[0], true); err != nil {
				return err
			}
			if !mail.ReportThreadChanged(dir, "api", []string{task.ID}, "new") {
				t.Error("fast report lost the original thread")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return mail.Result{State: mail.Delivered}
	}
	server.drain(context.Background())
	status, _, _ := mail.ReadStatus(dir, "api", task.ID)
	if status.State != mail.Read || must(mailtest.DeliveryThread(dir, "api", task.ID)) != "original" {
		t.Fatalf("read discarded delivery context: %+v", status)
	}
}

func TestAnUnknownDeliveryThreadWaitsBeforeReadability(t *testing.T) {
	dir := stateDir(t)
	task := message("work")
	task.FromEpoch = "2.2"
	if err := mail.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Thread: func() (string, error) { return "", errors.New("thread not ready") }, Deliver: func(context.Context, mail.Message) mail.Result {
		t.Fatal("delivered without context")
		return mail.Result{}
	}}
	server.drain(context.Background())
	unread, _ := mail.Open(dir, "api", "").PeekUnread()
	if len(unread) != 0 {
		t.Fatal("task became readable before thread identity")
	}
}

func TestUnavailableThreadFailsWithoutBecomingReadable(t *testing.T) {
	dir := stateDir(t)
	task := message("work")
	task.FromEpoch = "2.2"
	task.ToEpoch = "1.1"
	if err := mail.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "1.1", attempts: map[string]time.Time{}, outcomes: map[string]mail.Result{}, Thread: func() (string, error) { return "", mail.ErrThreadUnavailable }, Deliver: func(context.Context, mail.Message) mail.Result {
		t.Fatal("unknown thread was delivered")
		return mail.Result{}
	}}
	server.drain(context.Background())
	status, _, _ := mail.ReadStatus(dir, "api", task.ID)
	if status.State != mail.Failed {
		t.Fatalf("unavailable conversation left status %s", status.State)
	}
	if unread, _ := mail.Open(dir, "api", "1.1").PeekUnread(); len(unread) != 0 {
		t.Fatal("task was readable without a delivery thread")
	}
}
