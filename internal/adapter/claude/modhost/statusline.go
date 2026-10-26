package modhost

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// fromHarnessOrChild is fromHarness widened to the harness's descendants: the
// status line is a command Claude Code starts, not the harness's own process.
// Only this endpoint takes it — its payload is telemetry, and a tool call of
// the harness posting it is harmless.
func (s *Server) fromHarnessOrChild(r *http.Request) error {
	if s.fromHarness(r.Context()) == nil {
		return nil
	}
	p, _ := r.Context().Value(peerKey{}).(peer)
	if p.err != nil || p.pid == 0 {
		return fmt.Errorf("the connection's peer could not be read: %v", p.err)
	}
	s.mu.Lock()
	harness := s.harness
	s.mu.Unlock()
	if harness.PID == 0 {
		return errors.New("the harness's process is not known to this run yet")
	}
	if start, err := proc.StartTime(harness.PID); err != nil || start != harness.Start {
		return errors.New("the harness's process is gone or was replaced")
	}
	if err := proc.Default.Outside(p.pid, harness.PID); !errors.Is(err, proc.ErrDescendant) {
		return fmt.Errorf("process %d is neither the harness's nor runs below it", p.pid)
	}
	return nil
}

func (s *Server) statusLine(w http.ResponseWriter, r *http.Request) {
	var line StatusLineRequest
	if status, err := decode(w, r, &line, MaxBodyBytes); err != nil {
		refuse(w, status, err.Error())
		return
	}
	if err := s.fromHarnessOrChild(r); err != nil {
		s.refuseEffect(w, http.StatusForbidden, err)
		return
	}
	var life *session.CacheLife
	if line.Warm != nil {
		life = &session.CacheLife{Warm: *line.Warm}
		if line.Ttl != nil && (*line.Ttl == "5m" || *line.Ttl == "1h") {
			life.TTL = *line.Ttl
		}
		if line.ExpiresAt != nil && *line.ExpiresAt > 0 {
			at := time.Unix(int64(*line.ExpiresAt), 0)
			life.ExpiresAt = &at
		}
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		s.refuseEffect(w, http.StatusServiceUnavailable, errClosing)
		return
	}
	if life != nil {
		s.management.observed.life = life
	}
	if line.Effort != nil {
		s.management.observed.effortFrom(&s.management.observed.lineEffort, *line.Effort)
	}
	if line.ModelName != nil && *line.ModelName != "" {
		s.management.observed.modelName = *line.ModelName
	}
	if line.Thinking != nil {
		on := *line.Thinking
		s.management.observed.thinking = &on
	}
	s.mu.Unlock()
	reply(w, http.StatusOK, Done{})
}
