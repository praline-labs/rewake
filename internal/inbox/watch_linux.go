package inbox

import (
	"bytes"
	"context"
	"strings"
	"syscall"
	"time"
	"unsafe"

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

// events are what the kernel is asked to report. A message is written to a
// temporary file and renamed into place, so the move is the one that matters;
// the rest cover a sender that writes differently and a mailbox that is removed
// or replaced underneath the watch.
const events = syscall.IN_MOVED_TO | syscall.IN_CLOSE_WRITE | syscall.IN_CREATE |
	syscall.IN_DELETE_SELF | syscall.IN_MOVE_SELF

// fdSetSize is how many descriptors select can wait on.
const fdSetSize = 1024

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
	if descriptor >= fdSetSize {
		// select cannot wait on a descriptor this high, and reaching past the
		// end of the set is a panic that takes the whole wrapper with it. The
		// poll is the answer here, not a crash.
		syscall.Close(descriptor)
		return nil
	}
	if _, err := syscall.InotifyAddWatch(descriptor, state.InboxPath(dir, name), events); err != nil {
		syscall.Close(descriptor)
		return nil
	}

	changed := make(chan struct{}, 1)
	go func() {
		defer close(changed)
		defer func() { syscall.Close(descriptor) }()

		buffer := make([]byte, 16*(syscall.SizeofInotifyEvent+syscall.NAME_MAX+1))
		watching := true
		for ctx.Err() == nil {
			ready, err := waitReadable(descriptor, watchPatience)
			if err != nil {
				return
			}
			if !watching {
				// The mailbox went away and has not come back yet. Asked again
				// every wait, so the watch returns with it; giving up here left
				// delivery on the poll for the rest of the session.
				if _, err := syscall.InotifyAddWatch(descriptor, state.InboxPath(dir, name), events); err != nil {
					continue
				}
				watching = true
				notify(changed)
				continue
			}
			if !ready {
				continue
			}
			read, err := syscall.Read(descriptor, buffer)
			if err != nil {
				if err == syscall.EAGAIN || err == syscall.EINTR {
					continue
				}
				return
			}

			interesting, lost := readEvents(buffer[:read])
			if lost {
				// The directory this watch was on is gone or has been replaced.
				// Without asking for a new one the watch is over, and delivery
				// silently falls back to the poll for good.
				if _, err := syscall.InotifyAddWatch(descriptor, state.InboxPath(dir, name), events); err != nil {
					watching = false
					continue
				}
				interesting = true
			}
			if interesting {
				notify(changed)
			}
		}
	}()

	return changed
}

// notify asks for a pass. One pending is enough for any number of events.
func notify(changed chan struct{}) {
	select {
	case changed <- struct{}{}:
	default:
	}
}

// readEvents says whether anything worth a pass happened, and whether the watch
// itself is gone.
//
// The server writes into this directory too — statuses, temporary files — and
// reacting to its own writes turned one unarchivable message into a loop:
// status written, event, pass, status written again, hundreds of times a second.
// Only a message file counts.
func readEvents(buffer []byte) (interesting bool, lost bool) {
	for offset := 0; offset+syscall.SizeofInotifyEvent <= len(buffer); {
		raw := (*syscall.InotifyEvent)(unsafe.Pointer(&buffer[offset]))
		if raw.Mask&(syscall.IN_IGNORED|syscall.IN_DELETE_SELF|syscall.IN_MOVE_SELF) != 0 {
			lost = true
		}
		if raw.Mask&syscall.IN_Q_OVERFLOW != 0 {
			// Events were dropped, so what is in the mailbox is unknown: look.
			interesting = true
		}

		nameBytes := buffer[offset+syscall.SizeofInotifyEvent : offset+int(syscall.SizeofInotifyEvent)+int(raw.Len)]
		if name := string(bytes.TrimRight(nameBytes, "\x00")); isMessage(name) {
			interesting = true
		}
		offset += int(syscall.SizeofInotifyEvent) + int(raw.Len)
	}
	return interesting, lost
}

// isMessage reports whether this file name is a waiting message rather than
// something the server itself wrote.
func isMessage(name string) bool {
	return strings.HasSuffix(name, ".json") && !strings.HasPrefix(name, ".")
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
