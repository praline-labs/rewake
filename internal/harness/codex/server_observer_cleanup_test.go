package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"
)

type cleanupFixture struct {
	mu                sync.Mutex
	subscribed        map[string]bool
	calls             []string
	failUnsubscribe   bool
	beforeResumeReply func()
}

func (f *cleanupFixture) snapshot() (map[string]bool, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	subscriptions := make(map[string]bool)
	for id, on := range f.subscribed {
		subscriptions[id] = on
	}
	return subscriptions, append([]string(nil), f.calls...)
}

func observerFixture(t *testing.T) (*serverSession, *cleanupFixture, context.Context) {
	t.Helper()
	s := newServer("", nil, nil, "")
	f := &cleanupFixture{subscribed: map[string]bool{}}
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		for {
			_, raw, _, err := readClientFrame(r)
			if err != nil {
				return
			}
			var q struct {
				ID     uint64         `json:"id"`
				Method string         `json:"method"`
				Params map[string]any `json:"params"`
			}
			if json.Unmarshal(raw, &q) != nil {
				return
			}
			if q.Method == "initialized" {
				continue
			}
			result := any(map[string]any{})
			var failure bool
			f.mu.Lock()
			id, _ := q.Params["threadId"].(string)
			f.calls = append(f.calls, q.Method+":"+id)
			hook := f.beforeResumeReply
			switch q.Method {
			case "thread/resume":
				if q.Params["excludeTurns"] != true {
					t.Error("history requested")
				}
				f.subscribed[id] = true
			case "thread/unsubscribe":
				failure = f.failUnsubscribe
				if !failure {
					delete(f.subscribed, id)
				}
			case "thread/loaded/list":
				// Both roots remain loaded throughout the upstream unload grace.
				result = map[string]any{"data": []string{"previous", "replacement"}}
			case "thread/read":
				if q.Params["includeTurns"] != false {
					t.Error("history requested")
				}
				result = map[string]any{"thread": serverThread{ID: id, Source: []byte(`"cli"`), Originator: "rewake"}}
			}
			f.mu.Unlock()
			if q.Method == "thread/resume" && hook != nil {
				hook()
			}
			if failure {
				serverMessage(c, map[string]any{"id": q.ID, "error": map[string]any{"code": -32600, "message": "unsubscribe refused"}})
			} else {
				serverMessage(c, map[string]any{"id": q.ID, "result": result})
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	c, err := connectRPC(ctx, path, s.event)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.close)
	s.client = c
	s.current = "previous"
	return s, f, ctx
}

func switchObserverRoot(s *serverSession, id, status string) {
	raw, _ := json.Marshal(map[string]any{"thread": serverThread{ID: id, Source: []byte(`"cli"`), Originator: "rewake", Status: threadStatus{Kind: status}}})
	s.event("thread/started", raw)
}

func TestIdleReplacementReleasesObserverBeforePersistence(t *testing.T) {
	s, f, ctx := observerFixture(t)
	if err := s.resumeSubscription(ctx, s.client, "previous", 0); err != nil {
		t.Fatal(err)
	}
	switchObserverRoot(s, "replacement", "idle")
	// Exercise the actual periodic cleanup, without an active turn or discovery.
	run, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); s.subscribe(run) }()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		subscriptions, _ := f.snapshot()
		if !subscriptions["previous"] {
			break
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	subscriptions, calls := f.snapshot()
	if len(subscriptions) != 0 {
		t.Fatalf("idle switch retained subscriptions: %v", subscriptions)
	}
	for _, call := range calls {
		if call == "thread/resume:replacement" {
			t.Fatal("fresh idle replacement was resumed")
		}
	}
}

