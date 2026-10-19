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

// poll lists main's spaces once a second: a view for each new one, word of
// each that went, and what each says for the sidebar. A space whose view
// does not connect yet, still opening, is tried again at the next poll.
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
			select {
			case h.listed <- workers:
			case <-h.ctx.Done():
				return
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
	if len(h.early) > 0 {
		h.dispatch(h.in.feed(h.early, time.Now()))
	}
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
		case workers := <-h.listed:
			h.refresh(workers)
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

// apply takes a tab's new screen. Its events reach the person's terminal
// whichever tab is shown: a worker's bell or notification is for the person,
// and a copy comes from where the person's mouse is.
func (h *Host) apply(u update) {
	for _, e := range u.pic.Events {
		_ = h.canvas.Event(e)
	}
	u.pic.Events = nil
	before := h.label(u.tab)
	u.tab.pic = u.pic
	if h.shown(u.tab) || h.label(u.tab) != before {
		h.dirty = true
	}
}

func (h *Host) dispatch(actions []action) {
	for _, a := range actions {
		switch {
		case a.command != 0:
			h.command(a.command)
		case a.mouse != nil:
			h.mouse(*a.mouse)
		case a.paste == pasteOpens:
			h.framed = h.focus.modes().BracketedPaste
			if h.framed {
				h.focus.send(pasteStart)
			}
		case a.paste == pasteCloses:
			if h.framed {
				h.focus.send(pasteEnd)
			}
		case len(a.send) > 0:
			h.focus.send(a.send)
		}
	}
}

// resize follows the person's terminal: the canvas, then every program to
// its new region.
func (h *Host) resize() {
	size, err := terminal.SizeOf(h.cfg.Out)
	if err != nil || size == h.lay.size {
		return
	}
	h.canvas.Resize(size)
	h.relayout(size)
}

func (h *Host) add(o opened) {
	view := o.view
	t := newTab(o.worker.Session, o.worker.Space, view.Keys, view.Resize, func() { _ = view.Close() })
	t.info = o.worker
	h.watch(t, view)
	if h.chosen == nil {
		h.chosen = t
	}
	h.relayout(h.lay.size)
}

// remove drops a space's tab; the right half shows the next worker, or main
// takes the whole width when none is left.
func (h *Host) remove(space string) {
	for i, t := range h.tabs {
		if t.space != space || i == 0 {
			continue
		}
		t.close()
		h.tabs = append(h.tabs[:i:i], h.tabs[i+1:]...)
		if h.grab == t {
			h.grab = nil
		}
		if h.chosen == t {
			h.chosen = nil
			if len(h.tabs) > 1 {
				h.chosen = h.tabs[min(i, len(h.tabs)-1)]
			}
		}
		if h.focus == t {
			h.focus = h.tabs[0]
		}
		h.canvas.Redraw()
		h.relayout(h.lay.size)
		return
	}
}

// refresh takes what the poll read of each worker, for its tab's mark.
func (h *Host) refresh(workers []spacehost.Worker) {
	for _, w := range workers {
		for _, t := range h.tabs {
			if t.space == w.Space && t.info != w {
				before := h.label(t)
				t.info = w
				h.dirty = h.dirty || h.label(t) != before
			}
		}
	}
}
