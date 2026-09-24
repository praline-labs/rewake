package gateway

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

const lostWant = "the gateway lost sight of the compaction (the terminal left the conversation before it started)"

// The terminal leaves the conversation before the compaction's turn was seen:
// the stream that would show its item has a gap, so the mark never ties to a
// turn. Main is answered at once, deliveries go on, and the conversation stays
// uncertain — also when a goal's turn then compacts first and fails with no
// other item, which would otherwise pass for main's compaction.
func TestAMarkLosesSightWhenTheTerminalLeavesBeforeItsTurn(t *testing.T) {
	for _, variant := range []string{"then idle", "then a goal's turn fails at its compaction"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			g.markHold = 5 * time.Second
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			answers := steerAnswer(func() control.Answer { return g.Compact(context.Background(), "0123", "lead") })
			id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
			write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			leaveAndReturn(t, ui, native, "idle", "")
			if answer := within(t, answers); answer.Outcome != control.Failed || answer.Detail != lostWant {
				t.Fatalf("main's answer %+v", answer)
			}
			if err := reserveWithin(g, 200*time.Millisecond); err != nil {
				t.Fatalf("a delivery after main's answer: %v", err)
			}
			if variant == "then a goal's turn fails at its compaction" {
				events(t, ui, native, started("U"), compactionItem("U", "item/started"),
					`{"method":"error","params":{"threadId":"A","turnId":"U","error":{"message":"goal pre-compaction failed"},"willRetry":false}}`,
					`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"U","status":"failed","items":[],"error":{"message":"goal pre-compaction failed"}}}}`)
			}
			refusedAsUncertain(t, g, native, "after coming back")
		})
	}
}

// A compaction lost sight of may still run when the terminal comes back, and
// its turn may show nothing but its end. No turn not shown to be work settles
// anything: one its item shows to be a compaction publishes nothing, and one
// that shows nothing is only advisory. A turn shown to be work — by its item
// or by a reply — still reports.
func TestALostCompactionsTurnSettlesNothing(t *testing.T) {
	for _, variant := range []string{"its item after coming back", "only its end", "active with no turns", "after the terminal's own compaction"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			g.markHold = 5 * time.Second
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			out := callbacks(g)
			answers := steerAnswer(func() control.Answer { return g.Compact(context.Background(), "0123", "lead") })
			id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
			write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			switch variant {
			case "active with no turns":
				leaveAndReturn(t, ui, native, "active", "")
			case "after the terminal's own compaction":
				// The terminal's mark takes the lost one's place and ends with
				// its turn; the lost compaction still comes after.
				leaveAndReturn(t, ui, native, "idle", "")
				exchange(t, ui, native, `{"id":8,"method":"thread/compact/start","params":{"threadId":"A"}}`, `{"id":8,"result":{}}`)
				events(t, ui, native, started("K"), compactionItem("K", "item/started"), compactionItem("K", "item/completed"), completed("K", "completed"),
					`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"active","activeFlags":[]}}}`, started("C"))
			default:
				leaveAndReturn(t, ui, native, "active", `{"id":"C","items":[],"status":"inProgress"}`)
			}
			if variant == "its item after coming back" {
				events(t, ui, native, compactionItem("C", "item/completed"))
			}
			events(t, ui, native, `{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`, completed("C", "completed"))
			_ = within(t, answers)
			if variant == "its item after coming back" {
				nothingPublished(t, out)
			} else {
				// Only its end: nothing shows it was a compaction.
				publishedAs(t, out, "A/C/advisory", "stopped", advisoryText)
			}
			// A goal's turn shows it is work by its item, a turn of the
			// terminal's by its reply.
			events(t, ui, native, started("G"),
				`{"method":"item/completed","params":{"threadId":"A","turnId":"G","item":{"id":"i0","type":"agentMessage","text":"goal work"},"completedAtMs":2}}`,
				completed("G", "completed"))
			select {
			case v := <-out:
				if v.ID != "A/G" || v.Text != "goal work" {
					t.Fatalf("published %+v", v)
				}
			case <-time.After(time.Second):
				t.Fatal("the goal's turn was not published")
			}
			exchange(t, ui, native, `{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":30,"result":{"turn":{"id":"V","items":[],"status":"inProgress"}}}`)
			events(t, ui, native, started("V"),
				`{"method":"item/completed","params":{"threadId":"A","turnId":"V","item":{"id":"i1","type":"agentMessage","text":"work"},"completedAtMs":2}}`,
				completed("V", "completed"))
			select {
			case v := <-out:
				if v.ID != "A/V" || v.Text != "work" {
					t.Fatalf("published %+v", v)
				}
			case <-time.After(time.Second):
				t.Fatal("the work turn's report was not published")
			}
		})
	}
}

