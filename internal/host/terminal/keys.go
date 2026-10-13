package terminal

import (
	"errors"
	"fmt"
	"syscall"
	"time"
)

// What reaches the program is written with one write(2) per post, whole or
// said to be short. os.File's Write loops over a short write and sends the
// rest in later calls, which is the split this package exists to prevent: a
// pseudo-terminal whose program reads nothing takes part of a write and
// refuses the rest (a raw one took 11776 of 65536 bytes when probed in review,
// October 13, 2026).

// maxKeys is the most one post may carry. A post of the key surface is a few
// keys or a line of typed text; a larger one is refused before anything is
// written, so the whole-post promise stays one the kernel can keep.
const maxKeys = 4096

// ShortWrite is a write the program's terminal took only part of. What it
// took reached the program and cannot be taken back; the rest was never
// written, and is not written later.
type ShortWrite struct {
	Written, Of int
}

func (s *ShortWrite) Error() string {
	return fmt.Sprintf("the program's terminal took %d of %d bytes; the rest was not written", s.Written, s.Of)
}

// send writes p to the program with exactly one write(2) that takes bytes,
// within writeBound. A write interrupted before taking any byte is made again
// at once, and one that would block is made again once the terminal is
// writable; one that takes some
// is the only one, and a short one is a *ShortWrite.
func (t *Terminal) send(p []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	_ = t.master.SetWriteDeadline(time.Now().Add(writeBound))
	raw, err := t.master.SyscallConn()
	if err != nil {
		return err
	}
	written := 0
	var failed error
	if err := raw.Write(func(fd uintptr) bool {
		n, err := t.write(int(fd), p)
		// An interrupted call is made again at once: waiting for the poller
		// would wait for an event a writable terminal may never send.
		for errors.Is(err, syscall.EINTR) {
			n, err = t.write(int(fd), p)
		}
		if errors.Is(err, syscall.EAGAIN) {
			return false
		}
		written, failed = max(n, 0), err
		return true
	}); err != nil {
		return err
	}
	if failed != nil {
		return failed
	}
	if written < len(p) {
		return &ShortWrite{Written: written, Of: len(p)}
	}
	return nil
}

// Keys writes one post of keys to the program as one write, whole: never
// split, and never merged with another post. A post over maxKeys is refused
// with nothing written; a short write is a *ShortWrite.
func (t *Terminal) Keys(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	if len(p) > maxKeys {
		return fmt.Errorf("a post of %d bytes is more than the %d one post may carry; nothing was written", len(p), maxKeys)
	}
	if err := t.send(p); err != nil {
		return fmt.Errorf("could not write keys to the terminal: %w", err)
	}
	return nil
}
