package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/control"
)

// resumeA selects A again, its reply saying status with turns, and ends the
// resume's reads as the terminal's next request does.
func resumeA(t *testing.T, ui, native *socketClient, status, turns string) {
	t.Helper()
	exchange(t, ui, native, `{"id":21,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":21,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"`+status+`"},"turns":[`+turns+`]}}}`)
	exchange(t, ui, native, `{"id":22,"method":"thread/goal/get","params":{"threadId":"A"}}`, `{"id":22,"result":{}}`)
}

func refusedAsUncertain(t *testing.T, g *Gateway, native *socketClient, when string) {
	t.Helper()
	if answer := compactBounded(g, "0199"); answer.Reason != control.InTurn || answer.Detail != uncertainDetail {
		t.Fatalf("%s: %+v", when, answer)
	}
	nothingSent(t, native)
}

// An operation the server accepted and whose end was not read leaves the
// conversation uncertain for main's compaction: nothing but its end, or the
// answer to a turn sent after it, says it will not start or has finished.
// Neither a resume, an idle or systemError status, nor time does.
func TestAnAcceptedOperationWithoutItsEndLeavesTheConversationUncertain(t *testing.T) {
	for _, variant := range []string{"review, idle resume", "review started, idle resume", "turn, systemError", "turn, idle and its items"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			g.markHold = 100 * time.Millisecond
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			turn := "T"
			switch variant {
			case "review, idle resume", "review started, idle resume":
				turn = "R"
				exchange(t, ui, native, `{"id":7,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"},"delivery":"inline"}}`, `{"id":7,"result":{"reviewThreadId":"A","turn":{"id":"R","items":[],"status":"inProgress"}}}`)
				if variant == "review started, idle resume" {
					events(t, ui, native, started("R"))
				}
				resumeA(t, ui, native, "idle", "")
			case "turn, systemError":
				exchange(t, ui, native, `{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":7,"result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`)
				events(t, ui, native, started("T"), `{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"systemError"}}}`)
				resumeA(t, ui, native, "systemError", `{"id":"T","items":[],"status":"interrupted"}`)
			case "turn, idle and its items":
				exchange(t, ui, native, `{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":7,"result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`)
				events(t, ui, native, started("T"),
					`{"method":"item/completed","params":{"threadId":"A","turnId":"T","item":{"id":"i1","type":"agentMessage","text":"x"},"completedAtMs":2}}`,
					`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`)
			}
			time.Sleep(200 * time.Millisecond)
			if variant == "turn, idle and its items" {
				// The running turn refuses first, as a turn: the idle status
				// comes before its turn/completed and does not clear it.
				if answer := compactBounded(g, "0198"); answer.Reason != control.InTurn || answer.Detail != "a turn is running" {
					t.Fatalf("before its end: %+v", answer)
				}
				nothingSent(t, native)
			} else {
				refusedAsUncertain(t, g, native, "before its end")
			}
			events(t, ui, native, completed(turn, "interrupted"))
			_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0124", "lead") })
			injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
		})
	}
}

// A turn/start answered after the operation was sent proves its handler has
// run: it has started or been refused, and cannot first start later.
func TestAnAnsweredLaterTurnEndsTheUncertainty(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	exchange(t, ui, native, `{"id":7,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"},"delivery":"inline"}}`, `{"id":7,"result":{"reviewThreadId":"A","turn":{"id":"R","items":[],"status":"inProgress"}}}`)
	resumeA(t, ui, native, "idle", "")
	refusedAsUncertain(t, g, native, "after the idle resume")
	exchange(t, ui, native, `{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`)
	events(t, ui, native, started("U"), completed("U", "completed"))
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0124", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
}

