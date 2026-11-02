package delivery

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A task announced and not yet looked at by a finished turn's end does not
// keep the run from a switch: the handoff stops and hands the announcement
// over. The first finished end of the turn it belongs to, in the next image
// or in this one again after an exec that failed, reminds of the task once,
// and the next end not again.
func TestAnAnnouncedTaskGoesOverWithTheRunAndIsRemindedOnce(t *testing.T) {
	for _, next := range []string{"the next image", "this image after a failed exec"} {
		t.Run(next, func(t *testing.T) {
			dir := stateDir(t)
			s := batchServer(dir)
			var notices []mail.Message
			deliver := func(_ context.Context, m mail.Message) mail.Result {
				notices = append(notices, m)
				return mail.Result{State: mail.Delivered, Via: "socket"}
			}
			s.Deliver = deliver
			task := message("rerun the smoke")
			task.Kind, task.ToEpoch = mail.Task, s.Epoch
			if err := mail.Put(dir, task); err != nil {
				t.Fatal(err)
			}
			if !s.begin(context.Background()) {
				t.Fatal("the server did not begin")
			}
			s.drain(context.Background())
			if len(notices) != 1 {
				t.Fatalf("%d notices, want the task's", len(notices))
			}
			// No finished end reaches Ends before the switch.

			reply := make(chan Stop, 1)
			if !s.handoff(reply) {
				t.Fatalf("the handoff was refused: %s", (<-reply).Why)
			}
			stop := <-reply
			if _, ok := stop.Memory.Announced[task.ID]; !ok {
				t.Fatalf("the handoff carries %+v, without the task's announcement", stop.Memory)
			}

			adopter := s
			if next == "the next image" {
				adopter = batchServer(dir)
				adopter.Deliver, adopter.Remembered = deliver, &stop.Memory
			}
			if !adopter.begin(context.Background()) {
				t.Fatal("the adopting server did not begin")
			}
			adopter.remind(context.Background(), later(0))
			if len(notices) != 2 || !notices[1].Reminder || len(notices[1].Batch) != 1 || notices[1].Batch[0].ID != task.ID {
				t.Fatalf("notices %+v, want the task's and one reminder of it", notices)
			}
			adopter.remind(context.Background(), later(0))
			if len(notices) != 2 {
				t.Fatalf("%d notices: the next finished end reminded again", len(notices))
			}
		})
	}
}
