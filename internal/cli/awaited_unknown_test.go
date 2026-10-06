package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// rewake inbox --awaited only informs, so a recipient whose session record
// cannot be read is folded into the cautious answer: its run may still be
// live and report, so its task stays owed, never "no report coming", which
// would tell the sender to send it again.
func TestAwaitedTakesAnUnreadableRecipientForLive(t *testing.T) {
	for _, record := range []string{"unparseable", "closed to reading"} {
		t.Run(record, func(t *testing.T) {
			room := newAwaitedRoom(t)
			worker := otherRun(t, room.dir, "worker")
			task := room.sentAndRead(worker, "long job")
			path := state.SessionPath(room.dir, worker.Name)
			if record == "unparseable" {
				if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
			}
			if view := byID(awaitedJSON(t))[task]; view.State != inbox.StageOwed || view.Gone != "" {
				t.Fatalf("a task to a recipient whose record does not read: %+v", view)
			}
			if code, out, _ := run("inbox", "--awaited"); code != ExitOK || strings.Contains(out, "no report coming") {
				t.Fatalf("exit %d:\n%s", code, out)
			}
		})
	}
}
