package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// While a send waits for its answer, the answer goes to that send and the agent
// is not told a second time.
func TestAnAnswerAwaitedBySendIsNotAnnounced(t *testing.T) {
	for name, age := range map[string]time.Duration{"fresh mark": 0, "stale mark": 2 * answeringFresh} {
		t.Run(name, func(t *testing.T) {
			dir := stateDir(t)
			marks := state.AnsweringPath(dir, "api")
			if err := os.MkdirAll(marks, 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			mark := filepath.Join(marks, "q1")
			if err := os.WriteFile(mark, nil, 0o600); err != nil {
				t.Fatalf("mark: %v", err)
			}
			when := time.Now().Add(-age)
			_ = os.Chtimes(mark, when, when)

			report := message("port 8088")
			report.Kind, report.InReplyTo = Finished, []string{"q1"}
			if err := Put(dir, report); err != nil {
				t.Fatalf("Put: %v", err)
			}
			notices := 0
			server := &Server{Dir: dir, Name: "api", Deliver: func(context.Context, Message) Result {
				notices++
				return Result{State: Delivered, Via: "socket"}
			}}
			server.attempts, server.outcomes = map[string]time.Time{}, map[string]Result{}
			server.drain(context.Background())

			if want := map[bool]int{true: 0, false: 1}[age == 0]; notices != want {
				t.Errorf("notices = %d, want %d", notices, want)
			}
			if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), report.ID+".json")); err != nil {
				t.Errorf("the answer is not readable: %v", err)
			}
		})
	}
}

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
	got, found, err := AwaitAnswer(ctx, dir, "api", "5.5", "q1", func(Message) error { return nil })
	if err != nil || !found || got.ID != report.ID {
		t.Errorf("AwaitAnswer = %v, %v, %v; want the answer taken", got.ID, found, err)
	}
}
