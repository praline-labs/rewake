package inbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// A sweep beside a stop removes no path an open occurrence names: each of
// the mailbox's sweeps has a record it would remove, and with an occurrence
// naming it the record stays, as it does when the occurrence's resolution
// does not read. With the stop's records unreadable nothing is removed; with
// no stop, or with each occurrence resolved, each goes, which is what makes
// the first half mean anything.
func TestASweepBesideAStopRemovesNoNamedPath(t *testing.T) {
	for _, stop := range []string{"naming each", "resolution unreadable", "resolved", "unreadable", "none"} {
		t.Run(stop, func(t *testing.T) {
			dir := stateDir(t)
			run := liveRunOf(t, dir, "api")
			inbox := state.InboxPath(dir, "api")
			records := map[string]string{
				"the age sweep":     filepath.Join(inbox, NewID()+".status"),
				"claims":            filepath.Join(claimsPath(dir, "api"), NewID()),
				"publication marks": filepath.Join(inbox, "once", "ended-run", NewID()),
				"pending marks":     filepath.Join(pendingDir(dir, "api"), "marks", "ended-run", "mark"),
				"done journals":     filepath.Join(JournalPath(dir, "api"), "end"+doneSuffix),
				"waits":             filepath.Join(state.AwaitingPath(dir, "api"), "ended-run", "web"),
				"receipts":          filepath.Join(inbox, "receipts", "ended-run"),
			}
			old := time.Now().Add(-2 * keepFinished)
			for _, path := range records {
				content := "{}"
				if filepath.Base(path) == "end"+doneSuffix {
					content = `{"epoch":"ended-run","op":"end"}`
				}
				if filepath.Base(filepath.Dir(path)) == "receipts" {
					// An ended run's receipts, all of them gone.
					if err := os.MkdirAll(path, 0o700); err != nil {
						t.Fatal(err)
					}
				} else {
					writeRaw(t, path, content)
				}
				if err := os.Chtimes(path, old, old); err != nil {
					t.Fatal(err)
				}
			}
			switch stop {
			case "naming each", "resolution unreadable", "resolved":
				for what, path := range records {
					rel, err := filepath.Rel(dir, path)
					if err != nil {
						t.Fatal(err)
					}
					occurrence, err := live(dir).recordOccurrence("api", stopCause{Kind: causeUnreadable, Paths: []string{rel}, Cause: what})
					if err != nil {
						t.Fatal(err)
					}
					switch stop {
					case "resolution unreadable":
						writeRaw(t, occurrence.path(dir, "api")+resolvedSuffix, "{")
					case "resolved":
						if err := live(dir).resolveStop("api", occurrence, []string{"read"}); err != nil {
							t.Fatal(err)
						}
					}
				}
			case "unreadable":
				occurrence, err := live(dir).recordOccurrence("api", stopCause{Kind: causeUndecided, Cause: "names no path"})
				if err != nil {
					t.Fatal(err)
				}
				closeToReading(t, occurrence.path(dir, "api"))
			}
			(&Server{Dir: dir, Name: "api", Epoch: run}).sweepFinished()
			sweepAwaiting(dir, "api", run)
			for what, path := range records {
				_, err := os.Stat(path)
				if gone := os.IsNotExist(err); gone != (stop == "none" || stop == "resolved") {
					t.Errorf("%s: %s gone %v with a stop %s", what, path, gone, stop)
				}
			}
		})
	}
}
