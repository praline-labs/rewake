package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/control"
)

// An inline review runs on the conversation from its reply on, before its
// turn/started: a compaction then would abort it.
func TestACompactionIsRefusedWhileAReviewIsAcknowledged(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	exchange(t, ui, native, `{"id":7,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"},"delivery":"inline"}}`, `{"id":7,"result":{"reviewThreadId":"A","turn":{"id":"R","items":[],"status":"inProgress"}}}`)
	if answer := compactBounded(g, "0123"); answer.Outcome != control.Refused || answer.Reason != control.InTurn {
		t.Fatalf("answer %+v", answer)
	}
	nothingSent(t, native)
	// A detached review runs on a conversation of its own.
	events(t, ui, native, started("R"), completed("R", "completed"))
	exchange(t, ui, native, `{"id":8,"method":"review/start","params":{"threadId":"A","target":{"type":"uncommittedChanges"},"delivery":"detached"}}`, `{"id":8,"result":{"reviewThreadId":"V","turn":{"id":"S","items":[],"status":"inProgress"}}}`)
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0124", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
}

// The turn an interrupt names is the one the server named: in its reply to
// turn/start before turn/started, and in a resume's snapshot of a running
// conversation, which no turn/started follows.
func TestAnInterruptNamesTheTurnTheServerNamed(t *testing.T) {
	for _, scenario := range []string{"acknowledged, not announced", "resumed while running"} {
		t.Run(scenario, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			exchange(t, ui, native, `{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`, `{"id":7,"result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`)
			if scenario == "resumed while running" {
				events(t, ui, native, started("T"))
				exchange(t, ui, native, `{"id":8,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":8,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"active","activeFlags":[]},"turns":[{"id":"W","items":[],"status":"completed"},{"id":"T","items":[],"status":"inProgress"}]}}}`)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			answers := steerAnswer(func() control.Answer { return g.Interrupt(ctx, "lead") })
			select {
			case answer := <-answers:
				t.Fatalf("answered without asking the server: %+v", answer)
			case <-time.After(50 * time.Millisecond):
			}
			id := injected(t, native, "turn/interrupt", map[string]string{"threadId": "A", "turnId": "T"})
			write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			if answer := within(t, answers); answer.Outcome != control.Done {
				t.Fatalf("answer %+v", answer)
			}
		})
	}
}

// A delivery that queued on the gate before main's compaction took it learns
// of the compaction once its wait ends: it stays pending, not failed.
func TestADeliveryQueuedBehindACompactionStaysPending(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	// Hold the gate so both queue on it, the compaction first.
	if err := g.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	compactCtx, cancelCompact := context.WithTimeout(context.Background(), time.Second)
	defer cancelCompact()
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(compactCtx, "0123", "lead") })
	time.Sleep(30 * time.Millisecond)
	reserveCtx, cancelReserve := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancelReserve()
	result := make(chan error, 1)
	go func() {
		r, err := g.Reserve(reserveCtx)
		if r != nil {
			r.Close()
		}
		result <- err
	}()
	time.Sleep(30 * time.Millisecond)
	<-g.gate
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	if err := <-result; !errors.Is(err, ErrCompacting) {
		t.Fatalf("the delivery: %v", err)
	}
}

// leaveAndReturn resumes B and then A again, answering A's resume with status,
// and ends the resume's reads as the terminal's next request does.
func leaveAndReturn(t *testing.T, ui, native *socketClient, status, turns string) {
	t.Helper()
	exchange(t, ui, native, `{"id":20,"method":"thread/resume","params":{"threadId":"B","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":20,"result":{"thread":{"id":"B","canAcceptDirectInput":true}}}`)
	exchange(t, ui, native, `{"id":21,"method":"thread/resume","params":{"threadId":"A","config":{},"runtimeWorkspaceRoots":[]}}`, `{"id":21,"result":{"thread":{"id":"A","canAcceptDirectInput":true,"status":{"type":"`+status+`"},"turns":[`+turns+`]}}}`)
	exchange(t, ui, native, `{"id":22,"method":"thread/goal/get","params":{"threadId":"A"}}`, `{"id":22,"result":{}}`)
}

