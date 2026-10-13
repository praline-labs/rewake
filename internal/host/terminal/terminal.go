package terminal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// readSize is one read of the program's output.
	readSize = 32 << 10
	// holdFor is how long the last glyph cluster of a read waits for a mark
	// that may follow it in the next read (hold.go).
	holdFor = 5 * time.Millisecond
	// replyQueue bounds the answers waiting for the program to read them. A
	// program that queries and never reads its input loses answers beyond
	// it, counted, rather than stopping the emulator in the middle of a write.
	replyQueue = 64
	// writeBound is the longest a write to the program waits for it to read
	// its input: every wait has a bound.
	writeBound = 2 * time.Second
)

// Terminal is a program's pseudo-terminal and the screen it draws there. One
// goroutine reads the program's output; another writes each read whole into
// the emulator under the lock before any reader of the screen sees it, since
// the emulator's callbacks run inside the write and a look between them sees
// half a frame. A third takes the emulator's answers to the program's queries
// off its pipe at once, and a fourth writes them to the program, so the
// emulator never waits on a program that does not read.
type Terminal struct {
	master *os.File
	sideMu sync.Mutex
	side   *os.File

	// mu guards the emulator and what is said of it: everything written to
	// it, its size, whether anything changed since the last look.
	mu    sync.Mutex
	emu   emulator
	size  Size
	dirty bool
	ended bool
	// wake is closed when the screen may have changed, and replaced.
	wake chan struct{}

	// pubMu guards what was published; a look runs under mu inside it.
	pubMu sync.Mutex
	pub   published

	// writeMu makes every write to the program one write, whole, never
	// interleaved with another: a key split as ESC O and C reads as Alt+O
	// and a stray C.
	writeMu sync.Mutex
	out     io.Writer

	chunks  chan []byte
	answers chan []byte
	fed     chan struct{}
	done    chan struct{}
	wg      sync.WaitGroup
	once    sync.Once
	closed  error

	repliesDropped   atomic.Uint64
	clipboardRefused atomic.Uint64
	clipMu           sync.Mutex
	clipboard        []byte
	clipboardSeq     uint64
}

// Open opens a pseudo-terminal of size and starts reading it. The program
// is started on it by the caller, through Prepare or with Side, after which
// ReleaseSide lets its end be seen.
func Open(size Size) (*Terminal, error) {
	master, side, err := openPair(size)
	if err != nil {
		return nil, err
	}
	t := &Terminal{
		master: master, side: side, out: master, size: size,
		wake: make(chan struct{}), chunks: make(chan []byte, 4), answers: make(chan []byte, replyQueue),
		fed: make(chan struct{}), done: make(chan struct{}),
	}
	t.emu, err = newEmulator(size, t.takeClipboard, func() { t.clipboardRefused.Add(1) })
	if err != nil {
		_ = side.Close()
		_ = master.Close()
		return nil, err
	}
	t.wg.Add(4)
	go t.read()
	go t.feed()
	go t.drain()
	go t.answer()
	return t, nil
}

func (t *Terminal) read() {
	defer t.wg.Done()
	defer close(t.chunks)
	buf := make([]byte, readSize)
	for {
		n, err := t.master.Read(buf)
		if n > 0 {
			t.chunks <- append([]byte(nil), buf[:n]...)
		}
		if err != nil {
			// EIO once the program's last process closed its side, or the
			// master closed by Close: either way the output is over.
			return
		}
	}
}

func (t *Terminal) feed() {
	defer t.wg.Done()
	defer close(t.fed)
	var h holder
	timer := time.NewTimer(holdFor)
	timer.Stop()
	for {
		var expired <-chan time.Time
		if h.holding() {
			expired = timer.C
		}
		select {
		case chunk, ok := <-t.chunks:
			if !ok {
				t.commit(h.release(), true)
				return
			}
			t.commit(h.take(chunk), false)
			if h.holding() {
				timer.Reset(holdFor)
			}
		case <-expired:
			t.commit(h.release(), false)
		}
	}
}

// commit writes one piece of output into the emulator, whole, and only then
// wakes the readers of the screen.
func (t *Terminal) commit(p []byte, end bool) {
	if len(p) == 0 && !end {
		return
	}
	t.mu.Lock()
	if len(p) > 0 {
		t.emu.write(p)
		t.dirty = true
	}
	if end {
		t.ended = true
	}
	wake := t.wake
	t.wake = make(chan struct{})
	t.mu.Unlock()
	close(wake)
	if end {
		close(t.done)
	}
}

