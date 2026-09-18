package gateway

import "time"

// Completion carries only live outcomes. No resume/history item can reach this path.
type (
	Completion struct {
		ReadThrough            *uint64
		ID, Thread, Kind, Text string
		Epoch                  string
		Connection, Generation uint64
		Retained               bool
	}
	interval struct {
		endRead                           *uint64
		thread, turn, text                string
		active, done, compact, seenActive bool
		idle                              time.Time
		serial                            uint64
	}
	observer struct {
		thread    string
		watches   map[string]*interval
		manual    map[string]bool
		intervals []*interval
		sequence  uint64
		out       []Completion
		overflow  bool
	}
)

func newObserver() observer {
	return observer{watches: map[string]*interval{}, manual: map[string]bool{}}
}
func (o *observer) reset() { *o = newObserver() }
func (o *observer) allocate(thread string) *interval {
	if len(o.intervals) >= 64 {
		removed := false
		for i, w := range o.intervals {
			if w.done {
				o.intervals = append(o.intervals[:i], o.intervals[i+1:]...)
				if o.watches[w.thread] == w {
					delete(o.watches, w.thread)
				}
				removed = true
				break
			}
		}
		if !removed {
			o.overflow = true
			return nil
		}
	}
	o.sequence++
	w := &interval{thread: thread, serial: o.sequence, compact: o.manual[thread]}
	o.intervals = append(o.intervals, w)
	o.watches[thread] = w
	return w
}

func (o *observer) watch(thread string) *interval {
	if w := o.watches[thread]; w != nil {
		return w
	}
	return o.allocate(thread)
}

func (o *observer) find(thread, turn string) *interval {
	for i := len(o.intervals) - 1; i >= 0; i-- {
		w := o.intervals[i]
		if w.thread == thread && w.turn == turn && turn != "" {
			return w
		}
	}
	w := o.watches[thread]
	if w != nil && w.turn == "" && !w.done {
		return w
	}
	return nil
}

func (o *observer) bind(thread, status string, _ time.Time) {
	o.thread = thread
	if status == "active" {
		w := o.watch(thread)
		if w != nil && !w.done {
			w.seenActive = true
			if w.idle.IsZero() {
				w.active = true
			}
		}
	}
	kept := o.intervals[:0]
	for _, w := range o.intervals {
		if w.thread == thread {
			kept = append(kept, w)
		}
	}
	o.intervals = kept
	for key := range o.watches {
		if key != thread {
			delete(o.watches, key)
		}
	}
}

// Refused maintenance never reopens or removes a previous task interval.
func (o *observer) refuseManual(thread string) {
	delete(o.manual, thread)
	for _, w := range o.intervals {
		if w.thread == thread && w.compact {
			w.done = true
			w.active = false
			w.text = ""
		}
	}
}

func (o *observer) event(m meta, raw []byte, now time.Time) {
	if m.thread == "" || o.thread != "" && m.thread != o.thread {
		return
	}
	switch m.method {
	case "thread/status/changed", "turn/started", "turn/completed", "item/started", "item/completed":
	default:
		return
	}
	w := o.watch(m.thread)
	if w == nil {
		return
	}
	switch m.method {
	case "thread/status/changed":
		if m.status == "active" {
			if w.done || w.seenActive && !w.active && !w.idle.IsZero() {
				w = o.allocate(m.thread)
				if w == nil {
					return
				}
			}
			if !w.seenActive && w.turn == "" {
				w.compact = o.manual[m.thread]
			}
			w.active = true
			w.seenActive = true
			w.idle = time.Time{}
			w.endRead = nil
		}
		if m.status == "idle" || m.status == "systemError" {
			w.active = false
			if w.idle.IsZero() {
				w.idle = now
				w.endRead = m.readThrough
			}
		}
	case "turn/started":
		if w.done || w.turn != "" && w.turn != m.turn {
			w = o.allocate(m.thread)
			if w == nil {
				return
			}
		}
		if !w.seenActive && w.turn == "" {
			w.compact = o.manual[m.thread]
		}
		if !w.seenActive {
			w.endRead = nil
		}
		w.turn = m.turn
		w.active = true
		w.seenActive = true
	case "item/started", "item/completed":
		w = o.find(m.thread, m.turn)
		if w == nil {
			return
		}
		if m.method == "item/completed" && str(raw, "params", "item", "type") == "agentMessage" {
			v := field(raw, "params", "item", "text")
			if len(v) <= 1<<20 {
				w.text = decodeText(v)
			}
		}
	case "turn/completed":
		w = o.find(m.thread, m.turn)
		if w == nil || w.done || m.turn == "" {
			return
		}
		w.turn = m.turn
		w.active = false
		w.done = true
		if w.compact {
			delete(o.manual, m.thread)
			return
		}
		result := Completion{ID: m.thread + "/" + m.turn, Thread: m.thread, ReadThrough: endBoundary(w, m)}
		switch m.status {
		case "completed":
			result.Kind = "finished"
			result.Text = w.text
		case "failed":
			result.Kind = "error"
			result.Text = decodeText(field(raw, "params", "turn", "error", "message"))
		case "interrupted":
			result.Kind = "stopped"
			result.Text = "the person at the keyboard stopped this turn"
		default:
			return
		}
		w.text = ""
		o.out = append(o.out, result)
	}
}

func (o *observer) expire(now time.Time) {
	for _, w := range o.intervals {
		if w.thread != o.thread || !w.seenActive || w.done || w.compact || w.active || w.idle.IsZero() || now.Sub(w.idle) < 500*time.Millisecond {
			continue
		}
		w.done = true
		w.text = ""
		id := w.turn
		if id == "" {
			id = "gap-" + itoa(w.serial)
		}
		o.out = append(o.out, Completion{ID: w.thread + "/" + id, Thread: w.thread, Kind: "error", Text: "completion not observed", ReadThrough: w.endRead})
	}
}

func (o *observer) drain() []Completion {
	var out []Completion
	for _, c := range o.out {
		if c.Thread == o.thread {
			out = append(out, c)
		}
	}
	o.out = nil
	return out
}

func endBoundary(w *interval, m meta) *uint64 {
	if w != nil && w.endRead != nil {
		return w.endRead
	}
	return m.readThrough
}
