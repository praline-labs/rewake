package mail

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// While a send waits for its answer, the answer goes to that send and the agent
// is not told a second time.
func TestAnAnswerAwaitedBySendIsNotAnnounced(t *testing.T) {
	for name, age := range map[string]time.Duration{"fresh mark": 0, "stale mark": mailtest.StaleAnswering} {
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
