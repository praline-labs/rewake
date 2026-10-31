package pi

import (
	"bufio"
	"context"
	"net"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mirror"
)

// hello connects as the extension does and returns hello's answer.
func hello(t *testing.T, rig *rig, handle func(*link, frame)) (*link, frame) {
	t.Helper()
	conn, err := net.Dial("unix", rig.run.socket)
	if err != nil {
		t.Fatal(err)
	}
	l := newLink(conn)
	t.Cleanup(l.close)
	go l.serve(bufio.NewReader(conn), func(got frame) { handle(l, got) })
	reply, err := l.ask(t.Context(), frame{Op: "hello", Protocol: protocolVersion, State: &sample{Sequence: 1, Activity: "idle"}, Thread: "conversation"})
	if err != nil || !reply.OK {
		t.Fatalf("hello: %+v, %v", reply, err)
	}
	receive(t, rig.live)
	return l, reply
}

func items(model *mirror.Model) []mirror.Item {
	var out []mirror.Item
	for _, line := range model.Snapshot() {
		if line.Op == mirror.LineOpItem {
			out = append(out, *line.Item)
		}
	}
	return out
}

// Every pi worker is drawn by the face, --dev or not: the space host reads
// it from this interface alone.
func TestEveryPiWorkerIsDrawnByTheFace(t *testing.T) {
	if _, ok := any(harness{}).(adapter.FacedWorkersHarness); !ok {
		t.Fatal("pi's harness does not draw its workers")
	}
}

// A run the face draws says so in hello's answer and takes the extension's
// events into the model, a kind it does not know left out; one it does not
// draw says nothing and takes none.
func TestTheMirrorReachesTheModelOfARunTheFaceDraws(t *testing.T) {
	model := mirror.New()
	rig := newRig(t, func(r *run) { r.mirror = model })
	l, reply := hello(t, rig, func(l *link, got frame) { l.answer(got, frame{OK: true}) })
	if !reply.Mirror {
		t.Fatal("the hello answer of a drawn run does not say mirror")
	}
	askOK(t, l, frame{Op: "mirror", Thread: "conversation", Events: []mirrorEvent{
		{Kind: "turn", Turn: "t", State: "working"},
		{Kind: "tool", ID: "c1", Turn: "t", Name: "Bash"},
		{Kind: "input", ID: "c1", Text: `{"command":"ls"}`},
		{Kind: "result", ID: "c1", Status: "refused", Text: "no", Permit: &mirrorPermit{By: "hook"}},
		{Kind: "mode", Text: "auto"},
		{Kind: "delta", ID: "b1", Turn: "t", Block: "text", Text: "Hel"},
		{Kind: "settle", ID: "b1", Turn: "t", Block: "text", Of: 1, Text: "Hello"},
		{Kind: "notice", ID: "n1", Turn: "t", Status: "error", Text: "Operation aborted"},
		{Kind: "turn", Turn: "t", State: "idle"},
	}})
	got := items(model)
	if len(got) != 3 || got[2].Kind != mirror.ItemKindNotice || got[2].Status != mirror.ItemStatusError || got[0].Status != mirror.ItemStatusRefused || got[0].Permit == nil || got[0].Permit.By != mirror.PermitByHook || got[0].Text != `{"command":"ls"}` || got[1].Text != "Hello" || got[1].Status != mirror.ItemStatusSettled {
		t.Fatalf("the model holds %+v", got)
	}
	for _, line := range model.Snapshot() {
		if line.Op == mirror.LineOpBegin && (line.Mode != nil || *line.Turn != mirror.TurnStateIdle) {
			t.Fatalf("the model's state: %+v", line)
		}
	}

	plain := newRig(t, nil)
	pl, plainReply := hello(t, plain, func(l *link, got frame) { l.answer(got, frame{OK: true}) })
	if plainReply.Mirror {
		t.Fatal("a run the face does not draw asks for its conversation")
	}
	if reply, err := pl.ask(t.Context(), frame{Op: "mirror", Thread: "conversation", Events: []mirrorEvent{{Kind: "delta", ID: "b"}}}); err != nil || reply.OK {
		t.Fatalf("a batch to a run the face does not draw: %+v %v", reply, err)
	}
}

// The face's prompt goes to the extension with its text, and its Esc as an
// interrupt, each coming back with pi's answer; a refusal carries the
// extension's reason.
func TestAFacePromptAndEscReachTheExtension(t *testing.T) {
	model := mirror.New()
	rig := newRig(t, func(r *run) {
		r.mirror = model
		model.Take(r.input, mirror.InputKindPrompt, mirror.InputKindInterrupt)
	})
	if err := model.Input(t.Context(), mirror.Input{Kind: mirror.InputKindPrompt, Text: "early"}); err == nil {
		t.Fatal("a prompt before the extension connected was taken")
	}
	prompts := make(chan frame, 2)
	hello(t, rig, func(l *link, got frame) {
		prompts <- got
		if got.Text == "busy" {
			l.answer(got, frame{Error: "pi is compacting its context"})
			return
		}
		l.answer(got, frame{OK: true})
	})
	if err := model.Input(t.Context(), mirror.Input{Kind: mirror.InputKindPrompt, Text: "go on"}); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, prompts); got.Op != "prompt" || got.Thread != "conversation" || got.Text != "go on" {
		t.Fatalf("the prompt on the wire: %+v", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := model.Input(ctx, mirror.Input{Kind: mirror.InputKindPrompt, Text: "busy"}); err == nil || !strings.Contains(err.Error(), "compacting") {
		t.Fatalf("a refused prompt: %v", err)
	}
	receive(t, prompts)
	if err := model.Input(t.Context(), mirror.Input{Kind: mirror.InputKindInterrupt}); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, prompts); got.Op != "interrupt" || got.Thread != "conversation" || got.Text != "" {
		t.Fatalf("the interrupt on the wire: %+v", got)
	}
}

// A letter rewake delivered shows in the face as a letter, with its status;
// a run the face does not draw keeps none.
func TestADeliveredLetterIsMirroredAsALetter(t *testing.T) {
	model := mirror.New()
	rig := newRig(t, func(r *run) { r.mirror = model })
	hello(t, rig, func(l *link, got frame) { l.answer(got, frame{OK: true}) })
	result := rig.run.Deliver(t.Context(), mail.Message{ID: "m1", From: "main", Kind: mail.Task, Text: "fix <it>", DeliveryThread: "conversation"})
	if result.State != mail.Delivered {
		t.Fatalf("delivery: %+v", result)
	}
	got := items(model)
	if len(got) != 1 || got[0].Origin == nil || *got[0].Origin != "rewake" || !strings.Contains(got[0].Text, "<status>completed</status>") || !strings.Contains(got[0].Text, "<summary>Rewake: main task") || strings.Contains(got[0].Text, "<it>") {
		t.Fatalf("the letter: %+v", got)
	}
}
