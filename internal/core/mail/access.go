package mail

import (
	"errors"
	"io/fs"
	"os"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// world is the file access of the mailboxes of a room: a record of a mailbox
// is read as a regular file and nothing else (state.ReadRegular), since a
// mailbox holds no other kind, and a directory is listed only as itself
// (state.ReadPlainDir), never through a link.
type world struct {
	dir string
}

func live(dir string) world { return world{dir: dir} }

// maxRecordRead bounds a read of a record of a mailbox.
const maxRecordRead = 100 << 20

func (w world) readFile(path string) ([]byte, error) { return state.ReadRegular(path, maxRecordRead) }

func (w world) readDir(path string) ([]fs.DirEntry, error) { return state.ReadPlainDir(path) }

func (w world) stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

// writeFile replaces a file in one step (state.WriteAtomic).
func (w world) writeFile(path string, raw []byte) error { return state.WriteAtomic(path, raw) }

func (w world) ensureDir(path string) error { return state.EnsureSubdir(path) }

// remove removes a file; one already gone is removed.
func (w world) remove(path string) error {
	if err := state.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
