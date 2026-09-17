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

// A TUI detaching from the old root cannot unload it while the observer stays.
func TestSwitchReleasesPreviousObserverSubscription(t *testing.T) {
	var mu sync.Mutex
	subscribed := map[string]bool{}
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
			mu.Lock()
			switch q.Method {
			case "thread/resume":
				if q.Params["excludeTurns"] != true {
					t.Error("requested history")
				}
				subscribed[q.Params["threadId"].(string)] = true
			case "thread/unsubscribe":
				delete(subscribed, q.Params["threadId"].(string))
			}
			mu.Unlock()
			serverMessage(c, map[string]any{"id": q.ID, "result": map[string]any{}})
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	s := newServer(path, nil, nil, "")
	c, err := connectRPC(ctx, path, s.event)
	if err != nil {
		t.Fatal(err)
	}
	defer c.close()
	s.client = c
	s.current = "previous"
	if err = s.resumeSubscription(ctx, c, "previous", s.generation); err != nil {
		t.Fatal(err)
	}
	s.event("thread/started", json.RawMessage(`{"thread":{"id":"replacement","source":"cli","originator":"codex_cli_rs","status":{"type":"active"}}}`))
	s.subscriptionAttempt(ctx)
	mu.Lock()
	defer mu.Unlock()
	if !subscribed["replacement"] {
		t.Fatal("replacement observation missing")
	}
	if subscribed["previous"] {
		t.Fatal("observer retains previous root after TUI detaches; loaded-root discovery becomes ambiguous")
	}
}
