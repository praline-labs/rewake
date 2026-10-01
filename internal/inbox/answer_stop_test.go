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
// not take it, and it stays unread for once a look finds the stop's cause
// gone (8-stop).
func TestAStoppedMailboxDoesNotTakeAnAnswer(t *testing.T) {
	dir := stateDir(t)
	report := message("port 8088")
	report.Kind, report.InReplyTo, report.ToEpoch = Finished, []string{"q1"}, "5.5"
	unread(t, dir, report)
	if err := state.EnsureSubdir(JournalPath(dir, "api")); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(JournalPath(dir, "api"), "broken")
	if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, found, err := AwaitAnswer(ctx, dir, "api", "5.5", "q1", func(Message) error { return nil }); err == nil || found {
		t.Fatalf("a stopped mailbox took an answer: found %v, %v", found, err)
	}
	if _, err := os.Stat(filepath.Join(state.UnreadPath(dir, "api"), report.ID+".json")); err != nil {
		t.Fatalf("the answer is not left unread: %v", err)
	}
	if err := os.Remove(broken); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if got, found, err := AwaitAnswer(ctx, dir, "api", "5.5", "q1", func(Message) error { return nil }); err != nil || !found || got.ID != report.ID {
		t.Fatalf("once readable: %v %v %v", got.ID, found, err)
	}
}
