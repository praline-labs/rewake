package delivery

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/boottime"
)

// A task announced in a finished turn and left unread is reminded once, by a
// notice of its own through the same Deliver, and its status is not touched; a
// note announced with it is not, and the end of the reminded turn sends
// nothing. A task read before its turn ended is no part of it.
func TestAFinishedTurnGetsOneReminderOfItsUnreadTask(t *testing.T) {
	dir := stateDir(t)
	s := batchServer(dir)
	var notices []mail.Message
	s.Deliver = func(_ context.Context, m mail.Message) mail.Result {
		notices = append(notices, m)
		return mail.Result{State: mail.Delivered, Via: "socket"}
	}
	var task mail.Message
	for _, kind := range []mail.Kind{mail.Task, mail.Note} {
		m := message(string(kind))
		m.Kind, m.ToEpoch = kind, s.Epoch
		if err := mail.Put(dir, m); err != nil {
			t.Fatal(err)
		}
		if kind == mail.Task {
			task = m
		}
	}
	s.drain(context.Background())
	if len(notices) != 1 {
		t.Fatalf("%d notices, want the group's one", len(notices))
	}

	s.remind(context.Background(), boottime.Now())
	if len(notices) != 2 || !notices[1].Reminder || notices[1].ID == task.ID || notices[1].Unread != 1 || len(notices[1].Batch) != 1 || notices[1].Batch[0].ID != task.ID {
		t.Fatalf("reminder %+v, want one notice of the task alone", notices[len(notices)-1])
	}
	if mail.Notice(notices[1]) == mail.Notice(task) {
		t.Fatal("the reminder reads as the task's own notice")
	}
	if status, _, _ := mail.ReadStatus(dir, s.Name, task.ID); status.State != mail.Delivered {
		t.Fatalf("the task's status became %q", status.State)
	}

	s.remind(context.Background(), boottime.Now())
	if len(notices) != 2 {
		t.Fatal("the reminded turn's end reminded again")
	}

	read := message("read in time")
	read.Kind, read.ToEpoch = mail.Task, s.Epoch
	if err := mail.Put(dir, read); err != nil {
		t.Fatal(err)
	}
	s.drain(context.Background())
	readAll(t, dir, s.Epoch)
	s.remind(context.Background(), boottime.Now())
	if len(notices) != 3 {
		t.Fatalf("%d notices, want no reminder of a task read in its turn", len(notices))
	}
}
