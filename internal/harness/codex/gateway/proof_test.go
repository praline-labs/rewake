package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

// nothingPublished says no outcome was published for a while.
func nothingPublished(t *testing.T, out chan Completion) {
	t.Helper()
	select {
	case v := <-out:
		t.Fatalf("published %+v", v)
	case <-time.After(700 * time.Millisecond):
	}
}

// publishedAs waits for the outcome of turn and checks its kind and text.
func publishedAs(t *testing.T, out chan Completion, id, kind, text string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case v := <-out:
			if v.ID != id {
				continue
			}
			if v.Kind != kind || v.Text != text {
				t.Fatalf("published %+v; want %s %q", v, kind, text)
			}
			return
		case <-deadline:
			t.Fatalf("%s was not published", id)
		}
	}
}

// userItem is the input a turn of the terminal's or a delivery's records at
// its start.
func userItem(turn string) string {
	return `{"method":"item/completed","params":{"threadId":"A","turnId":"` + turn + `","item":{"id":"u-` + turn + `","type":"userMessage","content":[]}}}`
}

func answerItem(turn, text string) string {
	return `{"method":"item/completed","params":{"threadId":"A","turnId":"` + turn + `","item":{"id":"i-` + turn + `","type":"agentMessage","text":"` + text + `"},"completedAtMs":2}}`
}

// A compaction stopped before its item — by Esc, or by a PreCompact hook that
// stops it, which the server runs after the turn started and before the item
// (core/src/compact.rs) — shows no proof of work, and nothing shows it was a
// compaction either: it is reported only as advisory, which settles nothing.
func TestACompactionStoppedBeforeItsItemIsOnlyAdvisory(t *testing.T) {
	hook := []string{
		`{"method":"hook/started","params":{"threadId":"A","turnId":"C","run":{"id":"h1","eventName":"preCompact","status":"running"}}}`,
		`{"method":"hook/completed","params":{"threadId":"A","turnId":"C","run":{"id":"h1","eventName":"preCompact","status":"stopped"}}}`,
	}
	for _, variant := range []string{"main's, by Esc", "main's, by its hook", "the terminal's, by Esc"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			out := callbacks(g)
			if variant == "the terminal's, by Esc" {
				exchange(t, ui, native, `{"id":8,"method":"thread/compact/start","params":{"threadId":"A"}}`, `{"id":8,"result":{}}`)
			} else {
				_ = steerAnswer(func() control.Answer { return g.Compact(context.Background(), "0123", "lead") })
				id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
				write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			}
			events(t, ui, native, started("C"))
			if variant == "main's, by its hook" {
				events(t, ui, native, hook...)
			}
			events(t, ui, native, completed("C", "interrupted"))
			publishedAs(t, out, "A/C/advisory", "stopped", advisoryText+"; the person at the keyboard stopped this turn")
		})
	}
}

// A goal's turn takes the mark and ends; the compaction asked for then runs
// with no mark left. It shows no proof of work, and is never published.
func TestACompactionAfterAGoalTookItsMarkIsNeverPublished(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	out := callbacks(g)
	answers := steerAnswer(func() control.Answer { return g.Compact(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	events(t, ui, native, started("U"), compactionItem("U", "item/started"),
		`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"U","status":"failed","items":[],"error":{"message":"goal pre-compaction failed"}}}}`)
	_ = within(t, answers)
	events(t, ui, native, started("C"), compactionItem("C", "item/started"), compactionItem("C", "item/completed"), completed("C", "completed"))
	nothingPublished(t, out)
}

// reconnect closes the terminal's connection and opens the next one, resumed
// on A with status.
func reconnect(t *testing.T, g *Gateway, ui, native *socketClient, peers chan *socketClient, path, status string) (*socketClient, *socketClient) {
	t.Helper()
	_ = ui.conn.Close()
	_ = native.conn.Close()
	awaitReleased(t, g)
	next, err := dialSocket(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = next.conn.Close() })
	peer := <-peers
	t.Cleanup(func() { _ = peer.conn.Close() })
	exchange(t, next, peer, `{"id":0,"method":"initialize"}`, `{"id":0,"result":{}}`)
	exchange(t, next, peer, `{"id":1,"method":"thread/resume","params":{"threadId":"A","excludeTurns":true}}`, `{"id":1,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"`+status+`","activeFlags":[]},"turns":[]}}}`)
	return next, peer
}

// A compaction accepted on one connection may run on the next, where no mark
// was ever set. It shows no proof of work, and is never published; when its
// turn is never named, it is a gap, reported only as advisory.
func TestACompactionThatOutlivesTheConnectionIsNeverPublished(t *testing.T) {
	for _, variant := range []string{"its item unseen", "its item seen on the first", "its end unseen"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, path := setup(t)
			native := <-peers
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			out := callbacks(g)
			answers := steerAnswer(func() control.Answer {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				return g.Compact(ctx, "0123", "lead")
			})
			id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
			write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			if variant == "its item seen on the first" {
				events(t, ui, native, started("C"), compactionItem("C", "item/started"))
			}
			next, peer := reconnect(t, g, ui, native, peers, path, "active")
			_ = within(t, answers)
			events(t, next, peer, compactionItem("C", "item/completed"),
				`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`)
			if variant == "its end unseen" {
				publishedAs(t, out, "A/gap-1", "stopped", gapText)
				return
			}
			events(t, next, peer, completed("C", "completed"))
			nothingPublished(t, out)
		})
	}
}

