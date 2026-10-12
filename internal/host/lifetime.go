package host

import (
	"sync"
	"time"
)

// lifetime keeps the room's build lease through the run's last effect
// (docs/v2/stage3-state.md#one-build-per-room). Every goroutine the run starts
// beside its harness that may write the room counts among its workers, and
// the run's hold on the lease goes only once the last of them has stopped.
// The stops that end them wait only so long, so a write stalled in the
// filesystem does not hold the wrapper's exit; Run may then return first, and
// the lease goes when that worker ends, or with the process — never while it
// can still write.
type lifetime struct {
	workers sync.WaitGroup
	keep    func()
	// last are the run's cleanups that wait for its workers: they run once
	// the last has stopped, before the hold goes, latest first.
	last []func()
}

// workersWait is how long the end of a run waits for its workers before it
// returns and leaves its hold on the lease to the last of them.
const workersWait = 500 * time.Millisecond

// atEnd adds a cleanup that has to wait for every worker of the run: what a
// worker may still write into goes only after it has stopped.
func (l *lifetime) atEnd(cleanup func()) { l.last = append(l.last, cleanup) }

// end runs the run's last cleanups and lets go of its hold on the lease once
// its workers have stopped. It runs after every stop of the run, so no worker
// is added after it starts waiting. Past workersWait it returns and leaves
// both to the last worker: if the wrapper exits first, the cleanups do not
// run, as after a kill, and a later launch reclaims what they would have
// removed once the run is proven dead.
func (l *lifetime) end() {
	done := make(chan struct{})
	go func() {
		l.workers.Wait()
		close(done)
	}()
	finish := func() {
		for i := len(l.last) - 1; i >= 0; i-- {
			l.last[i]()
		}
		l.keep()
	}
	select {
	case <-done:
		finish()
	case <-time.After(workersWait):
		go func() {
			<-done
			finish()
		}()
	}
}

// spawn runs work as a worker of the run; with no workers to count in, as a
// plain goroutine.
func spawn(workers *sync.WaitGroup, work func()) {
	if workers == nil {
		go work()
		return
	}
	workers.Go(work)
}
