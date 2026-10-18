package pi

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/control"
	"github.com/praline-labs/rewake/internal/core/mail"
)

func TestStateOrdersPushesAndPollsAndClearsUnknownUsage(t *testing.T) {
	rig := newRig(t, func(r *run) { r.epoch = "run-epoch" })
	polls := make(chan frame, 2)
	l := rig.client(t, "conversation", func(_ *link, request frame) { polls <- request })
	poll := receive(t, polls)
	if poll.Op != "state" || poll.Thread != "conversation" {
		t.Fatalf("poll: %+v", poll)
	}
	model, effort := "selected-model", "high"
	tokens, window, percent := int64(42000), int64(100000), 42.0
	value := &sample{Sequence: 3, Activity: "idle", Model: &model, Effort: &effort, Tokens: &tokens, Window: &window, Percent: &percent}
	askOK(t, l, frame{Op: "state", Thread: "conversation", State: value})
	older := *value
	older.Sequence = 2
	older.Activity = "working"
	l.answer(poll, frame{OK: true, State: &older})
	// A following acknowledged push is an ordering barrier on the same socket.
	value.Sequence = 4
	value.Tokens = nil
	value.Percent = nil
	askOK(t, l, frame{Op: "state", Thread: "conversation", State: value})
	got := rig.run.SessionState()
	if got.Epoch != "run-epoch" || !got.Fresh || got.Thread != "conversation" || *got.Activity != "idle" || *got.Model != model || *got.Effort != effort || got.ContextUsed != nil || got.FilledPercent != nil || *got.ContextWindow != window {
		t.Fatalf("state after late poll and unknown usage: %+v", got)
	}
	reply, err := l.ask(t.Context(), frame{Op: "state", Thread: "another", State: &sample{Sequence: 99, Activity: "working"}})
	if err != nil || reply.OK {
		t.Fatalf("stale conversation state: %+v %v", reply, err)
	}
	l.close()
	receive(t, rig.live)
	got = rig.run.SessionState()
	if got.Fresh || got.Coverage != "partial" {
		t.Fatalf("disconnect freshness: %+v", got)
	}
	rig.client(t, "new", nil)
	got = rig.run.SessionState()
	if !got.Fresh || got.Thread != "new" || got.Model != nil || got.ContextUsed != nil || got.Coverage != "partial" {
		t.Fatalf("reconnected state: %+v", got)
	}
}

func TestControlDirectoryCarriesFocusAndRemembersCompletion(t *testing.T) {
	rig := newRig(t, func(r *run) {
		r.controlDir = filepath.Join(filepath.Dir(r.file), "control")
		if err := control.Prepare(r.controlDir); err != nil {
			t.Fatal(err)
		}
	})
	controls := make(chan frame, 4)
	l := rig.client(t, "conversation", func(l *link, request frame) {
		if request.Op == "control" {
			controls <- request
			l.answer(request, frame{Outcome: control.Started})
			return
		}
		l.answer(request, frame{OK: true})
	})
	request := control.Request{ID: control.NewID(), Action: control.Compact, From: "main", Focus: "keep the contract", Conversation: "conversation"}
	answer, err := control.Ask(t.Context(), rig.run.controlDir, request, control.Limits{Pickup: time.Second, Outcome: time.Second, Poll: 10 * time.Millisecond})
	if err != nil || answer.Outcome != control.Started {
		t.Fatalf("control: %+v %v", answer, err)
	}
	wire := receive(t, controls)
	if wire.Action != request.Action || wire.Focus != request.Focus || wire.Request != request.ID || wire.From != request.From || wire.Thread != request.Conversation {
		t.Fatalf("control wire: %+v", wire)
	}
	completion := frame{Op: "compacted", Thread: "conversation", Sequence: 1, Request: request.ID, From: request.From}
	askOK(t, l, completion)
	askOK(t, l, completion)
	before, after := int64(42000), int64(2000)
	result := frame{Op: "compaction-result", Thread: "conversation", Request: request.ID, From: request.From, Outcome: control.Done, TokensBefore: &before, TokensAfter: &after}
	askOK(t, l, result)
	askOK(t, l, result)
	got := rig.run.SessionState()
	if *got.Compactions != 1 || len(got.CompactionEvents) != 1 || len(got.CompactionOutcomes) != 1 {
		t.Fatalf("completion dedup: %+v", got)
	}
	end := got.CompactionOutcomes[0]
	if end.Request != request.ID || end.RequestedBy != request.From || end.Outcome != control.Done || *end.TokensBefore != before || *end.TokensAfter != after {
		t.Fatalf("completion counts: %+v", end)
	}
}

func TestControlDoesNotRepeatUnknownRequestsAndCompactionDefersWake(t *testing.T) {
	rig := newRig(t, nil)
	controls := make(chan frame, 2)
	l := rig.client(t, "conversation", func(l *link, request frame) {
		if request.Op == "wake" {
			l.answer(request, frame{Pending: true, Error: "compaction in progress"})
			return
		}
		if request.Op == "control" {
			controls <- request
		}
	})
	if result := rig.run.Deliver(t.Context(), mail.Message{DeliveryThread: "conversation"}); result.State != mail.Pending {
		t.Fatalf("compacting wake: %+v", result)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	answer := rig.run.carry(ctx, control.Request{ID: control.NewID(), Action: control.Interrupt, From: "main"})
	if answer.Outcome != control.Failed || !answer.Open {
		t.Fatalf("lost control acknowledgment: %+v", answer)
	}
	receive(t, controls)
	select {
	case again := <-controls:
		t.Fatalf("control repeated: %+v", again)
	default:
	}
	l.close()
}
