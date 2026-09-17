package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"
)

func TestSubscriptionReplyCannotBindAfterNewThread(t *testing.T) {
	s := newServer("", nil, nil, "")
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		for {
			_, raw, _, err := readClientFrame(r)
			if err != nil {
				return
			}
			var request struct {
				ID     uint64 `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(raw, &request) != nil {
				return
			}
			if request.Method == "initialized" {
				continue
			}
			if request.Method == "thread/resume" {
				serverMessage(c, map[string]any{"method": "thread/started", "params": map[string]any{"thread": serverThread{ID: "newer", Source: []byte(`"cli"`), Originator: "rewake"}}})
			}
			serverMessage(c, map[string]any{"id": request.ID, "result": map[string]any{}})
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := connectRPC(ctx, path, s.event)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	s.mu.Lock()
	s.client, s.current = client, "previous"
	s.observeStatus("active")
	s.mu.Unlock()
	if err := s.resumeSubscription(ctx, client, "previous", 0); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current != "newer" || s.subscribedClient != nil || s.subscribedThread != "" {
		t.Fatalf("stale subscription: current=%s subscribed=%s", s.current, s.subscribedThread)
	}
}

func TestRestoreActiveSnapshotCannotUndoLaterIdle(t *testing.T) {
	s := newServer("", nil, nil, "")
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		for {
			_, raw, _, err := readClientFrame(r)
			if err != nil {
				return
			}
			var request struct {
				ID     uint64 `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal(raw, &request) != nil {
				return
			}
			var result any = map[string]any{}
			switch request.Method {
			case "initialized":
				continue
			case "thread/loaded/list":
				result = map[string]any{"data": []string{fixtureRoot}}
			case "thread/read":
				result = map[string]any{"thread": serverThread{ID: fixtureRoot, Source: []byte(`"cli"`), Originator: "rewake", Status: threadStatus{Kind: "active"}}}
			case "thread/resume":
				serverMessage(c, map[string]any{"method": "thread/status/changed", "params": map[string]any{"threadId": fixtureRoot, "status": threadStatus{Kind: "idle"}}})
			}
			serverMessage(c, map[string]any{"id": request.ID, "result": result})
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := connectRPC(ctx, path, s.event)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	s.mu.Lock()
	s.client, s.current = client, fixtureRoot
	s.observeStatus("active")
	s.mu.Unlock()
	if err := s.restore(ctx, client); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.observation == nil || s.observation.active || s.observation.idleAt.IsZero() {
		t.Fatal("older active snapshot restarted observation after idle")
	}
}

func TestLaterCompletionDoesNotConsumeEarlierObservationGap(t *testing.T) {
	s := newServer("", nil, nil, "")
	s.current = fixtureRoot
	status := func(kind string) {
		raw, _ := json.Marshal(map[string]any{"threadId": fixtureRoot, "status": threadStatus{Kind: kind}})
		s.event("thread/status/changed", raw)
	}
	status("active")
	status("idle") // First turn ended before the observer subscribed.
	status("active")
	started, _ := json.Marshal(map[string]any{"threadId": fixtureRoot, "turn": map[string]string{"id": "second"}})
	s.event("turn/started", started)
	status("idle")
	completed, _ := json.Marshal(map[string]any{"threadId": fixtureRoot, "turn": map[string]any{"id": "second", "status": "completed", "items": []serverItem{{Kind: "agentMessage", Text: "second result"}}}})
	s.event("turn/completed", completed)
	s.expireObservations(time.Now().Add(completionGrace))
	if len(s.outcomes) != 2 || s.outcomes[0].Text != "second result" || s.outcomes[1].Text != "completion not observed" || s.outcomes[1].ID == fixtureRoot+"/second" {
		t.Fatalf("later result consumed the earlier gap: %+v", s.outcomes)
	}
}
