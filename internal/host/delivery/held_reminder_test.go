package delivery

import (
	"context"
	"testing"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/boottime"
)

// A held task the harness releases to the agent belongs to the turn it was
// released in, however late the server hears of the release: the release
// reaches the agent, the turn ends with the task unread, and only then is the
// receipt handled — before the server looks at that end, or after it. Either
// way the end reminds of the task once.
func TestAReleasedTaskIsRemindedByTheTurnItWasReleasedIn(t *testing.T) {
	for _, order := range []string{"receipt before the look", "look before the receipt"} {
		t.Run(order, func(t *testing.T) {
			dir := stateDir(t)
			s := batchServer(dir)
			var notices []mail.Message
			s.Deliver = func(_ context.Context, m mail.Message) mail.Result {
				notices = append(notices, m)
				if len(notices) == 1 {
					return mail.Result{State: mail.Held, Via: "socket", Detail: "the session holds the notice"}
				}
				return mail.Result{State: mail.Delivered, Via: "socket"}
			}
			task := message("rerun the smoke")
			task.Kind, task.ToEpoch = mail.Task, s.Epoch
			if err := mail.Put(dir, task); err != nil {
				t.Fatal(err)
			}
			s.drain(context.Background())
			if len(notices) != 1 || s.outcomes[task.ID].State != mail.Held {
				t.Fatalf("%d notices, outcome %+v; want the task held", len(notices), s.outcomes[task.ID])
			}

			released := later(0)
			ended := later(released)
			receipt := mail.Receipt{ID: notices[0].ID, Result: mail.Result{State: mail.Delivered, Via: "socket", Detail: "released after being held"}, At: released}
			if order == "receipt before the look" {
				s.receive(context.Background(), receipt)
				s.remind(context.Background(), ended)
			} else {
				s.remind(context.Background(), ended)
				s.receive(context.Background(), receipt)
			}
			if len(notices) != 2 || !notices[1].Reminder || len(notices[1].Batch) != 1 || notices[1].Batch[0].ID != task.ID {
				t.Fatalf("notices %+v, want the held one and a reminder of the task", notices)
			}

			s.remind(context.Background(), later(ended))
			if len(notices) != 2 {
				t.Fatal("the next turn's end reminded of the task again")
			}
		})
	}
}

// later is a boot-clock reading after since.
func later(since int64) int64 {
	for {
		if now := boottime.Now(); now > since {
			return now
		}
	}
}