func reserveWithin(g *Gateway, wait time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	r, err := g.Reserve(ctx)
	if r != nil {
		r.Close()
	}
	return err
}

// A compaction whose end this connection never reads — the terminal left the
// conversation during it — is not ended by a resume showing the conversation
// idle: only its turn/completed ends it, so main's next compaction stays
// refused. One whose start was seen stops holding deliveries, as the reply
// shows it no longer running, and main's wait for its end goes on; one only
// answered is lost sight of — main's wait fails at once, and deliveries go.
func TestACompactionLeftBehindIsNotEndedByAnIdleResume(t *testing.T) {
	for _, variant := range []string{"main's, started", "main's, only answered", "the terminal's"} {
		t.Run(variant, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ranATurn(t, ui, native)
			var answers <-chan control.Answer
			if variant == "the terminal's" {
				exchange(t, ui, native, `{"id":8,"method":"thread/compact/start","params":{"threadId":"A"}}`, `{"id":8,"result":{}}`)
			} else {
				answers = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
				id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
				write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
			}
			if variant != "main's, only answered" {
				events(t, ui, native, started("C"), compactionItem("C", "item/started"))
			}
			leaveAndReturn(t, ui, native, "idle", `{"id":"W","items":[],"status":"completed"},{"id":"C","items":[],"status":"completed"}`)
			if variant != "main's, only answered" {
				if err := reserveWithin(g, 200*time.Millisecond); err != nil {
					t.Fatalf("a delivery after coming back: %v", err)
				}
				refusedAsUncertain(t, g, native, "main's compaction after coming back")
				if answers != nil {
					select {
					case answer := <-answers:
						t.Fatalf("main's answer before its compaction ended: %+v", answer)
					default:
					}
				}
				return
			}
			if answer := within(t, answers); answer.Outcome != control.Failed || answer.Detail != lostWant {
				t.Fatalf("main's answer %+v", answer)
			}
			if err := reserveWithin(g, 200*time.Millisecond); err != nil {
				t.Fatalf("a delivery after main's answer: %v", err)
			}
			refusedAsUncertain(t, g, native, "after main's answer")
		})
	}
}

func TestACompactionStillRunningAfterAResumeHoldsUntilItsEnd(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	out := callbacks(g)
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	events(t, ui, native, started("C"), compactionItem("C", "item/started"))
	leaveAndReturn(t, ui, native, "active", `{"id":"W","items":[],"status":"completed"},{"id":"C","items":[],"status":"inProgress"}`)
	if err := reserveWithin(g, 200*time.Millisecond); !errors.Is(err, ErrCompacting) {
		t.Fatalf("a delivery during the compaction: %v", err)
	}
	if answer := g.Interrupt(context.Background(), "lead"); answer.Reason != control.NoTurn || answer.Detail != "a compaction is running, not a turn" {
		t.Fatalf("interrupt during the compaction: %+v", answer)
	}
	events(t, ui, native, compactionItem("C", "item/completed"), completed("C", "completed"))
	if answer := within(t, answers); answer.Outcome != control.Done {
		t.Fatalf("main's answer %+v", answer)
	}
	if err := reserveWithin(g, time.Second); err != nil {
		t.Fatalf("a delivery after the compaction: %v", err)
	}
	select {
	case v := <-out:
		t.Fatalf("the compaction's turn was published as work: %+v", v)
	case <-time.After(700 * time.Millisecond):
	}
}

// The server may send a turn's end before its reply to turn/start: a reply
// naming a turn already ended starts nothing.
func TestAReplyNamingATurnAlreadyEndedHoldsNothing(t *testing.T) {
	g, ui, peers, _ := setup(t)
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	write(t, ui, []byte(`{"id":7,"method":"turn/start","params":{"threadId":"A","input":[]}}`))
	_ = readWithin(t, native)
	events(t, ui, native, started("T"), completed("T", "completed"))
	write(t, native, []byte(`{"id":7,"result":{"turn":{"id":"T","items":[],"status":"inProgress"}}}`))
	_ = readWithin(t, ui)
	_ = steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
}
