package codex

import (
	"context"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

// controlPoll is how often the wrapper looks for a main's request; the asker
// gives the served side five seconds to take one (docs/remote-control.md).
const controlPoll = 100 * time.Millisecond

// The waits of a request, each inside the asker's own: its outcome limit is
// 90 seconds for a compaction and 10 for an interrupt, and an answer written
// within them says what happened instead of leaving the asker to give up.
var steerWaits = map[string]time.Duration{
	control.Compact:   80 * time.Second,
	control.Interrupt: 8 * time.Second,
}

// steer carries out a main's request on the owned app-server: Codex has no
// plugin, and the wrapper is what holds the connection and knows the turn.
func (s *serverSession) steer(ctx context.Context, request control.Request) control.Answer {
	wait, known := steerWaits[request.Action]
	if !known {
		return control.Answer{Outcome: control.Failed, Detail: "unknown action " + request.Action}
	}
	if s.gateway == nil {
		return control.Answer{Outcome: control.Failed, Detail: "the app-server is not connected yet"}
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	if request.Action == control.Interrupt {
		return s.gateway.Interrupt(ctx, request.From)
	}
	if request.Focus != "" {
		// The command refuses a focus for Codex before sending; one that
		// arrives anyway is not dropped silently.
		return control.Answer{Outcome: control.Failed, Detail: "Codex takes no focus for a compaction"}
	}
	return s.gateway.Compact(ctx, request.ID, request.From)
}
