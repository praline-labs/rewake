package codex

import (
	"context"
	"errors"
	"time"
)

type observerSubscription struct {
	client *rpcClient
	thread string
}

type observerLease struct {
	generation uint64
	retryAfter time.Time
}

// Caller holds mu. Selection changes wake cleanup even without an active turn.
// Old roots stay eligible for discovery: /resume can return to one without a
// thread/started event, and that ambiguity must invalidate the cached target.
func (s *serverSession) selectRoot(thread string) {
	if s.current == thread {
		return
	}
	s.messages = make(map[string]string)
	s.resetObservation()
	s.current = thread
	s.wakeSubscription()
}

// Caller holds discoverGate, shared with resume and discovery. Unsubscribe is
// connection-scoped: it releases only this observer, never the TUI or its thread.
func (s *serverSession) releaseObsoleteSubscriptions(ctx context.Context) {
	s.mu.Lock()
	obsolete := make([]observerSubscription, 0)
	for key, lease := range s.observerSubscriptions {
		if lease.generation != s.generation || key.thread != s.current {
			if !time.Now().Before(lease.retryAfter) {
				obsolete = append(obsolete, key)
			}
		}
	}
	s.mu.Unlock()
	for _, key := range obsolete {
		// A closed observer connection already releases its server subscriptions.
		var err error
		select {
		case <-key.client.done:
		default:
			attempt, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
			err = key.client.call(attempt, "thread/unsubscribe", map[string]any{"threadId": key.thread}, nil)
			cancel()
			closeUncertainObserver(key.client, err)
		}
		s.mu.Lock()
		if s.subscribedClient == key.client && s.subscribedThread == key.thread {
			s.subscribedClient = nil
			s.subscribedThread = ""
		}
		if err == nil {
			delete(s.observerSubscriptions, key)
		} else {
			lease := s.observerSubscriptions[key]
			lease.retryAfter = time.Now().Add(250 * time.Millisecond)
			s.observerSubscriptions[key] = lease
		}
		s.mu.Unlock()
		if err != nil && s.note != nil {
			s.note("could not release obsolete observer subscription: " + err.Error())
		}
	}
}

// A canceled RPC can still attach after a subsequent unsubscribe has completed.
// Retiring this observer connection fences that late server-side operation;
// the TUI has its own connection and the normal reconnect path restores ours.
func closeUncertainObserver(client *rpcClient, err error) {
	var refusal *rpcError
	if err != nil && !errors.As(err, &refusal) {
		_ = client.socket.conn.Close()
	}
}
