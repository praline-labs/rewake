package gateway

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The mail tool's part of the gateway (docs/mail-bridge-server.md#what-the-wrapper-matches):
// only the primary thread's ends are captured through the gate, and the
// completion carries the boundary and the moment the capture took; only that
// thread's turns and tool items reach the wrapper.
func TestTheMailToolSeesOnlyThePrimaryThread(t *testing.T) {
	var reads atomic.Uint64
	reads.Store(1)
	var captures atomic.Int32
	var mu sync.Mutex
	var events []string
	out := make(chan Completion, 4)
	g, ui, peers, _ := setupConfig(t, Config{
		ReadSequence: reads.Load,
		EndCapture: func() (uint64, int64) {
			captures.Add(1)
			return 7, 4242
		},
		ToolEvent: func(raw []byte) {
			mu.Lock()
			events = append(events, string(raw))
			mu.Unlock()
		},
		Complete: func(c Completion) { out <- c },
	})
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	for _, raw := range []string{
		`{"method":"turn/started","params":{"threadId":"B","turn":{"id":"U"}}}`,
		`{"method":"item/completed","params":{"threadId":"B","turnId":"U","item":{"type":"mcpToolCall","id":"call-b"}}}`,
		`{"method":"turn/completed","params":{"threadId":"B","turn":{"id":"U","status":"completed"}}}`,
		`{"method":"turn/started","params":{"threadId":"A","turn":{"id":"T"}}}`,
		`{"method":"item/started","params":{"threadId":"A","turnId":"T","item":{"type":"agentMessage","id":"words"}}}`,
		`{"method":"item/completed","params":{"threadId":"A","turnId":"T","item":{"type":"mcpToolCall","id":"call-a"}}}`,
	} {
		write(t, native, []byte(raw))
		_ = readWithin(t, ui)
	}
	if n := captures.Load(); n != 0 {
		t.Fatalf("another thread's end was captured %d times", n)
	}
	write(t, native, []byte(`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"T","status":"completed"}}}`))
	_ = readWithin(t, ui)
	deadline := time.After(2 * time.Second)
	for {
		select {
		case result := <-out:
			if result.Thread != "A" {
				continue
			}
			if result.ReadThrough == nil || *result.ReadThrough != 7 || result.Ended != 4242 {
				t.Fatalf("the completion does not carry the capture: %+v", result)
			}
			if n := captures.Load(); n != 1 {
				t.Fatalf("the primary end was captured %d times", n)
			}
			mu.Lock()
			defer mu.Unlock()
			joined := strings.Join(events, "\n")
			if len(events) != 3 || strings.Contains(joined, `"B"`) || strings.Contains(joined, "agentMessage") || !strings.Contains(joined, "call-a") {
				t.Fatalf("the tool's events: %q", events)
			}
			return
		case <-deadline:
			t.Fatal("the primary completion was lost")
		}
	}
}
