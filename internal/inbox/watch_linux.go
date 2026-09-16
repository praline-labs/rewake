package inbox

import (
	"context"
	"syscall"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

/*
Watching the mailbox instead of asking it four times a second.

A message arrives as a rename into the directory, and the kernel can say so. The
poll stays as the safety net — it is what retries a pending message and what
covers a watch that could not be set up at all — but it can be slow, because the
common case no longer waits for it.
*/

// watchPatience is how long one wait for an event lasts before the context is
// looked at again. The descriptor is non-blocking and waited on rather than read
// blindly: closing a descriptor that another goroutine is blocked on is a way to
// close somebody else's file once the number is reused, which is exactly what
// happened when it was written the other way.
const watchPatience = 250 * time.Millisecond

// watchMailbox reports each change to a mailbox until the context ends. It
// returns nil when the kernel cannot give a watch, and the caller then lives on
// its ticker alone.
func watchMailbox(ctx context.Context, dir, name string) <-chan struct{} {
	if err := state.EnsureSubdir(state.InboxPath(dir, name)); err != nil {
		return nil
	}

	descriptor, err := syscall.InotifyInit1(syscall.IN_CLOEXEC | syscall.IN_NONBLOCK)
	if err != nil {
		return nil
	}
	// A message is written to a temporary file and renamed into place, so the
	// event that matters is the move. The others are here for a sender that
	// writes differently, and for a mailbox that is recreated under us.
	const events = syscall.IN_MOVED_TO | syscall.IN_CLOSE_WRITE | syscall.IN_CREATE | syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF
	if _, err := syscall.InotifyAddWatch(descriptor, state.InboxPath(dir, name), events); err != nil {
		syscall.Close(descriptor)
		return nil
	}

	changed := make(chan struct{}, 1)
	go func() {
		defer close(changed)
		defer syscall.Close(descriptor)

		buffer := make([]byte, 8*syscall.SizeofInotifyEvent+syscall.NAME_MAX+1)
		for ctx.Err() == nil {
			ready, err := waitReadable(descriptor, watchPatience)
			if err != nil {
				return
			}
			if !ready {
				continue
			}
			if _, err := syscall.Read(descriptor, buffer); err != nil {
				if err == syscall.EAGAIN || err == syscall.EINTR {
					continue
				}
				return
			}
			select {
			case changed <- struct{}{}:
			default:
				// A pass is already due; one is enough for any number of events.
			}
		}
	}()

	return changed
}

// waitReadable reports whether the descriptor has something to read, waiting at
// most this long.
func waitReadable(descriptor int, patience time.Duration) (bool, error) {
	var set syscall.FdSet
	set.Bits[descriptor/64] |= 1 << (uint(descriptor) % 64)
	timeout := syscall.NsecToTimeval(int64(patience))

	ready, err := syscall.Select(descriptor+1, &set, nil, nil, &timeout)
	if err != nil {
		if err == syscall.EINTR {
			return false, nil
		}
		return false, err
	}
	return ready > 0, nil
}