func (t *Terminal) drain() {
	defer t.wg.Done()
	defer close(t.answers)
	r := t.emu.replies()
	buf := make([]byte, 4<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			select {
			case t.answers <- append([]byte(nil), buf[:n]...):
			default:
				t.repliesDropped.Add(1)
			}
		}
		if err != nil {
			return
		}
	}
}

func (t *Terminal) answer() {
	defer t.wg.Done()
	for reply := range t.answers {
		if err := t.send(reply); err != nil {
			t.repliesDropped.Add(1)
		}
	}
}

// send writes p to the program in one write, within writeBound.
func (t *Terminal) send(p []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	_ = t.master.SetWriteDeadline(time.Now().Add(writeBound))
	_, err := t.out.Write(p)
	return err
}

// Keys writes keys to the program as one write, whole: what one post of the
// key surface carries is never split, and never merged with another post.
func (t *Terminal) Keys(p []byte) error {
	if len(p) == 0 {
		return nil
	}
	if err := t.send(p); err != nil {
		return fmt.Errorf("could not write keys to the terminal: %w", err)
	}
	return nil
}

// Resize sizes the grid first and then the pseudo-terminal, under the lock
// that feeds the grid: the program draws for its new size only once the
// kernel tells it, and by then the grid has that size, so no output for one
// size lands in a grid of another. The library does not reflow; a new size
// republishes every row.
func (t *Terminal) Resize(size Size) error {
	if err := size.check(); err != nil {
		return err
	}
	t.mu.Lock()
	t.emu.resize(size)
	if err := setSize(t.master, size); err != nil {
		t.emu.resize(t.size)
		t.mu.Unlock()
		return err
	}
	t.size, t.dirty = size, true
	wake := t.wake
	t.wake = make(chan struct{})
	t.mu.Unlock()
	close(wake)
	return nil
}

// Screen is the screen for a reader that has the sequence after: at once
// when it changed since, when after is a sequence this terminal never gave,
// or when the output is over; otherwise when it changes, or ctx ends, and
// then with the same sequence and no rows.
func (t *Terminal) Screen(ctx context.Context, after uint64) Screen {
	for {
		t.mu.Lock()
		wake, ended := t.wake, t.ended
		t.mu.Unlock()
		s := t.publish(after)
		s.Ended = ended
		if s.Seq != after || ended || ctx.Err() != nil {
			return s
		}
		select {
		case <-wake:
		case <-ctx.Done():
		}
	}
}

func (t *Terminal) publish(after uint64) Screen {
	t.pubMu.Lock()
	defer t.pubMu.Unlock()
	t.mu.Lock()
	fresh := t.dirty || t.pub.seq == 0
	var l look
	if fresh {
		l = t.emu.look()
		t.dirty = false
	}
	t.mu.Unlock()
	if fresh {
		t.pub.take(l)
	}
	return t.pub.since(after)
}

// Done is closed once the program's output is over: its last process closed
// the side, or the terminal was closed.
func (t *Terminal) Done() <-chan struct{} { return t.done }

func (t *Terminal) takeClipboard(payload []byte) {
	t.clipMu.Lock()
	defer t.clipMu.Unlock()
	t.clipboard, t.clipboardSeq = payload, t.clipboardSeq+1
}

// Clipboard is the latest text the program asked to put on the clipboard,
// and how many it asked so far; the newest replaces the one before.
func (t *Terminal) Clipboard() ([]byte, uint64) {
	t.clipMu.Lock()
	defer t.clipMu.Unlock()
	return t.clipboard, t.clipboardSeq
}

// Stats counts what the terminal dropped rather than wait on the program.
type Stats struct {
	// RepliesDropped are answers to the program's queries it did not read
	// in time, or that did not fit the queue.
	RepliesDropped uint64 `json:"repliesDropped"`
	// ClipboardRefused are clipboard requests too large, unreadable, or
	// asking to read the clipboard.
	ClipboardRefused uint64 `json:"clipboardRefused"`
}

// Stats is what was dropped so far.
func (t *Terminal) Stats() Stats {
	return Stats{RepliesDropped: t.repliesDropped.Load(), ClipboardRefused: t.clipboardRefused.Load()}
}

// Close hangs the terminal up: the master is closed, so the program's
// session gets SIGHUP, the output ends and is written whole, then the reply
// pipe is closed and every goroutine joined. A second Close returns the
// first's answer.
func (t *Terminal) Close() error {
	t.once.Do(func() {
		t.closed = t.master.Close()
		if err := t.ReleaseSide(); err != nil && t.closed == nil {
			t.closed = err
		}
		<-t.fed
		if err := t.emu.stopReplies(); err != nil && !errors.Is(err, io.ErrClosedPipe) && t.closed == nil {
			t.closed = err
		}
		t.wg.Wait()
	})
	return t.closed
}
