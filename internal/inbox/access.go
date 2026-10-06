package inbox

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"sync"

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
	// Publish writes a file once (state.PublishExclusive): one that exists
	// answers state.ErrNameTaken.
	Publish(path string, raw []byte) error
	EnsureDir(path string) error
	Remove(path string) error
	Rename(from, to string) error
	SyncDir(path string) error
	// Lock runs fn under the lock of the mailbox at path
	// (state.WithMailboxLockAt): the lock file is a file of that mailbox.
	Lock(ctx context.Context, mailbox string, fn func() error) error
}

type osAccess struct{}

func (osAccess) ReadFile(path string) ([]byte, error)       { return state.ReadFile(path) }
func (osAccess) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (osAccess) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }
func (osAccess) WriteFile(path string, raw []byte) error    { return state.WriteAtomic(path, raw) }
func (osAccess) Publish(path string, raw []byte) error      { return state.PublishExclusive(path, raw) }
func (osAccess) EnsureDir(path string) error                { return state.EnsureSubdir(path) }
func (osAccess) Remove(path string) error                   { return state.Remove(path) }
func (osAccess) Rename(from, to string) error               { return state.Rename(from, to) }
func (osAccess) SyncDir(path string) error                  { return state.SyncDir(path) }
func (osAccess) Lock(ctx context.Context, mailbox string, fn func() error) error {
	return state.WithMailboxLockAt(ctx, mailbox, fn)
}

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
	// held are the open stop occurrences about a report, by its id: such a
	// report is never recorded moot (stop.go).
	held map[string]openStop
	// decided collects, in a plan, the operations it ran through: the
	// journals and the reports.
	decided map[string]bool
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

func (w world) publish(path string, raw []byte) error {
	if w.plan {
		return nil
	}
	return w.files.Publish(path, raw)
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

// lock holds the mailbox lock of name, in the pass that writes; the plan
// takes no lock, since it writes nothing and decides nothing for good.
func (w world) lock(ctx context.Context, name string, fn func() error) error {
	if w.plan {
		return fn()
	}
	return w.files.Lock(ctx, state.InboxPath(w.dir, name), fn)
}

func (w world) syncDir(path string) error {
	if w.plan {
		return nil
	}
	return w.files.SyncDir(path)
}
