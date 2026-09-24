package gateway

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

const userAnswer = `{"method":"item/completed","params":{"threadId":"A","turnId":"U","item":{"id":"i1","type":"agentMessage","text":"user answer"},"completedAtMs":2}}`

// An ordinary turn compacts inside itself with the same contextCompaction item
// a manual compaction sends — before its input when the context is full at
// its start, or midway. A mark still waiting for its compaction's turn never
// keeps such a turn: one a reply named, or one with any other item, is work,
// even when the proof comes after its compaction item.
func TestAnAutoCompactingTurnIsNeverTheMarks(t *testing.T) {
	for _, variant := range []string{"reply first", "item before the reply", "a review's, item before the reply", "a goal's, compacting first", "a goal's, compacting midway"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			g.markHold = time.Second
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			out := callbacks(g)
			answers := steerAnswer(func() control.Answer { return g.Compact(context.Background(), "0123", "lead") })
			id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
			write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			compaction := []string{compactionItem("U", "item/started"), compactionItem("U", "item/completed")}
			published := true
			switch variant {
			case "reply first":
				exchange(t, ui, native, `{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`)
				events(t, ui, native, append(append([]string{started("U")}, compaction...), userAnswer, completed("U", "completed"))...)
			case "item before the reply":
				write(t, ui, []byte(`{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
				_ = readWithin(t, native)
				events(t, ui, native, append([]string{started("U")}, compaction...)...)
				write(t, native, []byte(`{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`))
				_ = readWithin(t, ui)
				events(t, ui, native, completed("U", "completed"))
				published = false
			case "a review's, item before the reply":
				write(t, ui, []byte(`{"id":30,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"},"delivery":"inline"}}`))
				_ = readWithin(t, native)
				events(t, ui, native, compaction...)
				write(t, native, []byte(`{"id":30,"result":{"reviewThreadId":"A","turn":{"id":"U","items":[],"status":"inProgress"}}}`))
				_ = readWithin(t, ui)
				events(t, ui, native, completed("U", "completed"))
				published = false
			case "a goal's, compacting first":
				events(t, ui, native, append(append([]string{started("U")}, compaction...), userAnswer, completed("U", "completed"))...)
			case "a goal's, compacting midway":
				events(t, ui, native, append(append([]string{started("U"), userAnswer}, compaction...), completed("U", "completed"))...)
			}
			if answer := within(t, answers); answer.Outcome == control.Done {
				t.Fatalf("main was told the turn's own compaction was its: %+v", answer)
			}
			if variant == "a goal's, compacting first" || variant == "a goal's, compacting midway" {
				// No answer came after the compaction was sent, and its end
				// was not read: it is still open.
				refusedAsUncertain(t, g, native, "after the goal's turn")
			}
			if !published {
				return
			}
			select {
			case v := <-out:
				if v.ID != "A/U" || v.Text != "user answer" {
					t.Fatalf("published %+v", v)
				}
			case <-time.After(time.Second):
				t.Fatal("the turn's report was not published")
			}
		})
	}
}

// A mark tied to its compaction's turn stays with it: another turn's
// compaction item, after the terminal came back and found the conversation
// idle, does not move it.
func TestATiedMarkNeverMoves(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold = time.Second
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	answers := steerAnswer(func() control.Answer { return g.Compact(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	events(t, ui, native, started("C"), compactionItem("C", "item/started"))
	leaveAndReturn(t, ui, native, "idle", "")
	events(t, ui, native, started("U"), compactionItem("U", "item/started"), compactionItem("U", "item/completed"), completed("U", "completed"))
	if answer := within(t, answers); answer.Outcome == control.Done {
		t.Fatalf("main was told another turn's compaction was its: %+v", answer)
	}
}

// An operation accepted past what the record holds cannot be told apart from
// the others once they end, so it leaves the conversation uncertain for the
// rest of the connection.
func TestAnOperationPastTheRecordsCapacityKeepsTheConversationUncertain(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	for i := 1; i <= 65; i++ {
		exchange(t, ui, native,
			fmt.Sprintf(`{"id":%d,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"},"delivery":"inline"}}`, 100+i),
			fmt.Sprintf(`{"id":%d,"result":{"reviewThreadId":"A","turn":{"id":"R%d","items":[],"status":"inProgress"}}}`, 100+i, i))
	}
	resumeA(t, ui, native, "idle", "")
	for i := 1; i <= 64; i++ {
		events(t, ui, native, completed(fmt.Sprintf("R%d", i), "completed"))
	}
	refusedAsUncertain(t, g, native, "with the 65th review not seen to end")
}

// Main's wait is shorter than the bound: the hold ends with it, and the answer
// says the compaction's turn was never seen rather than that it was started.
func TestMainsWaitEndsTheHoldAndSaysWhatWasSeen(t *testing.T) {
	for _, seen := range []bool{false, true} {
		t.Run(fmt.Sprint("turn seen ", seen), func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			answers := steerAnswer(func() control.Answer { return g.Compact(ctx, "0123", "lead") })
			id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
			write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			want := "the compaction's turn was not seen to start when the wait ended; deliveries go on, and its turn is still not taken for work if it starts"
			if seen {
				events(t, ui, native, started("C"), compactionItem("C", "item/started"))
				want = "the compaction was started and had not ended when the wait did; it may still finish, and deliveries go on meanwhile"
			}
			if answer := within(t, answers); answer.Outcome != control.Failed || answer.Detail != want {
				t.Fatalf("main's answer %+v", answer)
			}
			if err := reserveWithin(g, 100*time.Millisecond); err != nil {
				t.Fatalf("a delivery after main's wait: %v", err)
			}
		})
	}
}
