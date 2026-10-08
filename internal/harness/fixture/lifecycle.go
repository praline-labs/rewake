//go:build rewakefixture

package fixture

import (
	"errors"
	"os"
	"sync/atomic"
	"syscall"
	"time"
)

// errClosing is a call into the wrapper's handler refused because the
// backend is closing.
var errClosing = errors.New("the fixture's adapter is closing")

// withdraw ends what a connection made live, at once. A nil link is the
// program's end, which withdraws whatever connection there is. What a
// withdrawn connection started stops at its next step: everything the adapter
// does for a frame asks liveOn of the connection the frame came on.
func (b *backend) withdraw(l *link) {
	b.mu.Lock()
	current := b.link
	if l == nil || current == l {
		b.link, b.live = nil, map[string]bool{}
		b.telem = telemetry{}
	}
	b.mu.Unlock()
	if l == nil && current != nil {
		current.close()
	}
}

// liveOn answers whether a capability is live on this connection: the one the
// backend holds now, not a later one that made the same capability live again,
// and not one already closed whose withdrawal has yet to land.
func (b *backend) liveOn(l *link, capability string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return l != nil && !l.gone() && b.link == l && b.live[capability]
}

// enter admits one call into the wrapper's handler, and refuses it once Close
// has begun; leave ends it.
func (b *backend) enter() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closing {
		return false
	}
	b.users.Add(1)
	return true
}

func (b *backend) leave() { b.users.Done() }

// dispatchHook, when a test sets it, runs on a frame's own goroutine before
// its work: the moment a scheduler could delay that work by.
var dispatchHook atomic.Pointer[func(Frame)]

// dispatch runs the part of a frame's handling that waits — for the core, or
// for the program to take an answer — on its own goroutine, so nothing behind
// it on the line waits too. What fixes the frame's moment has been done on
// the reader before this.
func (b *backend) dispatch(frame Frame, work func()) {
	b.mu.Lock()
	if b.closing {
		b.mu.Unlock()
		return
	}
	b.routines.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.routines.Done()
		if hook := dispatchHook.Load(); hook != nil {
			(*hook)(frame)
		}
		work()
	}()
}

// Close stops the program and returns once nothing of the backend runs: no
// connection is read, no frame is handled and no call into the wrapper's
// handler is under way. Each of those is bounded — the connections are closed,
// the context the core is called under is canceled — so the wait is too.
func (b *backend) Close() {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.closing = true
		var waiting []func() error
		for conn := range b.waiting {
			waiting = append(waiting, conn.Close)
		}
		b.mu.Unlock()
		if b.listener != nil {
			_ = b.listener.Close()
		}
		if b.cancel != nil {
			b.cancel()
		}
		for _, closeConn := range waiting {
			_ = closeConn()
		}
		b.withdraw(nil)
		b.stopProgram()
		b.routines.Wait()
		b.users.Wait()
		_ = os.Remove(b.socket)
	})
}

// stopProgram ends the program's group: SIGTERM, then SIGKILL.
func (b *backend) stopProgram() {
	if b.process == nil {
		return
	}
	b.stopOnce.Do(func() {
		select {
		case <-b.exited:
			return
		default:
		}
		_ = syscall.Kill(-b.pid, syscall.SIGTERM)
		select {
		case <-b.exited:
		case <-time.After(stopGrace):
			_ = syscall.Kill(-b.pid, syscall.SIGKILL)
			<-b.exited
		}
	})
}