// A mark tied to a turn later shown to be work leaves no author on that
// turn's compaction: the author is written only when the turn the mark is
// still tied to ends.
func TestAnUntiedMarkLeavesNoAuthorOnTheTurnsCompaction(t *testing.T) {
	for _, proof := range []string{"reply", "item"} {
		t.Run(proof, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			g.markHold = time.Second
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			answers := steerAnswer(func() control.Answer { return g.Compact(context.Background(), "0123", "lead") })
			id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
			write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			if proof == "reply" {
				write(t, ui, []byte(`{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
				_ = readWithin(t, native)
			}
			events(t, ui, native, started("U"), compactionItem("U", "item/started"), compactionItem("U", "item/completed"))
			if proof == "reply" {
				write(t, native, []byte(`{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`))
				_ = readWithin(t, ui)
			}
			events(t, ui, native, userAnswer, completed("U", "completed"))
			if answer := within(t, answers); answer.Outcome == control.Done {
				t.Fatalf("main was told the turn's own compaction was its: %+v", answer)
			}
			snapshot := assertCompactions(t, g, 1)
			if last := snapshot.CompactionEvents[len(snapshot.CompactionEvents)-1]; last.RequestedBy != "" || last.Request != "" {
				t.Fatalf("the turn's compaction was counted for main: %+v", last)
			}
		})
	}
}

// Main's wait ends while the compaction request has no reply: the hold ends
// with main's answer, as with any wait that ends first.
func TestMainsWaitWithoutAReplyEndsTheHold(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold = 5 * time.Second
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	answers := steerAnswer(func() control.Answer { return g.Compact(ctx, "0123", "lead") })
	_ = injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	if answer := within(t, answers); answer.Outcome != control.Failed {
		t.Fatalf("main's answer %+v", answer)
	}
	if err := reserveWithin(g, 500*time.Millisecond); err != nil {
		t.Fatalf("a delivery after main's answer: %v", err)
	}
}

// The compaction ends as main's wait does: the answer is by its end, not
// that it had not ended.
func TestACompactionEndedAsTheWaitEndsIsAnsweredByItsEnd(t *testing.T) {
	c := &connection{admitted: newAdmittedWork(), state: newState("epoch", 1)}
	ended := make(chan struct{})
	marker := &manualWork{before: map[string]bool{}, turn: "C", by: "lead", ended: ended}
	c.admitted.manual["A"] = marker
	c.admitted.event(meta{method: "turn/completed", thread: "A", turn: "C", status: "completed"}, []byte(`{}`), time.Now())
	if answer := c.waitEnded(marker); answer.Outcome != control.Done {
		t.Fatalf("main's answer %+v", answer)
	}
}

// A turn whose end comes before the reply naming it, while a mark waits for
// its compaction's item: the turn compacts first and fails. Its report is
// still published — the terminal's turn and a delivery's alike.
func TestATurnEndedBeforeItsReplyStillReports(t *testing.T) {
	for _, variant := range []string{"the terminal's", "a delivery's"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			g.markHold = 200 * time.Millisecond
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			out := callbacks(g)
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			answers := steerAnswer(func() control.Answer { return g.Compact(ctx, "0123", "lead") })
			_ = injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
			_ = within(t, answers)
			time.Sleep(300 * time.Millisecond)
			var reply func()
			if variant == "the terminal's" {
				write(t, ui, []byte(`{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
				_ = readWithin(t, native)
				reply = func() {
					write(t, native, []byte(`{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`))
					_ = readWithin(t, ui)
				}
			} else {
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
			events(t, ui, native, started("U"), compactionItem("U", "item/started"), compactionItem("U", "item/completed"),
				`{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"U","items":[],"status":"failed","error":{"message":"U's own error"}}}}`)
			reply()
			deadline := time.After(time.Second)
			for {
				select {
				case v := <-out:
					if v.ID == "A/U" {
						if v.Kind != "error" || v.Text != "U's own error" {
							t.Fatalf("published %+v", v)
						}
						return
					}
				case <-deadline:
					t.Fatal("the turn's report was not published")
				}
			}
		})
	}
}
