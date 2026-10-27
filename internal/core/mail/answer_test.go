package mail

import (
	"context"
	"testing"
	"time"
)

// A send that gives up takes an answer that got in just then: that answer was
// not announced, and nothing else would show it.
func TestAnAnswerAtTheLastMomentIsTaken(t *testing.T) {
	dir := stateDir(t)
	report := message("port 8088")
	report.Kind, report.InReplyTo, report.ToEpoch = Finished, []string{"q1"}, "5.5"
	previous := beforeGivingUp
	beforeGivingUp = func() { unread(t, dir, report) }
	t.Cleanup(func() { beforeGivingUp = previous })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	got, found, err := awaitAnswer(ctx, dir, "api", "5.5", "q1", func(Message) error { return nil })
	if err != nil || !found || got.ID != report.ID {
		t.Errorf("AwaitAnswer = %v, %v, %v; want the answer taken", got.ID, found, err)
	}
}
