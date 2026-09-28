package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

func TestAnAbandonedAnswerIsAnnounced(t *testing.T) {
	for _, release := range []string{"removed", "expired"} {
		t.Run(release, func(t *testing.T) {
			dir := stateDir(t)
			marks := state.AnsweringPath(dir, "api")
			if err := os.MkdirAll(marks, 0o700); err != nil {
				t.Fatal(err)
			}
			mark := filepath.Join(marks, "question")
			if err := os.WriteFile(mark, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			report := message("recover me")
			report.Kind = Finished
			report.InReplyTo = []string{"question"}
			if err := Put(dir, report); err != nil {
				t.Fatal(err)
			}
			notices := 0
			server := &Server{Dir: dir, Name: "api", attempts: map[string]time.Time{}, outcomes: map[string]Result{}, Deliver: func(context.Context, Message) Result { notices++; return Result{State: Delivered} }}
			server.drain(context.Background())
			if notices != 0 {
				t.Fatal("reserved answer was announced")
			}
			if release == "removed" {
				if err := os.Remove(mark); err != nil {
					t.Fatal(err)
				}
			} else {
				old := time.Now().Add(-2 * answeringFresh)
				if err := os.Chtimes(mark, old, old); err != nil {
					t.Fatal(err)
				}
			}
			server.drain(context.Background())
			if notices != 1 {
				t.Errorf("abandoned answer got %d notices, want one", notices)
			}
		})
	}
}
