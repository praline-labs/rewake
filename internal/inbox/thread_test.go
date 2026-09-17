package inbox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
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
		if delivered.DeliveryThread != "original" || deliveryThread(dir, "api", task.ID) != "original" {
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
	status, _ := ReadStatus(dir, "api", task.ID)
	if status.State != Read || deliveryThread(dir, "api", task.ID) != "original" {
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

func TestAWaitingReportKeepsItsDeliveryThread(t *testing.T) {
	dir := stateDir(t)
	task := message("work")
	task.FromEpoch = "2.2"
	task.ToEpoch = "1.1"
	unread(t, dir, task)
	if err := recordDeliveryThread(dir, "api", task.ID, "old"); err != nil {
		t.Fatal(err)
	}
	if err := MarkRead(dir, "api", "1.1", task, true); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(threadPath(dir, "api"), task.ID)
	old := time.Now().Add(-2 * keepFinished)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	server := &Server{Dir: dir, Name: "api", Epoch: "1.1"}
	server.sweepFinished()
	if !ReportThreadChanged(dir, "api", []string{task.ID}, "new") {
		t.Fatal("sweep discarded an unsettled wait's thread")
	}
	for _, waiter := range Waiters(dir, "api", "1.1") {
		ClearAwaiting(dir, "api", "1.1", waiter)
	}
	server.sweepFinished()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("settled thread record was not swept: %v", err)
	}
}

func TestNoticeContextUsesTheLatestAvailableLetter(t *testing.T) {
	dir := stateDir(t)
	old := message("old")
	latest := message("latest")
	latest.Kind = Error
	unread(t, dir, old, latest)
	shown := noticeContext(dir, "api", "", old)
	if shown.Unread != 2 || shown.Latest == nil || shown.Latest.Text != "latest" || shown.Text != "old" {
		t.Fatalf("context=%+v", shown)
	}
	release := reserve(t, dir, "q")
	defer release()
	reserved := message("reserved")
	reserved.Kind = Finished
	reserved.InReplyTo = []string{"q"}
	unread(t, dir, reserved)
	shown = noticeContext(dir, "api", "", old)
	if shown.Unread != 2 || shown.Latest.Text != "latest" {
		t.Fatalf("reserved answer entered preview: %+v", shown)
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
	status, _ := ReadStatus(dir, "api", task.ID)
	if status.State != Failed {
		t.Fatalf("unavailable conversation left status %s", status.State)
	}
	if unread, _ := PeekUnread(dir, "api", "1.1"); len(unread) != 0 {
		t.Fatal("task was readable without a delivery thread")
	}
}
