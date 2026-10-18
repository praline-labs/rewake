package hostmode

import (
	"context"
	"time"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// settle is how long main's screen is left to finish a frame before it is
// taken, as a space host's view waits before its frame: a program hides the
// cursor, draws and shows it again within a few milliseconds, and a screen
// taken at the first change shows it hidden.
const settle = 16 * time.Millisecond

// source gives a program's screen, whole, once its sequence passes after.
type source interface {
	Picture(ctx context.Context, after uint64) (terminal.Picture, error)
}

// tab is one program the host shows: main or a space's worker.
type tab struct {
	name string
	// space is the worker's space id, empty for main.
	space string
	pic   terminal.Picture
	// seen is the sequence last shown, which tells the bar's change mark.
	seen uint64
	// ops are the tab's writes — keys and sizes — done in order by a
	// goroutine of its own, so a program that stops reading never holds
	// the host.
	ops     chan func()
	keys    func([]byte) error
	resize  func(terminal.Size) error
	release func()
	cancel  context.CancelFunc
}

// opsQueue is how many writes a tab keeps waiting before the host waits on
// them: a paste is a write per read, far below it.
const opsQueue = 1024

func newTab(name, space string, keys func([]byte) error, resize func(terminal.Size) error, release func()) *tab {
	t := &tab{name: name, space: space, ops: make(chan func(), opsQueue), keys: keys, resize: resize, release: release}
	go func() {
		for op := range t.ops {
			op()
		}
	}()
	return t
}

func (t *tab) send(p []byte) { t.ops <- func() { _ = t.keys(p) } }

func (t *tab) size(s terminal.Size) { t.ops <- func() { _ = t.resize(s) } }

// update is a tab's new screen.
type update struct {
	tab *tab
	pic terminal.Picture
}

// watch hands each new screen of src to the host until ctx ends or the
// program's screen ends.
func (t *tab) watch(ctx context.Context, src source, updates chan<- update) {
	var after uint64
	for {
		pic, err := src.Picture(ctx, after)
		if ctx.Err() != nil {
			return
		}
		// A view whose space host went is shown as ended: nothing will
		// change it again.
		pic.Ended = pic.Ended || err != nil
		if pic.Seq != after || pic.Ended {
			select {
			case updates <- update{tab: t, pic: pic}:
			case <-ctx.Done():
				return
			}
			after = pic.Seq
		}
		if pic.Ended || err != nil {
			return
		}
	}
}

// close stops the tab's watch and writes, then lets go of its program.
func (t *tab) close() {
	t.cancel()
	close(t.ops)
	if t.release != nil {
		t.release()
	}
}

// gridSource is main's screen, kept from its local grid.
type gridSource struct {
	grid *terminal.Grid
	pic  terminal.Picture
}

func (g *gridSource) Picture(ctx context.Context, after uint64) (terminal.Picture, error) {
	s := g.grid.Screen(ctx, after)
	if after != 0 && s.Seq != after && !s.Ended {
		select {
		case <-ctx.Done():
		case <-time.After(settle):
		}
		s = g.grid.Screen(ctx, after)
	}
	g.pic.Apply(s)
	return g.pic, nil
}
