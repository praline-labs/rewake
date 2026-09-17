package codex

import (
	"context"
	"fmt"
)

// A disconnect may hide /new and the old thread's closure. Discover loaded root
// threads before rejoining; resuming the cached id could resurrect a closed one.
func (s *serverSession) restore(ctx context.Context, client *rpcClient) (resultErr error) {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case s.discoverGate <- struct{}{}:
	}
	defer func() { <-s.discoverGate }()

	s.mu.Lock()
	generation := s.generation
	hints := s.hintSequence
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.generation == generation {
			s.discoveryErr = resultErr
			if s.hintSequence == hints {
				s.dirty = false
			}
			s.signal()
		}
	}()
	var roots []string
	cursor := ""
	for {
		var list struct {
			Data []string `json:"data"`
			Next *string  `json:"nextCursor"`
		}
		params := map[string]any{"limit": 100}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := client.call(ctx, "thread/loaded/list", params, &list); err != nil {
			return err
		}
		for _, id := range list.Data {
			var response struct {
				Thread serverThread `json:"thread"`
			}
			if err := client.call(ctx, "thread/read", map[string]any{"threadId": id, "includeTurns": false}, &response); err != nil {
				return err
			}
			if tuiThread(response.Thread) {
				roots = append(roots, id)
			} else {
				s.mu.Lock()
				s.ignored[id] = true
				s.mu.Unlock()
			}
		}
		if list.Next == nil || *list.Next == "" {
			break
		}
		cursor = *list.Next
	}
	s.mu.Lock()
	pending := s.delayed[:0]
	for _, notice := range s.delayed {
		if !s.ignored[notice.thread] {
			pending = append(pending, notice)
		}
	}
	s.delayed = pending
	s.mu.Unlock()
	if len(roots) > 1 {
		return fmt.Errorf("cannot identify the TUI among %d loaded root threads", len(roots))
	}
	s.mu.Lock()
	changed := s.generation != generation
	s.mu.Unlock()
	if changed {
		return nil
	}
	thread := ""
	if len(roots) == 1 {
		thread = roots[0]
		s.mu.Lock()
		subscribed := s.current == thread && s.client == client
		s.mu.Unlock()
		if !subscribed {
			if err := client.call(ctx, "thread/resume", map[string]any{"threadId": thread, "excludeTurns": true}, nil); err != nil {
				return err
			}
		}
	}
	s.mu.Lock()
	if s.generation == generation {
		s.current = thread
	}
	s.signal()
	s.mu.Unlock()
	return nil
}
