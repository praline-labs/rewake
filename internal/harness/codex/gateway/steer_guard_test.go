package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/control"
)

// The server may answer turn/start before it announces the turn: from the
// reply on the turn is running, and a compaction sent then would abort it.
func TestACompactionIsRefusedBetweenATurnsReplyAndItsStart(t *testing.T) {
	for name, start := range map[string]func(t *testing.T, g *Gateway, ui, native *socketClient){
		"the terminal's turn": func(t *testing.T, _ *Gateway, ui, native *socketClient) {
			exchange(t, ui, native, `{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":7,"result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`)
		},
		"a delivery's turn": func(t *testing.T, g *Gateway, _, native *socketClient) {
			delivered := make(chan error, 1)
			go func() {
				_, err := g.Deliver(context.Background(), g.Binding(), "message-id", "fixture notice")
				delivered <- err
			}()
			var request struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(readWithin(t, native), &request); err != nil {
				t.Fatal(err)
			}
			write(t, native, []byte(`{"id":"`+request.ID+`","result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`))
			if err := <-delivered; err != nil {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			start(t, g, ui, native)
			answer := compactBounded(g, "0123")
			if answer.Outcome != control.Refused || answer.Reason != control.InTurn || answer.Detail != "a turn the server started has not ended" {
				t.Fatalf("answer %+v", answer)
			}
			nothingSent(t, native)
			// Once the turn has ended the conversation compacts.
			events(t, ui, native, started("T"), completed("T", "completed"))
			_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
			injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
		})
	}
}

// compactBounded asks for a compaction that is expected to be refused, bounded:
// one sent after all would wait for its end.
func compactBounded(g *Gateway, request string) control.Answer {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return g.compactToEnd(ctx, request, "lead")
}

// A turn acknowledged before the terminal left the thread and never seen to
// start keeps the conversation uncertain for main's compaction: its events stop
// coming with the selection, an idle resume does not prove it will not start,
// and no time does. The answer to a turn sent after it does.
func TestATurnAcknowledgedBeforeTheThreadWasLeftStaysUncertainUntilALaterTurnIsAnswered(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold = 100 * time.Millisecond
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	exchange(t, ui, native, `{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":7,"result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`)
	if answer := compactBounded(g, "0123"); answer.Reason != control.InTurn || answer.Detail == uncertainDetail {
		t.Fatalf("before leaving: %+v", answer)
	}
	exchange(t, ui, native, `{"id":8,"method":"thread/resume","params":{"threadId":"B","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":8,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`)
	exchange(t, ui, native, `{"id":9,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":9,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"idle"}}}}`)
	refusedAsUncertain(t, g, native, "after coming back")
	time.Sleep(300 * time.Millisecond)
	refusedAsUncertain(t, g, native, "past any bound")
	exchange(t, ui, native, `{"id":10,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":10,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`)
	events(t, ui, native, started("U"), completed("U", "completed"))
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0124", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
}

