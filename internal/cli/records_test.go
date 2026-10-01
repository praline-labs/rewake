package cli

import (
	"os"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// checkRecordKinds fails a test whose calls left a file in a mailbox of no
// kind the mailbox's list names (inbox/records.go): the barrier would stop on
// it as unknown. The cli's own writers — receipts, reads in parts, pending
// marks — land in the mailbox too.
func checkRecordKinds(t *testing.T, dir string) {
	t.Helper()
	t.Cleanup(func() {
		entries, err := os.ReadDir(state.InboxesPath(dir))
		if err != nil {
			return
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			unlisted, err := inbox.UnlistedRecords(dir, entry.Name())
			if err != nil {
				continue
			}
			for _, path := range unlisted {
				t.Errorf("a writer left %s in the mailbox of %s, which is no kind of record on the list", path, entry.Name())
			}
		}
	})
}
