package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/state"
)

// PutLocal retains a report in the caller's mailbox without announcing it back
// to the same failing conversation. The caller holds that mailbox's lock. A
// copy that cannot be looked up may be the one kept already, so it is an
// error rather than a second copy.
func PutLocal(dir string, message Message) error { return live(dir).putLocal(message) }

func (w world) putLocal(message Message) error {
	for _, directory := range []string{state.UnreadPath(w.dir, message.To), state.DonePath(w.dir, message.To)} {
		path := filepath.Join(directory, message.ID+".json")
		info, err := w.stat(path)
		if err == nil && !info.Mode().IsRegular() {
			return unknownRecord(path, fmt.Errorf("%s, where a letter belongs, is not a regular file", path))
		}
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	directory := state.UnreadPath(w.dir, message.To)
	if err := w.ensureDir(directory); err != nil {
		return err
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return w.writeFile(filepath.Join(directory, message.ID+".json"), raw)
}
