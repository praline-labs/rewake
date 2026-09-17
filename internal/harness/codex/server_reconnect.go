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
	s.releaseObsoleteSubscriptions(ctx)

	s.mu.Lock()
	generation := s.generation
	hints := s.hintSequence
	statusSequence := s.statusSequence
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
	var rootStatus string
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
				rootStatus = response.Thread.Status.Kind
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
		subscribed := s.subscribedThread == thread && s.subscribedClient == client
		s.mu.Unlock()
		if !subscribed {
			if err := s.resumeSubscription(ctx, client, thread, generation); err != nil && !noRollout(err) {
				return err
			}
		}
	}
	s.mu.Lock()
	if s.generation == generation {
		s.selectRoot(thread)
		if rootStatus == "active" && s.statusSequence == statusSequence {
			s.observeStatus(rootStatus)
		}
	}
	s.signal()
	s.mu.Unlock()
	return nil
}