// The terminal's /compact while main's compaction runs is answered by the
// gateway and never reaches the server, which would abort main's for it.
func TestTheTerminalsCompactionIsRefusedWhileMainsRuns(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	out := callbacks(g)
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	write(t, ui, []byte(`{"id":8,"method":"thread/compact/start","params":{"threadId":"A"}}`))
	var reply struct {
		ID    int `json:"id"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	raw := readWithin(t, ui)
	if err := json.Unmarshal(raw, &reply); err != nil || reply.ID != 8 || reply.Error.Code != -32600 || !strings.Contains(reply.Error.Message, "lead asked for a compaction") {
		t.Fatalf("the terminal read %s", raw)
	}
	nothingSent(t, native)
	events(t, ui, native,
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"active","activeFlags":[]}}}`,
		started("C"), compactionItem("C", "item/started"), compactionItem("C", "item/completed"), completed("C", "completed"),
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`)
	if answer := within(t, answers); answer.Outcome != control.Done {
		t.Fatalf("main's answer %+v", answer)
	}
	snapshot := assertCompactions(t, g, 1)
	if last := snapshot.CompactionEvents[0]; last.RequestedBy != "lead" || last.Request != "0123" {
		t.Fatalf("the compaction is not main's: %+v", last)
	}
	select {
	case v := <-out:
		t.Fatalf("published %+v", v)
	case <-time.After(700 * time.Millisecond):
	}
	// With main's compaction over, the terminal's passes again.
	exchange(t, ui, native, `{"id":9,"method":"thread/compact/start","params":{"threadId":"A"}}`, `{"id":9,"result":{}}`)
}

// A compaction request that got no answer may still have started it: the mark
// stays, so its turn counts as main's, and it is not published, having no
// proof of work.
func TestAnUnansweredCompactionKeepsItsMark(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	out := callbacks(g)
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(ctx, "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	if answer := within(t, answers); answer.Outcome != control.Failed || !strings.Contains(answer.Detail, "it may still start") || !answer.Open {
		t.Fatalf("answer %+v", answer)
	}
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	events(t, ui, native, started("C"), compactionItem("C", "item/started"), compactionItem("C", "item/completed"), completed("C", "completed"))
	snapshot := assertCompactions(t, g, 1)
	if last := snapshot.CompactionEvents[0]; last.RequestedBy != "lead" {
		t.Fatalf("the late compaction is not main's: %+v", last)
	}
	select {
	case v := <-out:
		t.Fatalf("published %+v", v)
	case <-time.After(700 * time.Millisecond):
	}
}

// A conversation started here that has run no turn is not compacted: Codex
// would, and spend a model call on nothing. One resumed, or one that has run
// a turn, is.
func TestAConversationWithNoTurnHasNothingToCompact(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	// Bounded: a compaction sent after all would wait for its end.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if answer := g.compactToEnd(ctx, "0123", "lead"); answer.Outcome != control.Refused || answer.Reason != control.NothingToCompact {
		t.Fatalf("new conversation: %+v", answer)
	}
	nothingSent(t, native)
	ranATurn(t, ui, native)
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})

	g, ui, peers, _ = setup(t)
	native = <-peers
	defer func() { _ = native.conn.Close() }()
	socketResume(t, ui, native)
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
}

// A mark whose compaction request has no reply stops holding with main's
// answer, but the compaction it stands for may still start: main's next one
// stays refused until a turn sent after it is answered.
func TestAMarkWhoseCompactionNeverStartsStopsHoldingAtItsBound(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold = 200 * time.Millisecond
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(ctx, "0123", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	if answer := within(t, answers); answer.Outcome != control.Failed {
		t.Fatalf("answer %+v", answer)
	}
	// Main's answer ended the hold; the compaction may still start.
	if answer := compactBounded(g, "0124"); answer.Reason != control.InTurn || answer.Detail != uncertainDetail {
		t.Fatalf("within the limit: %+v", answer)
	}
	time.Sleep(400 * time.Millisecond)
	if err := reserveWithin(g, 200*time.Millisecond); err != nil {
		t.Fatalf("a delivery past the limit: %v", err)
	}
	refusedAsUncertain(t, g, native, "past the limit")
	if answer := g.Interrupt(context.Background(), "lead"); answer.Reason != control.NoTurn || answer.Detail != "" {
		t.Fatalf("an interrupt past the limit: %+v", answer)
	}
	exchange(t, ui, native, `{"id":10,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":10,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`)
	events(t, ui, native, started("U"), completed("U", "completed"))
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0125", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
}

// A delivery that comes while main's compaction runs waits for its end and then
// goes, rather than being refused: compacting a worker and then sending it the
// task is the ordinary order.
func TestADeliveryWaitsOutMainsCompaction(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	events(t, ui, native, started("C"), compactionItem("C", "item/started"))
	// One whose wait ends first is told to come back, not refused.
	short, cancelShort := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelShort()
	if _, err := g.Reserve(short); !errors.Is(err, ErrCompacting) {
		t.Fatalf("a wait that ended during the compaction: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	type reserved struct {
		r   *Reservation
		err error
	}
	reservations := make(chan reserved, 1)
	go func() { r, err := g.Reserve(ctx); reservations <- reserved{r, err} }()
	select {
	case got := <-reservations:
		t.Fatalf("reserved during the compaction: %v", got.err)
	case <-time.After(150 * time.Millisecond):
	}
	events(t, ui, native, compactionItem("C", "item/completed"), completed("C", "completed"))
	if answer := within(t, answers); answer.Outcome != control.Done {
		t.Fatalf("main's answer %+v", answer)
	}
	var got reserved
	select {
	case got = <-reservations:
	case <-time.After(2 * time.Second):
		t.Fatal("the delivery did not go after the compaction")
	}
	if got.err != nil {
		t.Fatal(got.err)
	}
	defer got.r.Close()
	delivered := make(chan error, 1)
	go func() {
		_, err := got.r.Deliver(ctx, "message-id", MailboxNotice{Notice: "fixture notice"}, nil)
		delivered <- err
	}()
	var request struct {
		ID     string `json:"id"`
		Method string `json:"method"`
	}
	if raw := readWithin(t, native); json.Unmarshal(raw, &request) != nil || request.Method != "turn/start" {
		t.Fatalf("sent %s", raw)
	}
	write(t, native, []byte(`{"id":"`+request.ID+`","result":{"turn":{"id":"D","items":[],"status":"inProgress"}}}`))
	if err := <-delivered; err != nil {
		t.Fatal(err)
	}
}
