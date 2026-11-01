package mail

import (
	"errors"
	"os"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// maxRecordRead bounds a read of a record of a mailbox.
const maxRecordRead = 100 << 20

// readRecord reads a record of a mailbox as a regular file and nothing else
// (state.ReadRegular), since a mailbox holds no other kind; a directory of
// records is listed only as itself (state.ReadPlainDir), never through a link.
func readRecord(path string) ([]byte, error) { return state.ReadRegular(path, maxRecordRead) }

// removeRecord removes a record; one already gone is removed.
func removeRecord(path string) error {
	if err := state.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
