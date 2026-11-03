package pi

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/mirror"
)

// extension answers a control frame with reply and a trace with traced,
// passing each on to asked.
func extension(reply frame, traced frame, asked chan frame) func(*link, frame) {
	return func(l *link, request frame) {
		switch request.Op {
		case "control":
			asked <- request
			l.answer(request, reply)
		case "trace":
			asked <- request
			l.answer(request, traced)
		default:
			l.answer(request, frame{OK: true})
		}
	}
}

func interrupt(rig *rig) chan control.Answer {
	answered := make(chan control.Answer, 1)
	go func() {
		answered <- rig.run.carry(context.Background(), control.Request{ID: "r", Action: control.Interrupt, From: "lead"})
	}()
	return answered
}

// The wrapper names the turn it matched to its start; the extension aborts
// it and says requested; its aborted end settles the interrupt: the report
// names who stopped it, and the extension is asked to trace the same words.
func TestAPiInterruptSettlesOnTheAbortedEndOfItsTurn(t *testing.T) {
	rig := newRig(t, nil)
	asked := make(chan frame, 4)
	l := rig.client(t, "conversation", extension(frame{Outcome: control.Requested}, frame{OK: true}, asked))
	if answer := rig.run.carry(context.Background(), control.Request{ID: "idle", Action: control.Interrupt, From: "lead"}); answer.Reason != control.NoTurn {
		t.Fatalf("an idle session: %+v", answer)
	}
	askOK(t, l, frame{Op: "turn-start", Thread: "conversation", Turn: "t1"})
	answered := interrupt(rig)
	if wire := receive(t, asked); wire.Turn != "t1" || wire.Action != control.Interrupt {
		t.Fatalf("the interrupt: %+v", wire)
	}
	askOK(t, l, frame{Op: "turn-end", Thread: "conversation", Turn: "t1", Outcome: "aborted", Text: "half"})
	if end := receive(t, rig.ends); end.Answer != "lead interrupted this turn with rewake interrupt" {
		t.Fatalf("the report: %+v", end)
	}
	if trace := receive(t, asked); trace.Op != "trace" || trace.Text != "lead interrupted this turn with rewake interrupt" {
		t.Fatalf("the trace: %+v", trace)
	}
	if answer := receive(t, answered); answer.Outcome != control.Done {
		t.Fatalf("the answer: %+v", answer)
	}
}

// A turn that ended otherwise keeps its report, and the asker hears no turn
// was running; a trace the extension could not put down says so.
func TestAPiInterruptIsAnsweredByHowItsTurnEnded(t *testing.T) {
	rig := newRig(t, nil)
	asked := make(chan frame, 4)
	l := rig.client(t, "conversation", extension(frame{Outcome: control.Requested}, frame{Error: "no session"}, asked))
	askOK(t, l, frame{Op: "turn-start", Thread: "conversation", Turn: "t1"})
	answered := interrupt(rig)
	receive(t, asked)
	askOK(t, l, frame{Op: "turn-end", Thread: "conversation", Turn: "t1", Outcome: "answered", Text: "done anyway"})
	if end := receive(t, rig.ends); end.Answer != "done anyway" {
		t.Fatalf("the report: %+v", end)
	}
	if answer := receive(t, answered); answer.Outcome != control.Refused || answer.Reason != control.NoTurn {
		t.Fatalf("an answered turn: %+v", answer)
	}
	askOK(t, l, frame{Op: "turn-start", Thread: "conversation", Turn: "t2"})
	answered = interrupt(rig)
	receive(t, asked)
	askOK(t, l, frame{Op: "turn-end", Thread: "conversation", Turn: "t2", Outcome: "aborted"})
	receive(t, asked)
	if answer := receive(t, answered); answer.Outcome != control.Failed || answer.Detail != control.TraceLost {
		t.Fatalf("a trace that failed: %+v", answer)
	}
}

