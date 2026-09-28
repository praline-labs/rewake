package gateway

// The compaction mark: set when a compaction's request is sent, holding
// deliveries up to its bound, tied to its turn by the turn's item, and lost
// sight of when the selection changes first (docs/remote-control-codex.md).

import (
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/control"
)

// holding says a compaction mark on thread still holds deliveries.
func (a *admittedWork) holding(thread string) bool {
	marker := a.manual[thread]
	return marker != nil && !marker.released
}

func (a *admittedWork) manualStart(thread string, selected observer, sent, generation uint64, asked time.Time) bool {
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
	a.manual[thread] = &manualWork{before: before, sent: sent, asked: asked, generation: generation}
	return true
}

// finish ends the wait for the compaction's end, once.
func (m *manualWork) finish(status, failure string) {
	if m.ended == nil || m.answered {
		return
	}
	m.answered = true
	m.status, m.failure = status, failure
	close(m.ended)
}

// running is the status of a mark whose wait ended with its compaction still
// running: no turn ends with it.
const running = "running"

// stillRunning ends the wait for the compaction's end with it still running,
// for the reason given: main is told so, and told its end when it is seen.
func (m *manualWork) stillRunning(reason string) {
	m.late = m.by != ""
	m.finish(running, reason)
}

// outcome is main's answer from how the mark's wait ended.
func (m *manualWork) outcome() control.Answer {
	switch m.status {
	case "completed":
		answer := control.Answer{Outcome: control.Done, TokensAfter: m.after}
		if m.after != nil {
			answer.TokensBefore = m.tokensBefore
		}
		return answer
	case "interrupted":
		return control.Answer{Outcome: control.Failed, Detail: "the compaction was interrupted"}
	case running:
		// Started is no final outcome: main's wrapper waits on for the end
		// (internal/wrap, compaction_letters.go).
		return control.Answer{Outcome: control.Started, Detail: m.failure + "; it may still be running, and its end is reported when seen"}
	}
	return control.Answer{Outcome: control.Failed, Detail: m.failure}
}

// manualTurn ties the compaction to its turn: main's letter and the
// telemetry's author go by that turn's end.
func (a *admittedWork) manualTurn(thread, turn string) {
	marker := a.manual[thread]
	marker.turn = turn
	if marker.tied != nil {
		close(marker.tied)
		marker.tied = nil
	}
}
