package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/control"
)

// A compaction of a long conversation outlasts the start bound: once its
// item has tied the mark to its turn, the mark holds deliveries until that
// turn ends, however long the start bound. A delivery sent in the gap would
// be refused by the server; a live one on 0.155.1 was, and failed for good.
func TestATiedMarkHoldsPastItsStartBoundUntilItsEnd(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold, g.runHold = 200*time.Millisecond, time.Minute
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	events(t, ui, native, started("C"), compactionItem("C", "item/started"))
	time.Sleep(400 * time.Millisecond)
	if err := reserveWithin(g, 100*time.Millisecond); !errors.Is(err, ErrCompacting) {
		t.Fatalf("a delivery past the start bound, the compaction running: %v", err)
	}
	select {
	case answer := <-answers:
		t.Fatalf("main was answered before the compaction ended: %+v", answer)
	default:
	}
	events(t, ui, native, compactionItem("C", "item/completed"), completed("C", "completed"))
	if answer := within(t, answers); answer.Outcome != control.Done {
		t.Fatalf("main's answer %+v", answer)
	}
	if err := reserveWithin(g, time.Second); err != nil {
		t.Fatalf("a delivery after the compaction: %v", err)
	}
}

// Past the running bound the hold and main's wait end, and main is told the
// compaction may still be running — started, which its wrapper takes for no
// outcome yet, not failed. When the compaction's end comes after, it is
// recorded as main's outcome, with the tokens, for the letter still owed.
func TestACompactionEndingPastItsBoundIsRecordedWhenItEnds(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold, g.runHold = 200*time.Millisecond, 400*time.Millisecond
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	events(t, ui, native, started("W"), userItem("W"), usageEvent("A", "W", "121200", "272000"), completed("W", "completed"))
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	events(t, ui, native, started("C"), compactionItem("C", "item/started"))
	answer := within(t, answers)
	if answer.Outcome != control.Started || answer.Detail != "the compaction was not seen to end within 400ms; it may still be running, and its end is reported when seen" {
		t.Fatalf("main's answer at the bound %+v", answer)
	}
	if err := reserveWithin(g, 100*time.Millisecond); err != nil {
		t.Fatalf("a delivery past the running bound: %v", err)
	}
	events(t, ui, native, usageEvent("A", "C", "9000", "272000"), compactionItem("C", "item/completed"), completed("C", "completed"))
	outcomes := g.SessionState().CompactionOutcomes
	if len(outcomes) != 1 {
		t.Fatalf("outcomes %+v", outcomes)
	}
	last := outcomes[0]
	if last.Request != "0123" || last.RequestedBy != "lead" || last.Outcome != control.Done || last.TokensBefore == nil || *last.TokensBefore != 121200 || last.TokensAfter == nil || *last.TokensAfter != 9000 {
		t.Fatalf("the late end recorded %+v", last)
	}
	snapshot := assertCompactions(t, g, 1)
	if event := snapshot.CompactionEvents[0]; event.Request != "0123" || event.RequestedBy != "lead" {
		t.Fatalf("the compaction is not main's: %+v", event)
	}
}

// The server refuses input while a compaction runs, and refuses it rather
// than queue it: a delivery refused so is told apart from a failure, so its
// message waits and goes again once the compaction has ended.
func TestADeliveryRefusedForARunningCompactionWaits(t *testing.T) {
	for _, refusal := range []string{
		"failed to submit turn input: ActiveTurnNotSteerable { turn_kind: Compact }",
		"failed to submit turn input: ActiveTurnNotSteerable { turn_kind: Review }",
	} {
		t.Run(refusal, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			r, err := g.Reserve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			done := make(chan error, 1)
			go func() { _, err := r.Deliver(ctx, "task", MailboxNotice{Notice: "notice"}, nil); done <- err }()
			injection := metadata(t, string(readWithin(t, native)))
			write(t, native, []byte(fmt.Sprintf(`{"id":%q,"error":{"code":-32603,"message":%q}}`, injection.idText, refusal)))
			err = <-done
			compacting := strings.Contains(refusal, "Compact")
			if err == nil || errors.Is(err, ErrCompacting) != compacting || !strings.Contains(err.Error(), refusal) {
				t.Fatalf("the delivery's error %v; want it taken for a compaction running: %v", err, compacting)
			}
		})
	}
}

