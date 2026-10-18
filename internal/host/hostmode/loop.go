package hostmode

import (
	"time"

	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/host/terminal"
)

// opened is a space's view the poll connected, for a tab.
type opened struct {
	worker spacehost.Worker
	view   *spacehost.View
}

// poll lists main's spaces once a second: a view for each new one, and word
// of each that went. A space whose view does not connect yet, still opening,
// is tried again at the next poll.
func (h *Host) poll() {
	known := map[string]bool{}
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()
	for {
		if workers, err := h.cfg.Panel.Workers(); err == nil {
			listed := map[string]bool{}
			for _, w := range workers {
				listed[w.Space] = true
				if known[w.Space] {
					continue
				}
				view, err := h.cfg.Panel.View(w.Space)
				if err != nil {
					continue
				}
				select {
				case h.opened <- opened{worker: w, view: view}:
					known[w.Space] = true
				case <-h.ctx.Done():
					_ = view.Close()
					return
				}
			}
			for id := range known {
				if listed[id] {
					continue
				}
				select {
				case h.gone <- id:
					delete(known, id)
				case <-h.ctx.Done():
					return
				}
			}
		}
		select {
		case <-tick.C:
		case <-h.ctx.Done():
			return
		}
	}
}

// loop is the host's one goroutine of state: every tab, the input automaton
// and the canvas are touched here alone.
func (h *Host) loop() {
	defer close(h.done)
	// A panic leaves the person's terminal raw and on the alternate screen
	// with nobody to give it back: restore it on the way out.
	defer func() {
		if r := recover(); r != nil {
			h.leave()
			panic(r)
		}
	}()
	keyTimer := time.NewTimer(time.Hour)
	keyTimer.Stop()
	drawTimer := time.NewTimer(0)
	drawing := true
	keys := h.keys
	for {
		select {
		case <-h.ctx.Done():
			return
		case u := <-h.updates:
			h.apply(u)
		case data := <-keys:
			h.dispatch(h.in.feed(data, time.Now()))
		case <-keyTimer.C:
			h.dispatch(h.in.tick(time.Now()))
		case <-h.winch:
			h.resize()
		case o := <-h.opened:
			h.add(o)
		case id := <-h.gone:
			h.remove(id)
		case <-drawTimer.C:
			drawing = false
			h.draw()
		}
		if at := h.in.deadline(); at.IsZero() {
			keyTimer.Stop()
		} else {
			keyTimer.Reset(max(time.Until(at), 0))
		}
		if h.dirty && !drawing {
			drawTimer.Reset(frameGap)
			drawing = true
		}
	}
}

func (h *Host) apply(u update) {
	before := h.label(u.tab)
	u.tab.pic = u.pic
	if u.tab == h.cur || h.label(u.tab) != before {
		h.dirty = true
	}
}

func (h *Host) dispatch(actions []action) {
	for _, a := range actions {
		switch {
		case a.command != 0:
			h.command(a.command)
		case a.mouseRow > 0 && !h.lay.inRegion(a.mouseRow):
			// On the bar: the host's, never the program's.
		case len(a.send) > 0:
			h.cur.send(a.send)
		}
	}
}

// command runs a key typed after the prefix. A digit naming no tab, and the
// sidebar's key before there is a sidebar, do nothing.
func (h *Host) command(c byte) {
	at := 0
	for i, t := range h.tabs {
		if t == h.cur {
			at = i
		}
	}
	switch {
	case c == 'n':
		h.show(h.tabs[(at+1)%len(h.tabs)])
	case c == 'p':
		h.show(h.tabs[(at+len(h.tabs)-1)%len(h.tabs)])
	case c == 'm' || c == '0':
		h.show(h.tabs[0])
	case c >= '1' && c <= '9' && int(c-'0') < len(h.tabs):
		h.show(h.tabs[c-'0'])
	}
}

func (h *Host) show(t *tab) {
	if t == h.cur {
		return
	}
	h.cur = t
	h.canvas.Redraw()
	h.dirty = true
}

// resize follows the person's terminal: the canvas, then every program to
// the new region, so a switch never has to tell a program a new size.
func (h *Host) resize() {
	size, err := terminal.SizeOf(h.cfg.Out)
	if err != nil || size == h.lay.size {
		return
	}
	h.lay = layoutFor(size)
	h.canvas.Resize(size)
	for _, t := range h.tabs {
		t.size(h.lay.region)
	}
	h.dirty = true
}

func (h *Host) add(o opened) {
	view := o.view
	t := newTab(o.worker.Session, o.worker.Space, view.Keys, view.Resize, func() { _ = view.Close() })
	t.size(h.lay.region)
	h.watch(t, view)
	h.dirty = true
}

func (h *Host) remove(space string) {
	for i, t := range h.tabs {
		if t.space != space || i == 0 {
			continue
		}
		if t == h.cur {
			h.show(h.tabs[0])
		}
		t.close()
		h.tabs = append(h.tabs[:i:i], h.tabs[i+1:]...)
		h.dirty = true
		return
	}
}

func (h *Host) label(t *tab) label {
	return label{name: t.name, current: t == h.cur, ended: t.pic.Ended, changed: t != h.cur && t.pic.Seq != t.seen}
}

func (h *Host) draw() {
	h.dirty = false
	labels := make([]label, len(h.tabs))
	for i, t := range h.tabs {
		labels[i] = h.label(t)
	}
	pic := h.cur.pic
	h.cur.seen = pic.Seq
	modes := pic.Modes
	if pic.Ended {
		modes = terminal.Modes{}
		pic.Cursor.Visible = false
	}
	_ = h.canvas.Draw(terminal.Frame{
		Region: h.lay.region, Rows: pic.Rows, Cursor: pic.Cursor, Modes: modes,
		Bar: barRuns(labels, h.lay.size.Cols),
	})
}
