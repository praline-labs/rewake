package mail

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// matrixAccess is the file access of one world of the decision's matrix
// (docs/v2/stage3-decision-manifest.md#clean-runs-and-crash-cuts), put in
// place for its state directory alone, so worlds run side by side. It logs
// every durable step the product takes while a step of the schedule is armed
// — a write, a publication, a rename, a removal or a flush — and makes one of
// them its cut: killed, that step and every change after it fail as a process
// stopped there leaves them, until the schedule lifts the cut; flushed, that
// step's change is made and its flush answers an error. It counts every
// letter written into a mailbox, by id, whether or not the letter is still
// there, so a copy's landings are counted from the writes and not from what
// reads and sweeps left; and it plays the faults a world's evidence names,
// on reads.
type matrixAccess struct {
	osAccess
	mu      sync.Mutex
	armed   bool
	log     []string
	cut     int
	flush   bool
	fired   bool
	pending bool
	killed  bool
	letters map[string]int
	stat    func(path string, plan bool) (fs.FileInfo, error)
	// beforeOp is told each durable step a live process reaches, before it
	// is made: the world's events placed on a point of a path happen there,
	// and an error fails the step as a fault of the file system would.
	beforeOp func(op, path string) error
	// onStat is told each stat a pass makes.
	onStat func(path string, plan bool)
	// onCut is told when the cut fires, before its step is made or failed.
	onCut func()
	// beforeLock is told each mailbox lock a live process asks for, before
	// it waits for it.
	beforeLock func(mailbox string)
}

func newMatrixAccess(dir string) *matrixAccess {
	a := &matrixAccess{letters: map[string]int{}}
	testAccess.Store(dir, passAccess{live: matrixPass{a, false}, plan: matrixPass{a, true}})
	return a
}

// matrixPass is the access of one pass: the plan's reads may be faulted
// apart from the live pass's, as an unknown only an effect meets is.
type matrixPass struct {
	*matrixAccess
	plan bool
}

func (p matrixPass) Stat(path string) (fs.FileInfo, error) {
	p.mu.Lock()
	fault, hook, killed := p.stat, p.onStat, p.killed
	p.mu.Unlock()
	if hook != nil && !killed {
		hook(path, p.plan)
	}
	if fault != nil {
		if info, err := fault(path, p.plan); info != nil || err != nil {
			return info, err
		}
	}
	return p.osAccess.Stat(path)
}

// durable runs one durable step: logged when armed, and failed as the cut
// says. change is false for a flush, which a killed process never makes and
// a failing flush answers without doing.
func (a *matrixAccess) durable(op, path string, change bool, do func() error) error {
	a.mu.Lock()
	hook, killed := a.beforeOp, a.killed
	a.mu.Unlock()
	if killed {
		return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	if hook != nil {
		if err := hook(op, path); err != nil {
			return err
		}
	}
	a.mu.Lock()
	if a.killed {
		a.mu.Unlock()
		return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	at := 0
	if a.armed {
		a.log = append(a.log, op+" "+path)
		at = len(a.log)
	}
	cut := at > 0 && at == a.cut && !a.fired
	if cut {
		a.fired, a.pending = true, true
		a.killed = !a.flush
	}
	flush, onCut := a.flush, a.onCut
	a.mu.Unlock()
	if cut && onCut != nil {
		onCut()
	}
	if cut && !flush {
		return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	if cut && !change {
		return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	if err := do(); err != nil {
		return err
	}
	if cut {
		return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	return nil
}

// changing fails a change a killed process would not make, without logging
// it: making a directory is no step of its own.
func (a *matrixAccess) changing(op, path string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.killed {
		return &os.PathError{Op: op, Path: path, Err: syscall.EIO}
	}
	return nil
}

func (a *matrixAccess) noteLetter(path string) {
	if !strings.HasSuffix(path, ".json") || !strings.Contains(path, string(filepath.Separator)+"inbox"+string(filepath.Separator)) {
		return
	}
	a.mu.Lock()
	a.letters[strings.TrimSuffix(filepath.Base(path), ".json")]++
	a.mu.Unlock()
}

func (a *matrixAccess) WriteFile(path string, raw []byte) error {
	return a.durable("write", path, true, func() error {
		if err := a.osAccess.WriteFile(path, raw); err != nil {
			return err
		}
		a.noteLetter(path)
		return nil
	})
}

func (a *matrixAccess) Publish(path string, raw []byte) error {
	return a.durable("publish", path, true, func() error {
		if err := a.osAccess.Publish(path, raw); err != nil {
			return err
		}
		a.noteLetter(path)
		return nil
	})
}

func (a *matrixAccess) Rename(from, to string) error {
	return a.durable("rename", to, true, func() error { return a.osAccess.Rename(from, to) })
}

func (a *matrixAccess) Remove(path string) error {
	return a.durable("remove", path, true, func() error { return a.osAccess.Remove(path) })
}

// A flush is logged and cut as any durable step, and not made: no world
// loses power, so what reaches the disk is what was written, and the flushes
// would only make every world slower.
func (a *matrixAccess) SyncDir(path string) error {
	return a.durable("sync-dir", path, false, func() error { return nil })
}

func (a *matrixAccess) SyncFile(path string) error {
	return a.durable("sync-file", path, false, func() error { return nil })
}

func (a *matrixAccess) EnsureDir(path string) error {
	if err := a.changing("mkdir", path); err != nil {
		return err
	}
	return a.osAccess.EnsureDir(path)
}

func (a *matrixAccess) Lock(ctx context.Context, mailbox string, fn func() error) error {
	a.mu.Lock()
	hook := a.beforeLock
	a.mu.Unlock()
	if hook != nil {
		hook(mailbox)
	}
	return a.osAccess.Lock(ctx, mailbox, fn)
}

// arm logs the product's steps from here on, until disarm.
func (a *matrixAccess) arm(on bool) {
	a.mu.Lock()
	a.armed = on
	a.mu.Unlock()
}

// lift ends a kill: the process the cut stopped is gone, and the next
// command runs in one that is not. It says whether the cut fired since the
// last lift.
func (a *matrixAccess) lift() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	fired := a.pending
	a.killed, a.pending = false, false
	return fired
}

func (a *matrixAccess) steps() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.log...)
}

func (a *matrixAccess) written(id string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.letters[id]
}

// notRegular is what a stat says of a path that is there and is no file: a
// letter's place holding it hides whether the letter is there.
type notRegular struct{ name string }

func (n notRegular) Name() string     { return n.name }
func (notRegular) Size() int64        { return 0 }
func (notRegular) Mode() fs.FileMode  { return fs.ModeDir | 0o700 }
func (notRegular) ModTime() time.Time { return time.Time{} }
func (notRegular) IsDir() bool        { return true }
func (notRegular) Sys() any           { return nil }
