package gateway

import "github.com/iiiokojiadbi/rewake/internal/boottime"

// An explicit terminal event matching an acknowledged turn does not require a
// preceding turn/started event. Missing start evidence must not hide real results.
func (a *admittedWork) confirmedTerminal(s *admittedThread, m meta, raw []byte) {
	v := Completion{ID: m.thread + "/" + m.turn, Thread: m.thread, Ended: boottime.Now()}
	w := s.observed.find(m.thread, m.turn)
	v.ReadThrough = endBoundary(w, m)
	if w != nil && w.turn == m.turn {
		v.Started = w.started
	}
	switch m.status {
	case "completed":
		v.Kind = "finished"
		if w != nil {
			v.Text = w.text
		}
	case "failed":
		v.Kind = "error"
		v.Text = decodeText(field(raw, "params", "turn", "error", "message"))
	case "interrupted":
		v.Kind = "stopped"
		v.Text = "the person at the keyboard stopped this turn"
	default:
		return
	}
	if w != nil {
		w.turn = m.turn
		w.done = true
		w.active = false
		w.text = ""
	}
	s.observed.out = append(s.observed.out, v)
}
