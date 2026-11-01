package state

import (
	"fmt"
	"path/filepath"
)

// UpdateRecordName is the record of the last check for a newer release,
// directly under the state root: one per state base, shared by every room
// of it, and no room's state (docs/install.md#updating-from-inside-rewake).
const UpdateRecordName = "update.json"

// WriteUpdateRecord replaces root's update record in one step. It is the one
// write under the root outside every room, and needs no room's lease: the
// record is a cache of what the registry said, and the last writer winning is
// harmless. It still refuses in a view, refuses a root that is not one — a
// directory named v2 that verifies — and creates nothing: a root no launch
// made gets no record.
func WriteUpdateRecord(root string, data []byte) error {
	path := filepath.Join(root, UpdateRecordName)
	if err := viewWrite(OpWrite, path); err != nil {
		return err
	}
	if filepath.Base(root) != rootName || !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return fmt.Errorf("%s is not a state root", root)
	}
	if err := Verify(root); err != nil {
		return err
	}
	return replace(path, data, nil)
}
