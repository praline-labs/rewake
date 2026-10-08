package inbox

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/state"
)

// The latest reading wins whatever order the writers came in, an older one is
// dropped, none reads as zero, and the readings are records the mailbox
// knows.
func TestTheLatestTurnStartIsTheLargestReading(t *testing.T) {
	dir := stateDir(t)
	if got, err := LatestTurnStart(dir, "api", "e1"); got != 0 || err != nil {
		t.Fatalf("none recorded: %d %v", got, err)
	}
	for _, at := range []int64{200, 150, 300, 300} {
		if err := RecordTurnStart(dir, "api", "e1", at); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := LatestTurnStart(dir, "api", "e1"); got != 300 || err != nil {
		t.Fatalf("latest: %d %v", got, err)
	}
	entries, _ := os.ReadDir(filepath.Join(state.AwaitingPath(dir, "api"), "e1", turnStartsDir))
	if len(entries) != 1 {
		t.Fatalf("%d readings kept", len(entries))
	}
	if other, _ := LatestTurnStart(dir, "api", "e2"); other != 0 {
		t.Fatalf("another run's start: %d", other)
	}
	if err := MailboxStopped(dir, "api"); err != nil {
		t.Fatalf("the readings stopped the mailbox: %v", err)
	}
	if err := RecordTurnStart(dir, "api", "e1", 0); err == nil {
		t.Fatal("a start without a time was recorded")
	}
}

// Readings that cannot be read are an error, not zero: one of them may prove
// a mark's turn ended.
func TestAnUnreadableTurnStartIsAnError(t *testing.T) {
	dir := stateDir(t)
	if err := RecordTurnStart(dir, "api", "e1", 100); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(state.AwaitingPath(dir, "api"), "e1", turnStartsDir)
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o700) })
	if _, err := LatestTurnStart(dir, "api", "e1"); err == nil {
		t.Fatal("unreadable readings read as none")
	}
}
