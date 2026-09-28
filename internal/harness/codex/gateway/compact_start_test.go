package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/control"
)

// startAnswer is what Compact answers at once, with what carries it on to
// its end.
type startAnswer struct {
	answer control.Answer
	later  func() control.Answer
}

func compactStart(g *Gateway, start time.Duration) <-chan startAnswer {
	out := make(chan startAnswer, 1)
	go func() {
		answer, later := g.Compact(context.Background(), "0123", "lead", start)
		out <- startAnswer{answer, later}
	}()
	return out
}

func startWithin(t *testing.T, answers <-chan startAnswer) startAnswer {
	t.Helper()
	select {
	case got := <-answers:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("no answer")
	}
	return startAnswer{}
}

// The answer comes when the compaction's own item ties the mark to its turn,
// before the turn ends; the end, with the counts, is what later waits for, and
// it is kept in the snapshot for main's letter.
func TestACompactionIsAnsweredWhenItStarts(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	events(t, ui, native, started("W"), userItem("W"), usageEvent("A", "W", "121200", "272000"), completed("W", "completed"))
	answers := compactStart(g, 2*time.Second)
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	select {
	case got := <-answers:
		t.Fatalf("answered %+v before the compaction's turn started", got.answer)
	case <-time.After(100 * time.Millisecond):
	}
	events(t, ui, native, started("C"), compactionItem("C", "item/started"))
	got := startWithin(t, answers)
	if got.answer.Outcome != control.Started || got.later == nil {
		t.Fatalf("answer %+v", got.answer)
	}
	ends := make(chan control.Answer, 1)
	go func() { ends <- got.later() }()
	select {
	case end := <-ends:
		t.Fatalf("ended %+v before the compaction did", end)
	case <-time.After(100 * time.Millisecond):
	}
	events(t, ui, native, compactionItem("C", "item/completed"), usageEvent("A", "C", "9000", "272000"), completed("C", "completed"),
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`)
	end := within(t, ends)
	if end.Outcome != control.Done || *end.TokensBefore != 121200 || *end.TokensAfter != 9000 {
		t.Fatalf("end %+v", end)
	}
	g.CompactionEnded("0123", "lead", end)
	snapshot := assertCompactions(t, g, 1)
	if outcomes := snapshot.CompactionOutcomes; len(outcomes) != 1 || outcomes[0].Request != "0123" || outcomes[0].RequestedBy != "lead" ||
		outcomes[0].Outcome != control.Done || *outcomes[0].TokensAfter != 9000 {
		t.Fatalf("outcomes %+v", outcomes)
	}
}

// A start not seen within the bound is answered requested, saying whether
// the server had taken the request; a request the server refuses is answered
// at once, with nothing left to wait for.
func TestACompactionWithNoStartSeenIsAnsweredRequested(t *testing.T) {
	for _, taken := range []bool{false, true} {
		g, ui, peers, _ := setup(t)
		native := <-peers
		bindUI(t, g, ui, native)
		ranATurn(t, ui, native)
		answers := compactStart(g, 200*time.Millisecond)
		id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
		want := "the server has not answered the request within 200ms"
		if taken {
			write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			want = "the server took the request, and its turn was not seen to start within 200ms"
		}
		if got := startWithin(t, answers); got.answer.Outcome != control.Requested || got.answer.Detail != want || got.later == nil {
			t.Fatalf("taken %v: answer %+v", taken, got.answer)
		}
		_ = native.conn.Close()
	}
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	answers := compactStart(g, 2*time.Second)
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","error":{"code":-32600,"message":"compaction is disabled"}}`))
	if got := startWithin(t, answers); got.answer.Outcome != control.Failed || got.answer.Detail != "compaction is disabled" || got.later != nil || got.answer.Open {
		t.Fatalf("refused: %+v", got.answer)
	}
	// A final answer is the outcome too: a command whose wait was cut short
	// holds an open answer, and main's letter comes from this one at once.
	if outcomes := g.SessionState().CompactionOutcomes; len(outcomes) != 1 || outcomes[0].Request != "0123" || outcomes[0].RequestedBy != "lead" ||
		outcomes[0].Outcome != control.Failed || outcomes[0].Detail != "compaction is disabled" {
		t.Fatalf("outcomes %+v", outcomes)
	}
}

// A connection that ends after the request went out is no proof the
// compaction will not run: the failure leaves the outcome open, and main's
// record of it stays for the letter.
func TestACompactionWhoseConnectionEndsIsLeftOpen(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	answers := compactStart(g, 2*time.Second)
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	_ = native.conn.Close()
	if got := startWithin(t, answers); got.answer.Outcome != control.Failed || !got.answer.Open {
		t.Fatalf("answer %+v, want a failure that leaves the outcome open", got.answer)
	}
	if outcomes := g.SessionState().CompactionOutcomes; len(outcomes) != 0 {
		t.Fatalf("an open failure recorded as the outcome: %+v", outcomes)
	}
}

// The gateway losing sight of the compaction before its item tied the mark is
// not its start: the command is answered requested, saying why, and the end
// later answers it failed as lost sight of.
func TestACompactionLostSightOfBeforeItsStartIsRequested(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold = 5 * time.Second
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	answers := compactStart(g, 2*time.Second)
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	leaveAndReturn(t, ui, native, "idle", "")
	got := startWithin(t, answers)
	if got.answer.Outcome != control.Requested || got.answer.Detail != "its start was not seen: "+lostWant || got.later == nil {
		t.Fatalf("answer %+v", got.answer)
	}
	if end := got.later(); end.Outcome != control.Failed || end.Detail != lostWant {
		t.Fatalf("end %+v", end)
	}
}
