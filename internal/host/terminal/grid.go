package terminal

import (
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
	// carryFor is how long the cluster a grid of an earlier image held waits
	// for the first output this grid takes: the program's bytes written
	// meanwhile wait in the kernel's buffer and come with the first read.
	carryFor = time.Second
	// replyRead is the most one read of the emulator's answers takes.
	replyRead = 4096
	// replyQueue bounds the answers waiting to be taken. Answers beyond it
	// are dropped rather than stop the emulator in the middle of a write.
	replyQueue = 64
	// seeded follows a seed's answers on the reply pipe, which hands each
	// write to one read whole and never joins two; the emulator never
	// answers with a NUL.
	seeded = "\x00rewake: seeded\x00"
)

// Grid is the screen a program draws, kept by the emulator from the
// program's output as its reader hands it over. It never touches the
// pseudo-terminal: whoever reads the master writes to the grid, sizes it
// before the terminal, and decides where the emulator's answers go.
//
// One goroutine writes each piece of output whole into the emulator under
// the lock before a stream attaches or takes it, since the emulator's
// callbacks run inside the write and a preamble between them sees half a
// frame;
// another takes the emulator's answers off its pipe at once, so the emulator
// never waits on whoever takes them.
type Grid struct {
	// mu guards the emulator and what is said of it.
	mu    sync.Mutex
	emu   emulator
	ended bool
	// streams are the raw streams attached (stream.go), and unfinished the
	// bytes of a sequence the program has not finished, which each new one
	// ends its preamble with.
	streams    map[*Stream]struct{}
	unfinished unfinished

	// inMu orders a write with the end of the output: a write after it is
	// dropped, never sent on a closed channel.
	inMu    sync.RWMutex
	over    bool
	seeding bool
	// keep says the output ends for a handover (Stop): what is held is
	// kept, not written, and carried the held bytes a grid begins with.
	keep, carried []byte
	keeping       bool
	chunks        chan []byte
	answers       chan []byte
	fed           chan struct{}
	wg            sync.WaitGroup
	once          sync.Once
}

// NewGrid is the screen of a program on a terminal of size.
func NewGrid(size Size) (*Grid, error) { return newGrid(size, false, nil) }

// NewSeededGrid is the screen of a program handed over by an image before
// this one, seeded with what that image's grid had written (Seed) and
// holding what it held back (Stop), which joins the program's next output as
// it would have there. The seed's writing answers nothing: those answers, an
// in-band size report among them, were the old image's to give, and the
// program heard them or never asked.
func NewSeededGrid(size Size, seed, held []byte) (*Grid, error) {
	g, err := newGrid(size, true, held)
	if err != nil {
		return nil, err
	}
	g.commit(seed, false)
	g.emu.inject([]byte(seeded))
	return g, nil
}

func newGrid(size Size, seeding bool, held []byte) (*Grid, error) {
	if err := size.check(); err != nil {
		return nil, err
	}
	emu, err := newEmulator(size)
	if err != nil {
		return nil, err
	}
	g := &Grid{
		emu: emu, seeding: seeding, chunks: make(chan []byte, 64),
		answers: make(chan []byte, replyQueue), fed: make(chan struct{}),
		carried: append([]byte(nil), held...),
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
	h := holder{held: g.carried}
	timer := time.NewTimer(carryFor)
	if !h.holding() {
		timer.Stop()
	}
	for {
		var expired <-chan time.Time
		if h.holding() {
			expired = timer.C
		}
		select {
		case chunk, ok := <-g.chunks:
			if !ok {
				if g.keeping {
					g.keep = h.release()
				}
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
// hands it to the streams.
func (g *Grid) commit(p []byte, end bool) {
	if len(p) == 0 && !end {
		return
	}
	g.mu.Lock()
	if len(p) > 0 {
		g.emu.write(p)
	}
	g.streamLocked(p, end)
	if end {
		g.ended = true
	}
	g.mu.Unlock()
}

func (g *Grid) drain() {
	defer g.wg.Done()
	defer close(g.answers)
	r := g.emu.replies()
	buf := make([]byte, replyRead)
	skipping := g.seeding
	for {
		n, err := r.Read(buf)
		if n > 0 && skipping {
			skipping = string(buf[:n]) != seeded
		} else if n > 0 {
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

// Seed is what a grid of the next image is seeded with (NewSeededGrid): a
// raw stream's preamble, taken once the grid is closed, so it holds the last
// of the output.
func (g *Grid) Seed() []byte {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append(g.emu.replay(true), g.unfinished.bytes()...)
}

// Size is the grid's size.
func (g *Grid) Size() Size {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.emu.size()
}

// Resize sizes the grid. The caller sizes the terminal after it: the program
// draws for its new size only once the kernel tells it, and by then the grid
// has that size. The library does not reflow.
func (g *Grid) Resize(size Size) error {
	if err := size.check(); err != nil {
		return err
	}
	g.mu.Lock()
	g.emu.resize(size)
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

// Keep says the end of the output, whoever ends it, keeps what is held back
// rather than write it, for Stop to give to a handover: the reader that is
// stopped for one ends the output itself.
func (g *Grid) Keep() {
	g.inMu.Lock()
	g.keeping = !g.over
	g.inMu.Unlock()
}

// Stop is Close for a handover, after Keep: what was held back is answered,
// not written, for the next image's grid to begin with (NewSeededGrid). A
// mark that follows it in the program's next output joins it there, as it
// would have here; written at the end, the cluster was split and the mark
// lost (found in review, November 2, 2026).
func (g *Grid) Stop() ([]byte, error) {
	err := g.Close()
	return g.keep, err
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