// The server answers thread/compact/start once it has queued the compaction,
// and may answer a resume before it takes it: an idle snapshot then says
// nothing about the compaction, which starts after it. A resume is a change
// of selection, so the mark is given up rather than tied after it; the
// compaction's turn still settles nothing, and its operation stays open.
func TestACompactionAcceptedButNotStartedOutlivesAnIdleResume(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	out := callbacks(g)
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	resumeA(t, ui, native, "idle", "")
	if answer := within(t, answers); answer.Outcome != control.Failed || answer.Detail != lostWant {
		t.Fatalf("main's answer %+v", answer)
	}
	events(t, ui, native, started("C"), compactionItem("C", "item/started"), compactionItem("C", "item/completed"), completed("C", "completed"))
	select {
	case v := <-out:
		t.Fatalf("the compaction's turn was published as work: %+v", v)
	case <-time.After(700 * time.Millisecond):
	}
	refusedAsUncertain(t, g, native, "after the compaction's turn")
}

// The mark holds deliveries and main's wait only up to its bound. Past it
// deliveries go and main is told why, but the mark stays: a compaction that
// starts late is still not taken for work.
func TestAMarkHoldsDeliveriesOnlyUntilItsBound(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold = 300 * time.Millisecond
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	out := callbacks(g)
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	if err := reserveWithin(g, 50*time.Millisecond); !errors.Is(err, ErrCompacting) {
		t.Fatalf("a delivery within the bound: %v", err)
	}
	answer := within(t, answers)
	if answer.Outcome != control.Failed || answer.Detail != "the compaction's turn was not seen to start within 300ms; deliveries go on, and its turn is still not taken for work if it starts" {
		t.Fatalf("main's answer %+v", answer)
	}
	if err := reserveWithin(g, 100*time.Millisecond); err != nil {
		t.Fatalf("a delivery past the bound: %v", err)
	}
	events(t, ui, native, started("C"), compactionItem("C", "item/started"), compactionItem("C", "item/completed"), completed("C", "completed"))
	select {
	case v := <-out:
		t.Fatalf("the late compaction's turn was published as work: %+v", v)
	case <-time.After(700 * time.Millisecond):
	}
	// Its item named the open operation's turn, so that turn's end closed it.
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0124", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
}

