package terminal

import (
	"context"
	"errors"
	"image/color"
	"io"
	"sync"
	"time"
)

const (
	// holdFor is how long the last glyph cluster of a write waits for a mark
	// that may follow it in the next one (hold.go).
	holdFor = 5 * time.Millisecond
	// replyRead is the most one read of the emulator's answers takes.
	replyRead = 4096
	// replyQueue bounds the answers waiting to be taken. Answers beyond it
	// are dropped rather than stop the emulator in the middle of a write.
	replyQueue = 64
)

// Grid is the screen a program draws, kept by the emulator from the
// program's output as its reader hands it over. It never touches the
// pseudo-terminal: whoever reads the master writes to the grid, sizes it
// before the terminal, and decides where the emulator's answers go.
//
// One goroutine writes each piece of output whole into the emulator under
// the lock before any reader of the screen sees it, since the emulator's
// callbacks run inside the write and a look between them sees half a frame;
// another takes the emulator's answers off its pipe at once, so the emulator
// never waits on whoever takes them.
type Grid struct {
	// mu guards the emulator and what is said of it.
	mu    sync.Mutex
	emu   emulator
	dirty bool
	ended bool
	// streams are the raw streams attached (stream.go), and unfinished the
	// bytes of a sequence the program has not finished, which each new one
	// ends its preamble with.
	streams    map[*Stream]struct{}
	unfinished unfinished
	// wake is closed when the screen may have changed, and replaced.
	wake chan struct{}

	// pubMu guards what was published; a look runs under mu inside it.
	pubMu sync.Mutex
	pub   published

	// inMu orders a write with the end of the output: a write after it is
	// dropped, never sent on a closed channel.
	inMu    sync.RWMutex
	over    bool
	chunks  chan []byte
	answers chan []byte
	fed     chan struct{}
	wg      sync.WaitGroup
	once    sync.Once
}

// NewGrid is the screen of a program on a terminal of size.
func NewGrid(size Size) (*Grid, error) {
	if err := size.check(); err != nil {
		return nil, err
	}
	emu, err := newEmulator(size)
	if err != nil {
		return nil, err
	}
	g := &Grid{
		emu: emu, wake: make(chan struct{}), chunks: make(chan []byte, 64),
		answers: make(chan []byte, replyQueue), fed: make(chan struct{}),
	}
	g.wg.Add(2)
	go g.feed()
	go g.drain()
	return g, nil
}

// Write takes a piece of the program's output; p may be reused once it
// returns.
func (g *Grid) Write(p []byte) {
	g.inMu.RLock()
	defer g.inMu.RUnlock()
	if len(p) > 0 && !g.over {
		g.chunks <- append([]byte(nil), p...)
	}
}

// End says the program's output is over: what is held is written, and the
// screen changes no more.
func (g *Grid) End() {
	g.inMu.Lock()
	defer g.inMu.Unlock()
	if !g.over {
		g.over = true
		close(g.chunks)
	}
}

func (g *Grid) feed() {
	defer g.wg.Done()
	defer close(g.fed)
	var h holder
	timer := time.NewTimer(holdFor)
	timer.Stop()
	for {
		var expired <-chan time.Time
		if h.holding() {
			expired = timer.C
		}
		select {
		case chunk, ok := <-g.chunks:
			if !ok {
				g.commit(h.release(), true)
				return
			}
			g.commit(h.take(chunk), false)
			if h.holding() {
				timer.Reset(holdFor)
			}
		case <-expired:
			g.commit(h.release(), false)
		}
	}
}

// commit writes one piece of output into the emulator, whole, and only then
// wakes the readers of the screen.
func (g *Grid) commit(p []byte, end bool) {
	if len(p) == 0 && !end {
		return
	}
	g.mu.Lock()
	if len(p) > 0 {
		g.emu.write(p)
		g.dirty = true
	}
	g.streamLocked(p, end)
	if end {
		g.ended = true
	}
	g.wakeLocked()
	g.mu.Unlock()
}

func (g *Grid) wakeLocked() {
	close(g.wake)
	g.wake = make(chan struct{})
}

func (g *Grid) drain() {
	defer g.wg.Done()
	defer close(g.answers)
	r := g.emu.replies()
	buf := make([]byte, replyRead)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			select {
			case g.answers <- append([]byte(nil), buf[:n]...):
			default:
			}
		}
		if err != nil {
			return
		}
	}
}

// Answers are the emulator's answers to the program's queries — a cursor
// report, the device's attributes — for whoever answers the program; closed
// once the grid is.
func (g *Grid) Answers() <-chan []byte { return g.answers }

// Resize sizes the grid. The caller sizes the terminal after it: the program
// draws for its new size only once the kernel tells it, and by then the grid
// has that size. The library does not reflow; a new size republishes every
// row.
func (g *Grid) Resize(size Size) error {
	if err := size.check(); err != nil {
		return err
	}
	g.mu.Lock()
	g.emu.resize(size)
	g.dirty = true
	g.wakeLocked()
	g.mu.Unlock()
	return nil
}

// SetDefaults sets the colors the program's queries of the default
// foreground and background are answered with — the person's terminal's, so a
// program choosing a light or dark theme sees the real one; nil keeps one.
func (g *Grid) SetDefaults(fg, bg color.Color) {
	g.mu.Lock()
	g.emu.defaults(fg, bg)
	g.mu.Unlock()
}

// Screen is the screen for a reader that has the sequence after: at once
// when it changed since, when after is a sequence this grid never gave, or
// when the output is over; otherwise when it changes, or ctx ends, and then
// with the same sequence and no rows.
func (g *Grid) Screen(ctx context.Context, after uint64) Screen {
	for {
		g.mu.Lock()
		wake, ended := g.wake, g.ended
		g.mu.Unlock()
		s := g.publish(after)
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

func (g *Grid) publish(after uint64) Screen {
	g.pubMu.Lock()
	defer g.pubMu.Unlock()
	g.mu.Lock()
	fresh := g.dirty || g.pub.seq == 0
	var l look
	if fresh {
		l = g.emu.look()
		g.dirty = false
	}
	g.mu.Unlock()
	if fresh {
		g.pub.take(l)
	}
	return g.pub.since(after)
}

// Close ends the output, if not yet, writes what is held, then stops the
// answers and joins every goroutine.
func (g *Grid) Close() error {
	var err error
	g.once.Do(func() {
		g.End()
		<-g.fed
		if stop := g.emu.stopReplies(); stop != nil && !errors.Is(stop, io.ErrClosedPipe) {
			err = stop
		}
		g.wg.Wait()
	})
	return err
}