// A server silent on main's compaction request holds the terminal only as long
// as an untied mark: the request holds the admission gate every request of the
// terminal's waits for, and is bounded by the start bound, not by the wait for
// the compaction's end.
func TestASilentCompactionRequestHoldsTheTerminalOnlyToTheStartBound(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold, g.runHold = 200*time.Millisecond, 10*time.Second
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	bindUI(t, g, ui, native)
	ranATurn(t, ui, native)
	ctx, cancel := context.WithTimeout(context.Background(), g.CompactionWait())
	defer cancel()
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(ctx, "0123", "lead") })
	injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	asked := time.Now()
	write(t, ui, []byte(`{"id":30,"method":"thread/read","params":{"threadId":"A"}}`))
	if read := metadata(t, string(readWithin(t, native))); read.method != "thread/read" {
		t.Fatalf("the terminal's request reached the server as %+v", read)
	}
	if waited := time.Since(asked); waited > time.Second {
		t.Fatalf("the terminal's request waited %v behind a silent compaction request", waited)
	}
	if answer := within(t, answers); answer.Outcome != control.Failed || !answer.Open {
		t.Fatalf("main's answer %+v", answer)
	}
}

// main's command publishes its started answer after the connection's lock is
// let go, so the compaction's end, recorded under that lock, can come first. A
// started that comes after the end does not stand over it: main's letter would
// take the compaction for still running, and wait for its bound.
func TestALateStartedDoesNotStandOverTheEnd(t *testing.T) {
	g, ui, peers, _ := setup(t)
	g.markHold, g.runHold = 200*time.Millisecond, 400*time.Millisecond
	native := <-peers
	defer func() { _ = native.conn.Close() }()
	modelBinding(t, g, ui, native)
	events(t, ui, native, started("W"), userItem("W"), usageEvent("A", "W", "121200", "272000"), completed("W", "completed"))
	answers := steerAnswer(func() control.Answer { return g.compactToEnd(context.Background(), "0123", "lead") })
	id := injected(t, native, "thread/compact/start", map[string]string{"threadId": "A"})
	write(t, native, []byte(`{"id":"`+id+`","result":{}}`))
	events(t, ui, native, started("C"), compactionItem("C", "item/started"))
	running := within(t, answers)
	if running.Outcome != control.Started {
		t.Fatalf("main's answer at the bound %+v", running)
	}
	events(t, ui, native, usageEvent("A", "C", "9000", "272000"), compactionItem("C", "item/completed"), completed("C", "completed"))
	g.CompactionEnded("0123", "lead", running)
	if outcomes := g.SessionState().CompactionOutcomes; len(outcomes) != 1 || outcomes[0].Outcome != control.Done {
		t.Fatalf("the end followed by a late started: %+v", outcomes)
	}
}

// Whichever final outcome came first, a started after it is dropped, and one
// before it is kept: that is the order a compaction outliving the wait takes.
func TestAStartedAfterAFinalOutcomeIsDropped(t *testing.T) {
	running := control.Answer{Outcome: control.Started, Detail: "it may still be running"}
	for _, final := range []control.Answer{{Outcome: control.Done}, {Outcome: control.Failed, Detail: "the compaction was interrupted"}} {
		t.Run(final.Outcome, func(t *testing.T) {
			g, _, _, _ := setup(t)
			g.CompactionEnded("0123", "lead", final)
			g.CompactionEnded("0123", "lead", running)
			g.CompactionEnded("0456", "lead", running)
			outcomes := g.SessionState().CompactionOutcomes
			if len(outcomes) != 2 || outcomes[0].Outcome != final.Outcome || outcomes[1].Request != "0456" {
				t.Fatalf("final, then started: %+v", outcomes)
			}
			g.CompactionEnded("0456", "lead", final)
			if outcomes := g.SessionState().CompactionOutcomes; len(outcomes) != 3 || outcomes[2].Outcome != final.Outcome {
				t.Fatalf("started, then final: %+v", outcomes)
			}
		})
	}
}
