package modhost

import (
	"testing"

	"github.com/praline-labs/rewake/internal/core/session"
)

// What the launch passed, a sample's request and the status line each reach
// the session's observer, which merges them (internal/core/session): the
// launch's effort until one is seen, the one changed last after; the window
// the launch set below the model's as the limit, the percent of it.
func TestTheLaunchTheSamplesAndTheStatusLineReachTheObserver(t *testing.T) {
	server, socket := servingWith(t, func(s *Server) { s.cfg.Effort, s.cfg.Window = "high", 50_000 })
	if got := server.SessionState(); got.Effort == nil || *got.Effort != "high" || got.EffortSource != session.EffortFromLaunch {
		t.Fatalf("before anything: %v %q", got.Effort, got.EffortSource)
	}
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	sample := testState()
	low := "low"
	sample.Effort = &low
	postValue(t, socket, PathModState, sample)
	got := server.SessionState()
	if got.Effort == nil || *got.Effort != "low" || got.EffortSource != "" {
		t.Fatalf("a request's: %v %q", got.Effort, got.EffortSource)
	}
	if got.ContextWindow == nil || *got.ContextWindow != 50_000 || got.FilledPercent == nil || *got.FilledPercent != 50 {
		t.Fatalf("window %v, percent %v; want 50000 and 50", got.ContextWindow, got.FilledPercent)
	}
	if status, _, err := post(socket, PathStatusLine, []byte(`{"effort":"max"}`)); err != nil || status != 200 {
		t.Fatalf("the status line: %d %v", status, err)
	}
	if got := server.SessionState(); got.Effort == nil || *got.Effort != "max" || got.CacheLife != nil {
		t.Fatalf("the status line's: %v, life %+v", got.Effort, got.CacheLife)
	}
}
