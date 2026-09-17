package codex

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

const (
	subscriptionStep = 50 * time.Millisecond
	completionGrace  = 500 * time.Millisecond
)

type threadStatus struct {
	Kind string `json:"type"`
}

type turnObservation struct {
	serial uint64
	thread string
	turn   string
	active bool
	done   bool
	retry  bool
	idleAt time.Time
}

func noRollout(err error) bool {
	var failure *rpcError
	return errors.As(err, &failure) && failure.Code == -32600 && strings.Contains(failure.Message, "no rollout found")
}

// Identity is enough to deliver the first input. Subscription becomes possible
// only after that input materializes the rollout; never gate delivery on it.
func (s *serverSession) resumeSubscription(ctx context.Context, client *rpcClient, thread string, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.generation != generation {
		s.mu.Unlock()
		return errors.New("thread changed before observer subscription")
	}
	if s.observerSubscriptions == nil {
		s.observerSubscriptions = make(map[observerSubscription]observerLease)
	}
	key := observerSubscription{client: client, thread: thread}
	// A timeout can hide a successful attach. Track the attempt before sending
	// so a later switch can release it even without an acknowledgement.
	s.observerSubscriptions[key] = observerLease{generation: generation}
	s.mu.Unlock()
	if err := client.call(ctx, "thread/resume", map[string]any{"threadId": thread, "excludeTurns": true}, nil); err != nil {
		if noRollout(err) {
			s.mu.Lock()
			delete(s.observerSubscriptions, key)
			s.mu.Unlock()
		}
		closeUncertainObserver(client, err)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.generation == generation {
		s.subscribedClient, s.subscribedThread = client, thread
	}
	s.wakeSubscription()
	return nil
}

// Called under mu. Repeated active updates include flag changes, not new turns.
func (s *serverSession) observeStatus(kind string) {
	switch kind {
	case "active":
		if s.observation == nil || !s.observation.active {
			s.observationSequence++
			s.observation = &turnObservation{serial: s.observationSequence, thread: s.current, active: true, retry: true}
			s.observations = append(s.observations, s.observation)
		}
		s.wakeSubscription()
	case "idle", "systemError":
		if watch := s.observation; watch != nil && watch.active {
			watch.active = false
			watch.idleAt = time.Now()
			s.wakeSubscription()
		}
	}
}

func (s *serverSession) resetObservation() {
	s.observation, s.observations = nil, nil
}

// The status notification precedes the scoped completion in the pinned server.
// Give that completion time to arrive before declaring an observation gap.
func (s *serverSession) expireObservations(now time.Time) {
	pending := s.observations[:0]
	for _, watch := range s.observations {
		if !watch.done && !watch.active && now.Sub(watch.idleAt) >= completionGrace {
			id := watch.turn
			if id == "" {
				id = fmt.Sprintf("unobserved-%d", watch.serial)
			}
			s.queueCompletion(harness.Completion{ID: watch.thread + "/" + id, Thread: watch.thread, Kind: inbox.Error, Text: "completion not observed"})
			watch.done = true
		}
		if !watch.done {
			pending = append(pending, watch)
		}
	}
	s.observations = pending
}

func (s *serverSession) finishObservation(turn string) bool {
	watch := s.observation
	for _, pending := range s.observations {
		if pending.turn == turn {
			watch = pending
			break
		}
	}
	if watch == nil || watch.turn != "" && watch.turn != turn {
		return false
	}
	already := watch.done
	watch.turn, watch.done = turn, true
	return already
}

func (s *serverSession) queueCompletion(result harness.Completion) {
	s.outcomes = append(s.outcomes, result)
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *serverSession) wakeSubscription() {
	select {
	case s.subscriptionWake <- struct{}{}:
	default:
	}
}

func (s *serverSession) subscribe(ctx context.Context) {
	ticker := time.NewTicker(subscriptionStep)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-s.subscriptionWake:
		}
		s.subscriptionAttempt(ctx)
	}
}

func (s *serverSession) subscriptionAttempt(ctx context.Context) {
	// Serialize observer resume calls; each reply is bound to its generation.
	select {
	case s.discoverGate <- struct{}{}:
		defer func() { <-s.discoverGate }()
	default:
		return
	}
	// Release the old observer even when the new thread is idle and has no
	// rollout. Waiting for its next resume would retain the old root forever.
	s.releaseObsoleteSubscriptions(ctx)
	s.mu.Lock()
	s.expireObservations(time.Now())
	watch, client, generation := s.observation, s.client, s.generation
	if client == nil || s.dirty || s.discoveryErr != nil || watch == nil || !watch.active || watch.done || !watch.retry || s.subscribedClient == client && s.subscribedThread == s.current {
		s.mu.Unlock()
		return
	}
	attempt, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	// A lifecycle change stops retries but lets an already sent RPC settle.
	// Canceling it here would make every /new race retire a healthy connection.
	s.mu.Unlock()
	err := s.resumeSubscription(attempt, client, watch.thread, generation)
	cancel()
	s.mu.Lock()
	// Only the documented persistence race is retried within this active turn.
	if err != nil && !noRollout(err) {
		watch.retry = false
	}
	s.mu.Unlock()
}