// No end within the settle wait leaves the outcome open.
func TestAPiInterruptWithoutAnEndStaysOpen(t *testing.T) {
	rig := newRig(t, nil)
	asked := make(chan frame, 4)
	l := rig.client(t, "conversation", extension(frame{Outcome: control.Requested}, frame{OK: true}, asked))
	askOK(t, l, frame{Op: "turn-start", Thread: "conversation", Turn: "t1"})
	start := time.Now()
	var answer control.Answer
	select {
	case answer = <-interrupt(rig):
	case <-time.After(2 * control.SettleWait):
		t.Fatal("the interrupt was not answered")
	}
	if answer.Outcome != control.Failed || !answer.Open || answer.Detail != control.NotSettled || time.Since(start) < control.SettleWait-100*time.Millisecond {
		t.Fatalf("no end in time: %+v after %s", answer, time.Since(start))
	}
}

// The extension's code and its native result are worded here, pi's error
// text for a conversation too short to compact among them.
func TestAPiControlReplyIsWordedHere(t *testing.T) {
	cases := []struct {
		action string
		reply  frame
		want   control.Answer
	}{
		{control.Compact, frame{Outcome: control.Refused, Code: control.CodeTurn}, control.Answer{Outcome: control.Refused, Reason: control.InTurn}},
		{control.Interrupt, frame{Outcome: control.Refused, Code: control.CodeIdle}, control.Answer{Outcome: control.Refused, Reason: control.NoTurn}},
		{control.Compact, frame{Outcome: control.Refused, Code: control.CodeBusy}, control.Answer{Outcome: control.Refused, Reason: control.Busy}},
		{control.Compact, frame{Outcome: control.Failed, Code: control.CodeClosed}, control.Answer{Outcome: control.Failed, Detail: "the conversation closed before its control outcome", Open: true}},
		{control.Compact, frame{Outcome: control.Failed, Native: json.RawMessage(`{"error":"Session too small to compact"}`)}, control.Answer{Outcome: control.Refused, Reason: control.NothingToCompact}},
		{control.Compact, frame{Outcome: control.Failed, Native: json.RawMessage(`{"error":"provider down"}`)}, control.Answer{Outcome: control.Failed, Reason: control.CutShort, Detail: "native compaction failed or was interrupted"}},
		{control.Interrupt, frame{Outcome: control.Failed, Native: json.RawMessage(`{"error":"x"}`)}, control.Answer{Outcome: control.Failed, Detail: "native interrupt could not be requested"}},
	}
	for _, c := range cases {
		if got := worded(c.action, c.reply); got != c.want {
			t.Errorf("%s %+v: %+v, want %+v", c.action, c.reply, got, c.want)
		}
	}
}

// The face's shift+tab cycles the levels read here off the last sample's
// model: those its map does not take away, xhigh and max where it names them.
func TestTheThinkingCycleIsReadOffTheModelHere(t *testing.T) {
	rig := newRig(t, func(r *run) { r.mirror = mirror.New() })
	asked := make(chan frame, 2)
	l := rig.client(t, "conversation", func(l *link, request frame) {
		if request.Op == "thinking" {
			asked <- request
		}
		l.answer(request, frame{OK: true})
	})
	thinker := map[string]any{"id": "m", "provider": "p", "reasoning": true, "thinkingLevelMap": map[string]any{"minimal": nil, "xhigh": "max"}}
	askOK(t, l, frame{Op: "state", Thread: "conversation", State: &sample{Sequence: 2, Idle: true, Native: nativeOf(map[string]any{"model": thinker, "thinkingLevel": "low"})}})
	if err := rig.run.input(t.Context(), mirror.Input{Kind: mirror.InputKindThinking}); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, asked).Levels; len(got) != 5 || got[0] != "off" || got[1] != "low" || got[4] != "xhigh" {
		t.Fatalf("the levels: %v", got)
	}
	plain := map[string]any{"id": "plain", "provider": "p", "reasoning": false}
	askOK(t, l, frame{Op: "state", Thread: "conversation", State: &sample{Sequence: 3, Idle: true, Native: nativeOf(map[string]any{"model": plain})}})
	if err := rig.run.input(t.Context(), mirror.Input{Kind: mirror.InputKindThinking}); err == nil {
		t.Fatal("a model that does not reason cycled its thinking")
	}
}
