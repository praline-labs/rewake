package inbox

import (
	"errors"
	"io/fs"
	"os"
	"sync"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// The barrier decides by what its effects read, and a stop is worth something
// only when it is found before the first effect. A second list of what the
// effects will read, kept beside them, drifts from them whenever an effect
// learns to read something new. So there is no such list: the barrier first
// runs its own code as a plan, which reads everything the effects will read
// and writes nothing, and only a plan with no unknown lets the same code run
// again for real (docs/mailbox-records.md#the-plan-and-the-seam). Every file
// the two passes touch goes through one seam, fileAccess, so a test can see
// every path the plan read and fail each of them in turn.

// fileAccess is every file operation of the barrier and the gates.
type fileAccess interface {
	ReadFile(path string) ([]byte, error)
	ReadDir(path string) ([]fs.DirEntry, error)
	Stat(path string) (fs.FileInfo, error)
	// WriteFile replaces a file in one step (state.WriteAtomic).
	WriteFile(path string, raw []byte) error
	EnsureDir(path string) error
	Remove(path string) error
	Rename(from, to string) error
	SyncDir(path string) error
}

type osAccess struct{}

func (osAccess) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (osAccess) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (osAccess) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }
func (osAccess) WriteFile(path string, raw []byte) error    { return state.WriteAtomic(path, raw) }
func (osAccess) EnsureDir(path string) error                { return state.EnsureSubdir(path) }
func (osAccess) Remove(path string) error                   { return os.Remove(path) }
func (osAccess) Rename(from, to string) error               { return os.Rename(from, to) }
func (osAccess) SyncDir(path string) error                  { return state.SyncDir(path) }

// testAccess holds, by state directory, the passAccess a test put in place
// of the file system: one that records what each pass read, or fails a path
// in one pass or both.
var testAccess sync.Map

type passAccess struct{ live, plan fileAccess }

func accessFor(dir string, plan bool) fileAccess {
	if access, ok := testAccess.Load(dir); ok {
		if plan {
			return access.(passAccess).plan
		}
		return access.(passAccess).live
	}
	return osAccess{}
}

// world is one pass over the mailboxes of a room: the plan, which writes
// nothing, or the pass that makes the effects.
type world struct {
	dir   string
	files fileAccess
	plan  bool
}

func live(dir string) world { return world{dir: dir, files: accessFor(dir, false)} }

func planning(dir string) world { return world{dir: dir, files: accessFor(dir, true), plan: true} }

// readFile reads a file of a mailbox. Only a missing file is an absence
// (rule 6); any other failure leaves what the file says unknown, whichever
// pass met it, so an effect that meets it stops the mailbox as the plan
// would have.
func (w world) readFile(path string) ([]byte, error) {
	raw, err := w.files.ReadFile(path)
	return raw, unknownRead(path, err)
}

func (w world) readDir(path string) ([]fs.DirEntry, error) {
	entries, err := w.files.ReadDir(path)
	return entries, unknownRead(path, err)
}

func (w world) stat(path string) (fs.FileInfo, error) {
	info, err := w.files.Stat(path)
	return info, unknownRead(path, err)
}

func unknownRead(path string, err error) error {
	var unknown *UnknownRecordError
	if err == nil || errors.Is(err, os.ErrNotExist) || errors.As(err, &unknown) {
		return err
	}
	return unknownRecord(path, err)
}

// The writes do nothing in a plan: it decides as the effects will, and the
// decisions it keeps in memory are what those writes would have recorded.

func (w world) writeFile(path string, raw []byte) error {
	if w.plan {
		return nil
	}
	return w.files.WriteFile(path, raw)
}

func (w world) ensureDir(path string) error {
	if w.plan {
		return nil
	}
	return w.files.EnsureDir(path)
}

// remove removes a file; one already gone is removed.
func (w world) remove(path string) error {
	if w.plan {
		return nil
	}
	if err := w.files.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (w world) rename(from, to string) error {
	if w.plan {
		return nil
	}
	return w.files.Rename(from, to)
}

func (w world) syncDir(path string) error {
	if w.plan {
		return nil
	}
	return w.files.SyncDir(path)
}

// The registry is not behind the seam: which runs live is live state, which
// changes between any two looks. A plan asks it without the cleanup a lookup
// does on the way, and gets the same answer.

func (w world) lookup(name string) (registry.Session, error) {
	if w.plan {
		return registry.LookupReadOnly(w.dir, name)
	}
	return registry.Lookup(w.dir, name)
}

func (w world) sessions() ([]registry.Session, error) {
	if w.plan {
		return registry.ListReadOnly(w.dir)
	}
	return registry.List(w.dir)
}
