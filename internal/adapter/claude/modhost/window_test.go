package modhost

import "testing"

// A window the launch set below the model's is the limit shown, and the
// percent is of it, not of the model's window the mod measured against.
func TestLaunchWindowIsTheLimitAndTheFillIsOfIt(t *testing.T) {
	server, socket := servingWith(t, func(s *Server) { s.cfg.Window = 50_000 })
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	postValue(t, socket, PathModState, testState())
	got := server.SessionState()
	if got.ContextWindow == nil || *got.ContextWindow != 50_000 || got.FilledPercent == nil || *got.FilledPercent != 50 {
		t.Fatalf("window %v, percent %v; want 50000 and 50", got.ContextWindow, got.FilledPercent)
	}
}