// The mark is tied to its turn only by a contextCompaction item: a turn of the
// terminal's that starts while it is unbound is work, published as such, and
// main is not told its compaction was done.
func TestAnUnboundMarkTakesNoOtherTurn(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold = 300 * time.Millisecond
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	out := callbacks(g)
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	leaveAndReturn(t, ui, native, "idle", "")
	exchange(t, ui, native, `{"id":30,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":30,"result":{"turn":{"id":"U","items":[],"status":"inProgress"}}}`)
	events(t, ui, native, `{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"active","activeFlags":[]}}}`, started("U"),
		`{"method":"item/completed","params":{"threadId":"A","turnId":"U","item":{"id":"i1","type":"agentMessage","text":"user answer"},"completedAtMs":2}}`,
		`{"method":"thread/status/changed","params":{"threadId":"A","status":{"type":"idle"}}}`, completed("U", "completed"))
	select {
	case v := <-out:
		if v.ID != "A/U" || v.Text != "user answer" {
			t.Fatalf("published %+v", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the terminal's turn was not published")
	}
	if answer := within(t, answers); answer.Outcome == control.Done {
		t.Fatalf("main was told a compaction that did not run was done: %+v", answer)
	}
}

// A resume without turns, the terminal's ordinary one, says a turn runs but
// not which; the turn's next event names it.
func TestAnInterruptLearnsTheTurnFromItsEvents(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	resumeA(t, ui, native, "active", "")
	events(t, ui, native, `{"method":"item/started","params":{"threadId":"A","turnId":"T","item":{"id":"i1","type":"agentMessage","text":""},"startedAtMs":1}}`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	answers := steerAnswer(func() control.Answer { return g.Interrupt(ctx, "lead") })
	id := injected(t, native, "turn/interrupt", map[string]string{"threadId": "A", "turnId": "T"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	if answer := within(t, answers); answer.Outcome != control.Done {
		t.Fatalf("answer %+v", answer)
	}
}

// A reply to a request of an earlier selection names no running turn of this
// one, even for the same conversation — whether its turn was read to its end
// there or not.
func TestALateReplyFromAnEarlierSelectionNamesNoTurn(t *testing.T) {
	requests := map[string][2]string{
		"review": {`{"id":7,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"},"delivery":"inline"}}`, `{"id":7,"result":{"reviewThreadId":"A","turn":{"id":"R","items":[],"status":"inProgress"}}}`},
		"turn":   {`{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":7,"result":{"turn":{"id":"R","items":[],"status":"inProgress"}}}`},
	}
	for _, request := range []string{"review", "turn"} {
		for _, variant := range []string{"read to its end", "end not seen"} {
			t.Run(request+", "+variant, func(t *testing.T) {
				g, ui, peers, _ := setup(t)
				native := <-peers
				defer func() { _ = native.conn.Close() }()
				bindUI(t, g, ui, native)
				write(t, ui, []byte(requests[request][0]))
				_ = readWithin(t, native)
				if variant == "read to its end" {
					events(t, ui, native, started("R"), completed("R", "completed"))
				}
				leaveAndReturn(t, ui, native, "active", `{"id":"U","items":[],"status":"inProgress"}`)
				write(t, native, []byte(requests[request][1]))
				_ = readWithin(t, ui)
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_ = steerAnswer(func() control.Answer { return g.Interrupt(ctx, "lead") })
				injected(t, native, "turn/interrupt", map[string]string{"threadId": "A", "turnId": "U"})
			})
		}
	}
}

// The answer to a turn/start proves only what was sent before it: a
// compaction the terminal sent while that answer was on its way stays open.
func TestAnAnswerProvesNothingOfWhatWasSentAfterItsRequest(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold = 100 * time.Millisecond
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	write(t, ui, []byte(`{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
	_ = readWithin(t, native)
	write(t, ui, []byte(`{"id":8,"method":"thread/compact/start","params":{"threadId":"A"}}`))
	_ = readWithin(t, native)
	write(t, native, []byte(`{"id":7,"result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`))
	_ = readWithin(t, ui)
	write(t, native, []byte(`{"id":8,"result":{}}`))
	_ = readWithin(t, ui)
	events(t, ui, native, started("T"), completed("T", "completed"))
	time.Sleep(200 * time.Millisecond)
	refusedAsUncertain(t, g, native, "after the earlier turn ended")
}

// Only a final status ends a turn: a turn/completed that says the turn is
// still in progress is not its end.
func TestATurnCompletedWithoutAFinalStatusEndsNothing(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	exchange(t, ui, native, `{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":7,"result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`)
	events(t, ui, native, started("T"), completed("T", "inProgress"))
	if answer := compactBounded(g, "0198"); answer.Reason != control.InTurn {
		t.Fatalf("before its end: %+v", answer)
	}
	nothingSent(t, native)
	events(t, ui, native, completed("T", "completed"))
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0124", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
}

// A reply to a turn/start sent for another conversation names no running turn
// once the terminal has selected that one: the selection's reply says what runs.
func TestAReplyToARequestForAnotherConversationNamesNoTurn(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	write(t, ui, []byte(`{"id":7,"method":"turn/start","params":{"threadId":"B","input":[]}}`))
	_ = readWithin(t, native)
	exchange(t, ui, native, `{"id":20,"method":"thread/resume","params":{"threadId":"B","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":20,"result":{"thread":{"id":"B","canAcceptDirectInput":true,"status":{"type":"active"},"turns":[{"id":"U","items":[],"status":"inProgress"}]}}}`)
	exchange(t, ui, native, `{"id":22,"method":"thread/goal/get","params":{"threadId":"B"}}`, `{"id":22,"result":{}}`)
	write(t, native, []byte(`{"id":7,"result":{"turn":{"id":"R","items":[],"status":"inProgress"}}}`))
	_ = readWithin(t, ui)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = steerAnswer(func() control.Answer { return g.Interrupt(ctx, "lead") })
	injected(t, native, "turn/interrupt", map[string]string{"threadId": "B", "turnId": "U"})
}
