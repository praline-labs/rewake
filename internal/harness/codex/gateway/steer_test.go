package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/control"
)

// nothingSent says the gateway wrote nothing to the server for a while.
func nothingSent(t *testing.T, native *socketClient) {
	t.Helper()
	_ = native.conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	raw, err := native.readMessage()
	_ = native.conn.SetReadDeadline(time.Time{})
	if err == nil {
		t.Fatalf("sent %s", raw)
	}
}

// injected reads the gateway's own request and checks its method and params.
func injected(t *testing.T, native *socketClient, method string, params map[string]string) string {
	t.Helper()
	var request struct {
		ID     string            `json:"id"`
		Method string            `json:"method"`
		Params map[string]string `json:"params"`
	}
	raw := readWithin(t, native)
	if err := json.Unmarshal(raw, &request); err != nil || request.Method != method || fmt.Sprint(request.Params) != fmt.Sprint(params) {
		t.Fatalf("sent %s; want %s %v", raw, method, params)
	}
	return request.ID
}

func events(t *testing.T, ui, native *socketClient, raws ...string) {
	t.Helper()
	for _, raw := range raws {
		telemetryEvent(t, ui, native, raw)
	}
}

func started(turn string) string {
	return `{"method":"turn/started","params":{"threadId":"A","turn":{"id":"` + turn + `","items":[],"status":"inProgress"}}}`
}

func completed(turn, status string) string {
	return `{"method":"turn/completed","params":{"threadId":"A","turn":{"id":"` + turn + `","items":[],"status":"` + status + `"}}}`
}

func compactionItem(turn, method string) string {
	return `{"method":"` + method + `","params":{"threadId":"A","turnId":"` + turn + `","item":{"type":"contextCompaction","id":"item-` + turn + `"}}}`
}

// ranATurn plays a turn of the terminal's to its end, so the conversation has
// something to compact. Its input item shows it to be work.
func ranATurn(t *testing.T, ui, native *socketClient) {
	t.Helper()
	events(t, ui, native, started("W"), userItem("W"), completed("W", "completed"))
}

// compactToEnd asks for a compaction and waits for its end, as main's letter
// does: most cases here are about the end, not the start.
func (g *Gateway) compactToEnd(ctx context.Context, request, by string) control.Answer {
	answer, later := g.Compact(ctx, request, by, 5*time.Second)
	if later == nil {
		return answer
	}
	return later()
}

func steerAnswer(ask func() control.Answer) <-chan control.Answer {
	out := make(chan control.Answer, 1)
	go func() { out <- ask() }()
	return out
}

func within(t *testing.T, answers <-chan control.Answer) control.Answer {
	t.Helper()
	select {
	case answer := <-answers:
		return answer
	case <-time.After(2 * time.Second):
		t.Fatal("no answer")
	}
	return control.Answer{}
}

func TestACompactionOnRequestIsManualAndCountedAsTheAskers(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	events(t, ui, native, started("W"), userItem("W"), usageEvent("A", "W", "121200", "272000"), completed("W", "completed"))
	out := callbacks(g)
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	events(t, ui, native,
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"active","activeFlags":[]}}}`,
		started("C"), compactionItem("C", "item/started"))
	// The compaction's turn is not one to interrupt.
	if answer := g.Interrupt(context.Background(), "lead"); answer.Reason != control.NoTurn || answer.Detail != "a compaction is running, not a turn" {
		t.Fatalf("interrupt during the compaction: %+v", answer)
	}
	events(t, ui, native, compactionItem("C", "item/completed"),
		usageEvent("A", "C", "9000", "272000"), completed("C", "completed"),
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`)
	answer := within(t, answers)
	if answer.Outcome != control.Done || answer.TokensBefore == nil || *answer.TokensBefore != 121200 || answer.TokensAfter == nil || *answer.TokensAfter != 9000 {
		t.Fatalf("answer %+v", answer)
	}
	snapshot := assertCompactions(t, g, 1)
	if last := snapshot.CompactionEvents[len(snapshot.CompactionEvents)-1]; last.RequestedBy != "lead" || last.Request != "0123" {
		t.Fatalf("the compaction is not the asker's: %+v", last)
	}
	select {
	case v := <-out:
		t.Fatalf("the compaction's turn was published as work: %+v", v)
	case <-time.After(700 * time.Millisecond):
	}
}

