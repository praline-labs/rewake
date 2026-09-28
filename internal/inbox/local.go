package inbox

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/state"
)

// PutLocal retains a report in the caller's mailbox without announcing it back
// to the same failing conversation. The caller holds that mailbox's lock.
func PutLocal(dir string, message Message) error {
	for _, directory := range []string{state.UnreadPath(dir, message.To), state.DonePath(dir, message.To)} {
		if _, err := os.Stat(filepath.Join(directory, message.ID+".json")); err == nil {
			return nil
		}
	}
	directory := state.UnreadPath(dir, message.To)
	if err := state.EnsureSubdir(directory); err != nil {
		return err
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return state.WriteAtomic(filepath.Join(directory, message.ID+".json"), raw)
}
