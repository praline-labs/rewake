package inbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// A question's answer is a read of the asker's mailbox: a stopped one does
// not take it, and it stays unread (8-stop). Removing the journal that
// stopped it is no evidence: the answer stays unread and the stop holds. The
// same journal with valid bytes, closed to reading and opened again, is, and
// the answer is taken.
func TestAStoppedMailboxDoesNotTakeAnAnswer(t *testing.T) {
	for _, bytes := range []string{"invalid, removed", "valid, closed and reopened"} {
		t.Run(bytes, func(t *testing.T) {
			dir := stateDir(t)
			report := message("port 8088")
			report.Kind, report.InReplyTo, report.ToEpoch = Finished, []string{"q1"}, "5.5"
			unread(t, dir, report)
			if err := state.EnsureSubdir(JournalPath(dir, "api")); err != nil {
				t.Fatal(err)
			}
			broken := filepath.Join(JournalPath(dir, "api"), "broken")
			valid := bytes != "invalid, removed"
			if valid {
				if err := WriteJournal(dir, "api", "broken", TurnJournal{Epoch: "5.5", Op: "end"}); err != nil {
					t.Fatal(err)
				}
				closeToReading(t, broken)
			} else if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
			await := func() (Message, bool, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
				defer cancel()
				return AwaitAnswer(ctx, dir, "api", "5.5", "q1", func(Message) error { return nil })
			}
			if _, found, err := await(); err == nil || found {
				t.Fatalf("a stopped mailbox took an answer: found %v, %v", found, err)
			}
			left := filepath.Join(state.UnreadPath(dir, "api"), report.ID+".json")
			if _, err := os.Stat(left); err != nil {
				t.Fatalf("the answer is not left unread: %v", err)
			}
			if valid {
				readable(t, broken)
				if got, found, err := await(); err != nil || !found || got.ID != report.ID {
					t.Fatalf("once readable: %v %v %v", got.ID, found, err)
				}
				return
			}
			if err := os.Remove(broken); err != nil {
				t.Fatal(err)
			}
			if _, found, err := await(); !removedWhileStopped(err, broken) || found {
				t.Fatalf("with the journal removed: found %v, %v", found, err)
			}
			if _, err := os.Stat(left); err != nil {
				t.Fatalf("the answer is not left unread: %v", err)
			}
		})
	}
}
