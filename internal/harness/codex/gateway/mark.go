package gateway

// The compaction mark: set when a compaction's request is sent, holding
// deliveries up to its bound, tied to its turn by the turn's item, and lost
// sight of when the selection changes first (docs/remote-control-codex.md).

import (
	"strings"
	"time"
)

// holding says a compaction mark on thread still holds deliveries.
func (a *admittedWork) holding(thread string) bool {
	marker := a.manual[thread]
	return marker != nil && !marker.released
}

func (a *admittedWork) manualStart(thread string, selected observer, sent, generation uint64, hold time.Time) bool {
	if a.manual[thread] == nil && len(a.manual) >= 64 {
		return false
	}
	before := map[string]bool{}
	for _, w := range selected.intervals {
		if w.thread == thread && w.turn != "" {
			before[w.turn] = true
		}
	}
	if s := a.threads[thread]; s != nil {
		for id := range s.turns {
			before[id] = true
		}
	}
	for id, old := range a.stopped {
		if old.binding.Thread == thread {
			before[strings.TrimPrefix(id, thread+"/")] = true
		}
	}
	a.manual[thread] = &manualWork{before: before, sent: sent, hold: hold, generation: generation}
	return true
}

// finish answers main's wait for the compaction, once.
func (m *manualWork) finish(status, failure string) {
	if m.ended == nil || m.answered {
		return
	}
	m.answered = true
	m.status, m.failure = status, failure
	close(m.ended)
}

// manualTurn ties the compaction to its turn: main's answer and the
// telemetry's author go by that turn's end.
func (a *admittedWork) manualTurn(thread, turn string) {
	a.manual[thread].turn = turn
}
