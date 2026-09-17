package codex

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type serverNotice struct {
	method string
	raw    json.RawMessage
	thread string
}

func (s *serverSession) wakeDiscovery() {
	select {
	case s.discoverWake <- struct{}{}:
	default:
	}
}

// Resume replies go only to the TUI. Global thread-id hints and this startup
// poll discover it without relying on a fabricated thread/started notification.
func (s *serverSession) discover(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.discoverWake:
		case <-ticker.C:
		}
		lookup, cancel := context.WithTimeout(ctx, 2*time.Second)
		_ = s.ensureThread(lookup)
		cancel()
	}
}

func (s *serverSession) ensureThread(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.mu.Lock()
		client := s.client
		ready := client != nil && s.current != "" && !s.dirty && s.discoveryErr == nil
		s.mu.Unlock()
		if ready {
			s.replayDiscovered()
			return nil
		}
		if client == nil {
			return errors.New("connection is not ready")
		}
		if err := s.restore(ctx, client); err != nil {
			return err
		}
		s.mu.Lock()
		dirty, current, active := s.dirty, s.current, s.client == client
		s.mu.Unlock()
		if !active {
			return errors.New("connection changed during discovery")
		}
		if dirty {
			continue
		}
		if current == "" {
			return errors.New("no loaded TUI thread; wait for it to start or resume")
		}
		s.replayDiscovered()
		return nil
	}
}

// Terminal notifications can outrun metadata discovery. Keep them until the
// root identity is verified, then replay only events for that selected thread.
func (s *serverSession) replayDiscovered() {
	s.mu.Lock()
	if s.dirty || s.discoveryErr != nil {
		s.mu.Unlock()
		return
	}
	pending := s.delayed
	s.delayed = nil
	current := s.current
	s.mu.Unlock()
	for _, notice := range pending {
		if notice.thread == current {
			s.event(notice.method, notice.raw)
		}
	}
}