func TestLateResumeAndRapidSwitchReleaseEveryObsoleteAttach(t *testing.T) {
	s, f, ctx := observerFixture(t)
	f.mu.Lock()
	f.beforeResumeReply = func() { switchObserverRoot(s, "intermediate", "idle"); switchObserverRoot(s, "replacement", "idle") }
	f.mu.Unlock()
	if err := s.resumeSubscription(ctx, s.client, "previous", 0); err != nil {
		t.Fatal(err)
	}
	s.subscriptionAttempt(ctx)
	subscriptions, _ := f.snapshot()
	if len(subscriptions) != 0 {
		t.Fatalf("late resume leaked: %v", subscriptions)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != "replacement" || s.subscribedThread != "" || len(s.observerSubscriptions) != 0 {
		t.Fatal("late acknowledgement rebound observer")
	}
}

func TestUnsubscribeFailureRetriesWithoutBlockingKnownReplacement(t *testing.T) {
	s, f, ctx := observerFixture(t)
	if err := s.resumeSubscription(ctx, s.client, "previous", 0); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.failUnsubscribe = true
	f.mu.Unlock()
	switchObserverRoot(s, "replacement", "active")
	s.subscriptionAttempt(ctx)
	subscriptions, _ := f.snapshot()
	if !subscriptions["previous"] || !subscriptions["replacement"] {
		t.Fatalf("fixture or replacement subscription failed: %v", subscriptions)
	}
	f.mu.Lock()
	f.failUnsubscribe = false
	f.mu.Unlock()
	s.mu.Lock()
	for key, lease := range s.observerSubscriptions {
		lease.retryAfter = time.Time{}
		s.observerSubscriptions[key] = lease
	}
	s.mu.Unlock()
	s.subscriptionAttempt(ctx)
	subscriptions, _ = f.snapshot()
	if subscriptions["previous"] || !subscriptions["replacement"] {
		t.Fatalf("cleanup lost its retry or detached current: %v", subscriptions)
	}
}

func TestOldRootEventsRefuseAmbiguousDiscovery(t *testing.T) {
	s, f, ctx := observerFixture(t)
	if err := s.resumeSubscription(ctx, s.client, "previous", 0); err != nil {
		t.Fatal(err)
	}
	switchObserverRoot(s, "replacement", "idle")
	s.subscriptionAttempt(ctx)
	for _, method := range []string{"thread/status/changed", "turn/completed", "thread/closed"} {
		s.event(method, json.RawMessage(`{"threadId":"previous","status":{"type":"active"},"turn":{"id":"old","status":"completed"}}`))
	}
	if err := s.restore(ctx, s.client); err == nil {
		t.Fatal("ambiguous roots were selected")
	}
	subscriptions, _ := f.snapshot()
	if len(subscriptions) != 0 {
		t.Fatalf("ambiguous discovery resumed a root: %v", subscriptions)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.outcomes) != 0 {
		t.Fatal("late old result entered reporting")
	}
}

func TestDisconnectedObserverCleanupDoesNotTouchNewConnection(t *testing.T) {
	s, f, ctx := observerFixture(t)
	old := &rpcClient{done: make(chan struct{})}
	close(old.done)
	s.mu.Lock()
	s.observerSubscriptions = map[observerSubscription]observerLease{{client: old, thread: "previous"}: {generation: 0}}
	s.generation++
	s.mu.Unlock()
	s.subscriptionAttempt(ctx)
	_, calls := f.snapshot()
	for _, call := range calls {
		if call == "thread/unsubscribe:previous" {
			t.Fatal("old connection cleanup unsubscribed through new client")
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.observerSubscriptions) != 0 {
		t.Fatal("dead observer retained bookkeeping")
	}
}

func TestResumeFormerRootDoesNotSilentlyKeepReplacement(t *testing.T) {
	s, _, ctx := observerFixture(t)
	if err := s.resumeSubscription(ctx, s.client, "previous", 0); err != nil {
		t.Fatal(err)
	}
	switchObserverRoot(s, "replacement", "idle")
	s.subscriptionAttempt(ctx)
	// A real /resume may only expose global status, without thread/started.
	s.event("thread/status/changed", json.RawMessage(`{"threadId":"previous","status":{"type":"active"}}`))
	if thread, err := s.Thread(); err == nil {
		t.Fatalf("resume of previous root silently kept delivery target %q", thread)
	}
}

func TestCanceledResumeRetiresOnlyUncertainObserverConnection(t *testing.T) {
	s, f, ctx := observerFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	f.mu.Lock()
	f.beforeResumeReply = func() { close(entered); <-release }
	f.mu.Unlock()
	attempt, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- s.resumeSubscription(attempt, s.client, "previous", 0) }()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("resume did not reach server")
	}
	switchObserverRoot(s, "replacement", "idle")
	cancel()
	if err := <-done; err == nil {
		t.Fatal("canceled resume succeeded")
	}
	select {
	case <-s.client.done:
	case <-ctx.Done():
		t.Fatal("uncertain connection remains open for a late attach")
	}
	s.subscriptionAttempt(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.subscribedThread != "" || len(s.observerSubscriptions) != 0 {
		t.Fatal("uncertain observer was retained")
	}
}

func TestSwitchDuringResumeKeepsSettledConnection(t *testing.T) {
	s, f, ctx := observerFixture(t)
	f.mu.Lock()
	f.beforeResumeReply = func() { switchObserverRoot(s, "replacement", "idle") }
	f.mu.Unlock()
	s.mu.Lock()
	s.observeStatus("active")
	s.mu.Unlock()
	s.subscriptionAttempt(ctx)
	select {
	case <-s.client.done:
		t.Fatal("root switch closed a successfully settled observer RPC")
	default:
	}
	s.subscriptionAttempt(ctx)
	subscriptions, _ := f.snapshot()
	if len(subscriptions) != 0 {
		t.Fatalf("late attachment survived cleanup: %v", subscriptions)
	}
}
