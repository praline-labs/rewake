package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// PutLocal retains a report in the caller's mailbox without announcing it back
// to the same failing conversation. The caller holds that mailbox's lock. A
// copy that cannot be looked up may be the one kept already, so it is an
// error rather than a second copy.
func PutLocal(dir string, message Message) error {
	for _, directory := range []string{state.UnreadPath(dir, message.To), state.DonePath(dir, message.To)} {
		path := filepath.Join(directory, message.ID+".json")
		info, err := os.Stat(path)
		if err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("%s, where a letter belongs, is not a regular file", path)
		}
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
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
