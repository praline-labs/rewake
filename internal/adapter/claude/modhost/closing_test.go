package modhost

import (
	"net/http"
	"sync/atomic"
	"testing"
)

// session/closing tells the run's Closing for an admitted load, each time it
// is said, and is refused for a load the run never admitted: the mod↔host
// contract that starts the shell's closing screen.
func TestSessionClosingTellsTheRunForAnAdmittedLoad(t *testing.T) {
	var told atomic.Int32
	_, socket := servingWith(t, func(s *Server) { s.cfg.Closing = func() { told.Add(1) } })
	closing := []byte(`{"loadId":"load-1","reason":"prompt_input_exit"}`)
	if status, text, err := post(socket, PathSessionClosing, closing); err != nil || status != http.StatusBadRequest || told.Load() != 0 {
		t.Fatalf("a closing before any hello: %d %s %v, told %d", status, text, err, told.Load())
	}
	if status, text, err := post(socket, PathHello, validHello("load-1")); err != nil || status != http.StatusOK {
		t.Fatalf("the hello: %d %s %v", status, text, err)
	}
	for range 2 {
		if status, text, err := post(socket, PathSessionClosing, closing); err != nil || status != http.StatusOK {
			t.Fatalf("the closing: %d %s %v", status, text, err)
		}
	}
	if told.Load() != 2 {
		t.Fatalf("Closing was told %d times for two closings", told.Load())
	}
}
