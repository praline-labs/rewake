package delivery

import (
	"maps"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// A wrapper about to exec a new image of itself asks its server to stop
// (Server.Handoff), the probe of .scratch/reexec-probe.md. The server stops
// only when nothing it holds in memory alone would be lost — a held notice,
// one a late word may still take back, a letter of this run still waiting —
// and then answers its Memory without refusing anything: the next image
// serves the same mailbox from disk, as a server's begin always does, and
// takes the reminder's memory from the handover. Otherwise it answers why and
// serves on.

// Memory is the reminder's state (reminder.go), which a handoff carries to
// the next image of the same run: its letters announced and not yet looked
// at by a finished turn's end, and the latest end looked at. Both are boot-
// clock times, which the exec does not change. An announced letter does not
// keep the run from a switch: a turn that ended in an error or was
// interrupted looks at nothing, and the run would wait for its next finished
// turn (the owner's decision of November 2, 2026, docs/rules/state.md).
type Memory struct {
	Announced map[string]int64 `json:"announced,omitempty"`
	Looked    int64            `json:"looked,omitempty"`
}

// Stop is the server's answer to a handoff: why it serves on, or, empty,
// the memory it hands over.
type Stop struct {
	Why    string
	Memory Memory
}

// busy answers what this server holds that the disk does not and a handoff
// cannot carry, "" when nothing.
func (s *Server) busy() string {
	switch {
	case len(s.held) > 0:
		return "the harness holds a notice whose receipt only this image would take"
	case len(s.recent) > 0:
		return "a notice reported delivered may still be taken back by a late word"
	}
	messages, err := mail.List(s.Dir, s.Name)
	if err != nil {
		return "the mailbox could not be read: " + err.Error()
	}
	for _, message := range messages {
		if s.Epoch == "" || message.ToEpoch == s.Epoch {
			return "a letter waits for its delivery"
		}
	}
	return ""
}

// handoff answers a request to stop: true stops the server. What it hands
// over is also what its begin takes again when the exec fails and it serves
// on.
func (s *Server) handoff(reply chan<- Stop) bool {
	if why := s.busy(); why != "" {
		reply <- Stop{Why: why}
		return false
	}
	memory := Memory{Announced: maps.Clone(s.announced), Looked: s.looked}
	s.Remembered = &memory
	reply <- Stop{Memory: Memory{Announced: maps.Clone(memory.Announced), Looked: memory.Looked}}
	return true
}

// remembered starts the reminder's state from the previous image's.
func (s *Server) remembered() {
	s.announced = map[string]int64{}
	if s.Remembered != nil {
		maps.Copy(s.announced, s.Remembered.Announced)
		s.looked = max(s.looked, s.Remembered.Looked)
	}
}