// A turn of the terminal's, a delivery's or a review's is named by its reply,
// and reports in every order of the reply and its events: also when it has no
// item, and also when the reply comes after its end.
func TestWorkTurnsReportInEveryOrder(t *testing.T) {
	orders := map[string][]string{
		"reply first":            {"reply", "started", "answer", "completed"},
		"reply after its start":  {"started", "reply", "answer", "completed"},
		"reply after its end":    {"started", "answer", "completed", "reply"},
		"stopped, no item":       {"reply", "started", "stopped"},
		"stopped, reply at last": {"started", "stopped", "reply"},
	}
	for _, source := range []string{"the terminal's", "a delivery's", "a review's"} {
		for name, order := range orders {
			t.Run(source+", "+name, func(t *testing.T) {
				g, ui, peers, _ := setup(t)
				native := <-peers
				defer func() { _ = native.conn.Close() }()
				out := callbacks(g)
				bindUI(t, g, ui, native)
				var reply func()
				switch source {
				case "the terminal's":
					write(t, ui, []byte(`{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
					_ = readWithin(t, native)
					reply = func() {
						write(t, native, []byte(`{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`))
						_ = readWithin(t, ui)
					}
				case "a review's":
					write(t, ui, []byte(`{"id":30,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"}}}`))
					_ = readWithin(t, native)
					reply = func() {
						write(t, native, []byte(`{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"},"reviewThreadId":"A"}}`))
						_ = readWithin(t, ui)
					}
				default:
					delivered := make(chan error, 1)
					go func() {
						_, err := g.Deliver(context.Background(), g.Binding(), "message-id", "fixture notice")
						delivered <- err
					}()
					var start struct{ ID string }
					_ = json.Unmarshal(readWithin(t, native), &start)
					reply = func() {
						write(t, native, []byte(`{"id":"`+start.ID+`","result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`))
						if err := <-delivered; err != nil {
							t.Fatalf("delivery: %v", err)
						}
					}
				}
				kind, text := "finished", "the report"
				for _, step := range order {
					switch step {
					case "reply":
						reply()
					case "started":
						events(t, ui, native, started("U"))
					case "answer":
						events(t, ui, native, answerItem("U", "the report"))
					case "completed":
						events(t, ui, native, completed("U", "completed"))
					case "stopped":
						events(t, ui, native, completed("U", "interrupted"))
						kind, text = "stopped", "the person at the keyboard stopped this turn"
					}
				}
				publishedAs(t, out, "A/U", kind, text)
			})
		}
	}
}

// The reply that names a turn proves it on any connection: a turn of the
// terminal's that ends on the next connection with nothing but its end still
// reports.
func TestATurnNamedOnOneConnectionReportsOnTheNext(t *testing.T) {
	g, ui, peers, path := setup(t)
	native := <-peers
	bindUI(t, g, ui, native)
	out := callbacks(g)
	exchange(t, ui, native, `{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`)
	events(t, ui, native, started("U"))
	next, peer := reconnect(t, g, ui, native, peers, path, "active")
	events(t, next, peer, `{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`, completed("U", "interrupted"))
	publishedAs(t, out, "A/U", "stopped", "the person at the keyboard stopped this turn")
}

// A detached review runs in a conversation of its own; the reply names its
// turn there, and that proves it.
func TestADetachedReviewsReplyProvesItsTurn(t *testing.T) {
	c := &connection{admitted: newAdmittedWork(), state: newState("epoch", 1)}
	raw := []byte(`{"id":5,"result":{"reviewThreadId":"R","turn":{"id":"T","items":[],"status":"inProgress"}}}`)
	c.replied(meta{id: "5", turn: "T"}, raw, pending{method: "review/start", target: "A"}, true, admittedRequest{}, false)
	if !c.admitted.proven.has("R/T") || c.admitted.proven.has("A/T") {
		t.Fatal("the review's turn was not proven in its own conversation")
	}
}
