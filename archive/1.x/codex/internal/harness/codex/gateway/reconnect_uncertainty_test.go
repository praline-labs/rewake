package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/control"
)

// The server keeps an operation it accepted when the terminal disconnects, and
// runs it later. What is open outlives the connection: after a reconnect and an
// idle resume main's compaction is still refused, until the operation's end or
// the answer to a later turn is read on the next connection.
func TestAnOperationAcceptedBeforeAReconnectKeepsTheConversationUncertain(t *testing.T) {
	for _, variant := range []string{"a review, to its end", "a review, a later turn", "a turn whose reply was lost, a later turn", "a turn whose reply was lost, a delivery"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, path := setup(t)
			native := <-peers
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			if strings.HasPrefix(variant, "a turn whose reply was lost") {
				write(t, ui, []byte(`{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
				_ = readWithin(t, native)
			} else {
				exchange(t, ui, native, `{"id":30,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"},"delivery":"inline"}}`, `{"id":30,"result":{"reviewThreadId":"A","turn":{"id":"R","items":[],"status":"inProgress"}}}`)
			}
			next, peer := reconnect(t, g, ui, native, peers, path, "idle")
			exchange(t, next, peer, `{"id":31,"method":"thread/goal/get","params":{"threadId":"A"}}`, `{"id":31,"result":{}}`)
			refusedAsUncertain(t, g, peer, "after the reconnect")
			switch variant {
			case "a review, to its end":
				events(t, next, peer, started("R"), completed("R", "completed"))
			case "a turn whose reply was lost, a delivery":
				// A message delivered is one of the ways out the refusal names.
				delivered := make(chan error, 1)
				go func() {
					_, err := g.Deliver(context.Background(), g.Binding(), "message-id", "fixture notice")
					delivered <- err
				}()
				var start struct{ ID string }
				_ = json.Unmarshal(readWithin(t, peer), &start)
				write(t, peer, []byte(`{"id":"`+start.ID+`","result":{"turn":{"id":"D","items":[],"status":"inProgress"}}}`))
				if err := <-delivered; err != nil {
					t.Fatalf("delivery: %v", err)
				}
				events(t, next, peer, started("D"), completed("D", "completed"))
			default:
				exchange(t, next, peer, `{"id":32,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":32,"result":{"turn":{"id":"V","items":[],"status":"inProgress"}}}`)
				events(t, next, peer, started("V"), completed("V", "completed"))
			}
			_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0124", "lead") })
			injected(t, peer, "thread/compact/start", map[string]string{"threadId": "A"})
		})
	}
}

// The refusal names the ways out that work: a message delivered or a turn typed
// at the terminal, which the server answers, and the TUI. An operation that
// could not be recorded, with operations open in 64 conversations, has no way
// out but the wrapper's end, and its refusal says so.
func TestTheUncertaintyRefusalNamesItsWayOut(t *testing.T) {
	for _, way := range []string{"message delivered", "turn typed at the terminal", "TUI"} {
		if !strings.Contains(uncertainDetail, way) {
			t.Errorf("%q does not name %q", uncertainDetail, way)
		}
	}
	c := &connection{admitted: newAdmittedWork(), state: newState("epoch", 1)}
	for i := range 64 {
		c.state.ops.opened("T"+itoa(uint64(i)), uint64(i+1), "")
	}
	if why := c.busyWith("T0"); why != uncertainDetail {
		t.Fatalf("with one open: %q", why)
	}
	c.state.ops.opened("B", 100, "")
	for _, thread := range []string{"B", "C"} {
		if why := c.busyWith(thread); why != untrackedDetail {
			t.Fatalf("%s past the record: %q", thread, why)
		}
	}
	if !strings.Contains(untrackedDetail, "until this session's wrapper ends") {
		t.Fatalf("%q does not say how long", untrackedDetail)
	}
}
