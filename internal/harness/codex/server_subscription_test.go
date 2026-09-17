package codex

import (
	"context"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

func statusFixture(t *testing.T, s *serverSession, thread, status string) {
	t.Helper()
	emitFixture(t, s, "thread/status/changed", map[string]any{"threadId": thread, "status": threadStatus{Kind: status}})
}

func beginSubscribedTurn(t *testing.T, s *serverSession, thread string) {
	t.Helper()
	statusFixture(t, s, thread, "active")
	waitSubscription(t, s, thread)
}

func waitSubscription(t *testing.T, s *serverSession, thread string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		ready := s.subscribedThread == thread && s.subscribedClient == s.client
		s.mu.Unlock()
		if ready {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("observer did not subscribe during the turn")
}

func terminalFixture(t *testing.T, s *serverSession, thread, turn string) {
	t.Helper()
	emitFixture(t, s, "turn/completed", map[string]any{"threadId": thread, "turn": map[string]any{"id": turn, "status": "completed", "items": []serverItem{{Kind: "agentMessage", Text: "observed final"}}}})
}

func awaitOutcome(t *testing.T, outcomes <-chan harness.Completion) harness.Completion {
	t.Helper()
	select {
	case result := <-outcomes:
		return result
	case <-time.After(2 * time.Second):
		t.Fatal("no terminal outcome")
	}
	return harness.Completion{}
}

func TestFreshThreadSubscribesAfterRolloutAppears(t *testing.T) {
	t.Setenv("RW_SERVER_ROLLOUT_DELAY", "180ms")
	s, outcomes := runtimeFixture(t)
	for _, thread := range []string{fixtureRoot, "fresh-new"} {
		if thread != fixtureRoot {
			emitFixture(t, s, "thread/started", map[string]any{"thread": serverThread{ID: thread, Source: []byte(`"vscode"`), Originator: "rewake"}})
		}
		// The first notice must reach the model before a rollout can exist.
		if result := s.Deliver(context.Background(), inbox.Message{ID: thread}); result.State != inbox.Delivered {
			t.Fatal(result)
		}
		waitSubscription(t, s, thread)
		statusFixture(t, s, thread, "idle")
		terminalFixture(t, s, thread, "first-turn")
		if result := awaitOutcome(t, outcomes); result.Kind != inbox.Finished || result.Text != "observed final" || result.Thread != thread {
			t.Fatalf("result=%+v", result)
		}
	}
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var info struct {
		Attempts int `json:"resumeAttempts"`
	}
	if err := client.call(ctx, "fixture/inspect", nil, &info); err != nil {
		t.Fatal(err)
	}
	if info.Attempts < 4 {
		t.Fatalf("persistence failures were not retried: %+v", info)
	}
}

func TestShortUnsubscribedTurnReportsObservationGap(t *testing.T) {
	t.Setenv("RW_SERVER_ROLLOUT_DELAY", "1h")
	s, outcomes := runtimeFixture(t)
	statusFixture(t, s, fixtureRoot, "active")
	// This event is lost by the strict fixture: the observer has no subscription.
	terminalFixture(t, s, fixtureRoot, "too-short")
	statusFixture(t, s, fixtureRoot, "idle")
	result := awaitOutcome(t, outcomes)
	if result.Kind != inbox.Error || result.Text != "completion not observed" {
		t.Fatalf("result=%+v", result)
	}
	statusFixture(t, s, fixtureRoot, "idle")
	select {
	case result := <-outcomes:
		t.Fatalf("duplicate missing completion: %+v", result)
	case <-time.After(completionGrace + 2*subscriptionStep):
	}
}

func TestIdleAllowsScopedCompletionBeforeGapReport(t *testing.T) {
	s, outcomes := runtimeFixture(t)
	beginSubscribedTurn(t, s, fixtureRoot)
	statusFixture(t, s, fixtureRoot, "idle")
	time.Sleep(100 * time.Millisecond)
	terminalFixture(t, s, fixtureRoot, "normal")
	if result := awaitOutcome(t, outcomes); result.Kind != inbox.Finished {
		t.Fatalf("result=%+v", result)
	}
	select {
	case result := <-outcomes:
		t.Fatalf("false gap after completed: %+v", result)
	case <-time.After(completionGrace + 2*subscriptionStep):
	}
}

func TestNewThreadCancelsPendingSubscriptionAndGap(t *testing.T) {
	t.Setenv("RW_SERVER_ROLLOUT_DELAY", "1h")
	s, outcomes := runtimeFixture(t)
	statusFixture(t, s, fixtureRoot, "active")
	statusFixture(t, s, fixtureRoot, "idle")
	emitFixture(t, s, "thread/started", map[string]any{"thread": serverThread{ID: "replacement", Source: []byte(`"vscode"`), Originator: "rewake"}})
	select {
	case result := <-outcomes:
		t.Fatalf("old turn reported into new conversation: %+v", result)
	case <-time.After(completionGrace + 2*subscriptionStep):
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observation != nil || s.subscribedThread == "replacement" {
		t.Fatal("new thread inherited old observation/subscription")
	}
}

func TestSubscriptionRetriesStopAtIdleOrPermanentRefusal(t *testing.T) {
	for _, permanent := range []bool{false, true} {
		t.Run(map[bool]string{false: "idle", true: "permanent refusal"}[permanent], func(t *testing.T) {
			t.Setenv("RW_SERVER_ROLLOUT_DELAY", "1h")
			if permanent {
				t.Setenv("RW_SERVER_RESUME_ERROR", "access denied")
			}
			s, _ := runtimeFixture(t)
			statusFixture(t, s, fixtureRoot, "active")
			attempts := func() int {
				t.Helper()
				s.mu.Lock()
				client := s.client
				s.mu.Unlock()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				var result struct {
					Count int `json:"resumeAttempts"`
				}
				if err := client.call(ctx, "fixture/inspect", nil, &result); err != nil {
					t.Fatal(err)
				}
				return result.Count
			}
			deadline := time.Now().Add(time.Second)
			for attempts() == 0 && time.Now().Before(deadline) {
				time.Sleep(subscriptionStep)
			}
			if !permanent {
				statusFixture(t, s, fixtureRoot, "idle")
			}
			time.Sleep(2 * subscriptionStep)
			before := attempts()
			if before == 0 || permanent && before != 1 {
				t.Fatalf("attempts=%d", before)
			}
			time.Sleep(4 * subscriptionStep)
			if after := attempts(); after != before {
				t.Fatalf("unexpected retry: before=%d after=%d", before, after)
			}
		})
	}
}