func TestACompactionIsRefusedWhileAnythingRuns(t *testing.T) {
	for name, running := range map[string]func(t *testing.T, ui, native *socketClient){
		"a turn": func(t *testing.T, ui, native *socketClient) {
			events(t, ui, native, started("T"))
		},
		"a turn/start in flight": func(t *testing.T, ui, native *socketClient) {
			write(t, ui, []byte(`{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
			_ = readWithin(t, native)
		},
		"a review/start in flight": func(t *testing.T, ui, native *socketClient) {
			write(t, ui, []byte(`{"id":9,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"}}}`))
			_ = readWithin(t, native)
		},
		"the terminal's /compact": func(t *testing.T, ui, native *socketClient) {
			exchange(t, ui, native, `{"id":8,"method":"thread/compact/start","params":{"threadId":"A"}}`, `{"id":8,"result":{}}`)
		},
		"a working status": func(t *testing.T, ui, native *socketClient) {
			events(t, ui, native, `{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"active","activeFlags":[]}}}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			running(t, ui, native)
			answer := g.compactToEnd(context.Background(), "0123", "lead")
			if answer.Outcome != control.Refused || answer.Reason != control.InTurn || answer.Detail == "" {
				t.Fatalf("answer %+v", answer)
			}
			nothingSent(t, native)
		})
	}
}

func TestACompactionTheServerRefusesLeavesNoMark(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	out := callbacks(g)
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","error":{"code":-32600,"message":"compaction is disabled"}}`))
	if answer := within(t, answers); answer.Outcome != control.Failed || answer.Detail != "compaction is disabled" {
		t.Fatalf("answer %+v", answer)
	}
	// The next turn is work again, not the refused compaction's.
	events(t, ui, native, started("T"), userItem("T"), completed("T", "completed"))
	select {
	case v := <-out:
		if v.Kind != "finished" {
			t.Fatalf("published %+v", v)
		}
	case <-time.After(time.Second):
		t.Fatal("the turn after a refused compaction was taken for it")
	}
}

func TestAnInterruptNamesTheMainInTheStoppedOutcome(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	out := callbacks(g)
	bindUI(t, g, ui, native)
	events(t, ui, native, started("T"), userItem("T"))
	answers := steerAnswer(func() control.Answer { return g.Interrupt(context.Background(), "lead") })
	id := injected(t, native, "turn/interrupt", map[string]string{"threadId": "A", "turnId": "T"})
	events(t, ui, native, completed("T", "interrupted"))
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	if answer := within(t, answers); answer.Outcome != control.Done {
		t.Fatalf("answer %+v", answer)
	}
	select {
	case v := <-out:
		if v.Kind != "stopped" || v.Text != "lead interrupted this turn with rewake interrupt" {
			t.Fatalf("published %+v", v)
		}
	case <-time.After(time.Second):
		t.Fatal("no stopped outcome")
	}
	// A person's Esc on the next turn is the person's again.
	events(t, ui, native, started("U"), userItem("U"), completed("U", "interrupted"))
	if v := <-out; v.Text != "the person at the keyboard stopped this turn" {
		t.Fatalf("published %+v", v)
	}
}

func TestAnInterruptWithNoTurnIsRefused(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	if answer := g.Interrupt(context.Background(), "lead"); answer.Outcome != control.Refused || answer.Reason != control.NoTurn {
		t.Fatalf("idle: %+v", answer)
	}
	nothingSent(t, native)
	// A compaction's turn is not one to interrupt.
	exchange(t, ui, native, `{"id":8,"method":"thread/compact/start","params":{"threadId":"A"}}`, `{"id":8,"result":{}}`)
	events(t, ui, native, started("C"), compactionItem("C", "item/started"))
	if answer := g.Interrupt(context.Background(), "lead"); answer.Outcome != control.Refused || answer.Reason != control.NoTurn {
		t.Fatalf("compacting: %+v", answer)
	}
	nothingSent(t, native)
	events(t, ui, native, completed("C", "completed"), started("T"))
	// The server's refusal of a turn that ended meanwhile is the same answer.
	answers := steerAnswer(func() control.Answer { return g.Interrupt(context.Background(), "lead") })
	id := injected(t, native, "turn/interrupt", map[string]string{"threadId": "A", "turnId": "T"})
	write(t, native, []byte(`{"id":"`+id+`","error":{"code":-32600,"message":"no active turn to interrupt"}}`))
	if answer := within(t, answers); answer.Outcome != control.Refused || answer.Reason != control.NoTurn || answer.Detail != "no active turn to interrupt" {
		t.Fatalf("refused by the server: %+v", answer)
	}
}

// A turn a notice started is published through the admitted work, and names
// the main as well.
func TestAnInterruptedDeliveryNamesTheMain(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	out := callbacks(g)
	bindUI(t, g, ui, native)
	delivered := make(chan error, 1)
	go func() {
		_, err := g.Deliver(context.Background(), g.Binding(), "message-id", "fixture notice")
		delivered <- err
	}()
	var start struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(readWithin(t, native), &start); err != nil {
		t.Fatal(err)
	}
	write(t, native, []byte(`{"id":"`+start.ID+`","result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`))
	if err := <-delivered; err != nil {
		t.Fatal(err)
	}
	events(t, ui, native, started("T"))
	answers := steerAnswer(func() control.Answer { return g.Interrupt(context.Background(), "lead") })
	id := injected(t, native, "turn/interrupt", map[string]string{"threadId": "A", "turnId": "T"})
	events(t, ui, native, completed("T", "interrupted"))
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	if answer := within(t, answers); answer.Outcome != control.Done {
		t.Fatalf("answer %+v", answer)
	}
	select {
	case v := <-out:
		if v.Kind != "stopped" || v.Text != "lead interrupted this turn with rewake interrupt" || !v.Retained {
			t.Fatalf("published %+v", v)
		}
	case <-time.After(time.Second):
		t.Fatal("no stopped outcome")
	}
}
