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
}

// workersWait is how long the end of a run waits for its workers before it
// returns and leaves its hold on the lease to the last of them.
const workersWait = 500 * time.Millisecond

// end lets go of the run's hold on the lease once its workers have stopped.
// It runs after every stop of the run, so no worker is added after it starts
// waiting.
func (l *lifetime) end() {
	done := make(chan struct{})
	go func() {
		l.workers.Wait()
		close(done)
	}()
	select {
	case <-done:
		l.keep()
	case <-time.After(workersWait):
		go func() {
			<-done
			l.keep()
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
