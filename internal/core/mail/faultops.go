//go:build rewakefault

package mail

import (
	"io/fs"
	"os"
	"sync"
	"sync/atomic"
)

// The operations of this file let the delivery server's tests, in another
// package, build the two fault schedules that no exported operation, file or
// state.Fault reaches: a waiting copy whose removal fails, and the moment the
// stops of a mailbox are listed (docs/v2/stage3-moves.md, the fault exception
// (c)). They exist only in a build with the rewakefault tag.

// failingRemovals holds, by path, the attempt counter of each waiting copy
// whose removal a test made fail.
var failingRemovals sync.Map

func init() {
	// Replaced once, before any test or server runs, so a test switching a
	// failure on while a server serves races with nothing.
	removeWaiting = func(path string) error {
		if attempts, ok := failingRemovals.Load(path); ok {
			attempts.(*atomic.Int64).Add(1)
			return os.ErrPermission
		}
		return os.Remove(path)
	}
}

// FailWaitingRemoval makes the removal of exactly the waiting copy at path fail
// with os.ErrPermission until restore, whether settling or a withdrawal removes
// it. attempts says how many removals it refused, so a test can tell the step
// was reached. Every other path is removed as in any build.
func FailWaitingRemoval(path string) (attempts func() int64, restore func()) {
	counter := &atomic.Int64{}
	failingRemovals.Store(path, counter)
	return counter.Load, func() { failingRemovals.CompareAndDelete(path, counter) }
}

// AfterStopListing runs fn once, right after the stops of name's mailbox in dir
// are next listed, the only way to tell whether a settling read them. Every
// other file operation is as before, including a test's own access to dir put in
// place earlier. restore takes the observer away; calling it again does nothing.
func AfterStopListing(dir, name string, fn func()) (restore func()) {
	previous, had := testAccess.Load(dir)
	access := passAccess{live: osAccess{}, plan: osAccess{}}
	if had {
		access = previous.(passAccess)
	}
	probe := &stopAfterListing{fileAccess: access.live, root: stopsPath(dir, name)}
	probe.after.Store(&fn)
	testAccess.Store(dir, passAccess{live: probe, plan: access.plan})
	var once sync.Once
	return func() {
		once.Do(func() {
			if had {
				testAccess.Store(dir, previous)
			} else {
				testAccess.Delete(dir)
			}
		})
	}
}

// stopAfterListing runs after once, right after the stops of a mailbox are
// listed: where a holder of the lock records an occurrence while a settling
// that does not hold it has decided and not yet removed.
type stopAfterListing struct {
	fileAccess
	root  string
	after atomic.Pointer[func()]
}

func (p *stopAfterListing) ReadDir(path string) ([]fs.DirEntry, error) {
	entries, err := p.fileAccess.ReadDir(path)
	if path == p.root {
		if after := p.after.Swap(nil); after != nil {
			(*after)()
		}
	}
	return entries, err
}
