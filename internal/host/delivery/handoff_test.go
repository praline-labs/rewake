package delivery

import (
	"context"
	"testing"
	"time"

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
			if !s.handoff(context.Background(), reply) {
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

// A finished end the wrapper published before it asked for the handoff is
// looked at by this image, in its real loop, whichever of the two the loop
// takes first: the next image's channel starts empty, and the task its turn
// left unread is reminded of once, not lost with the end (review 89).
func TestAFinishedEndQueuedBeforeTheHandoffIsLookedAt(t *testing.T) {
	// Both are ready when the loop selects, which it does at random.
	for trial := range 16 {
		dir := stateDir(t)
		var notices []mail.Message
		deliver := func(_ context.Context, m mail.Message) mail.Result {
			notices = append(notices, m)
			return mail.Result{State: mail.Delivered, Via: "socket"}
		}
		first := batchServer(dir)
		first.Deliver = deliver
		task := message("left unread at a finished turn")
		task.Kind, task.ToEpoch = mail.Task, first.Epoch
		if err := mail.Put(dir, task); err != nil {
			t.Fatal(err)
		}
		if !first.begin(context.Background()) {
			t.Fatal("the server did not begin")
		}
		first.drain(context.Background())

		s := batchServer(dir)
		s.Deliver, s.Remembered = deliver, &Memory{Announced: first.announced}
		end := later(0)
		ends := make(chan int64, 1)
		ends <- end
		handoff := make(chan chan<- Stop, 1)
		reply := make(chan Stop, 1)
		handoff <- reply
		s.Ends, s.Handoff = ends, handoff
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { s.Serve(ctx); close(done) }()
		var stop Stop
		select {
		case stop = <-reply:
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("the handoff was never answered")
		}
		cancel()
		<-done
		if stop.Why != "" {
			t.Fatalf("trial %d: the handoff was refused: %s", trial, stop.Why)
		}

		adopter := batchServer(dir)
		adopter.Deliver, adopter.Remembered = deliver, &stop.Memory
		if !adopter.begin(context.Background()) {
			t.Fatal("the adopting server did not begin")
		}
		adopter.drain(context.Background())
		reminded := reminders(notices, task.ID)
		// The next finished end is not this turn's, and reminds of nothing new.
		adopter.remind(context.Background(), later(end))
		if len(ends) != 0 || reminded != 1 || reminders(notices, task.ID) != 1 {
			t.Fatalf("trial %d: %d ends left behind, the handoff looked at %d, the task reminded of %d times before the next end and %d after; want the end looked at and one reminder",
				trial, len(ends), stop.Memory.Looked, reminded, reminders(notices, task.ID))
		}
	}
}

// reminders counts the reminders among notices that name id.
func reminders(notices []mail.Message, id string) int {
	n := 0
	for _, notice := range notices {
		if notice.Reminder && len(notice.Batch) == 1 && notice.Batch[0].ID == id {
			n++
		}
	}
	return n
}
