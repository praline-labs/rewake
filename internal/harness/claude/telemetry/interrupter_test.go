package telemetry

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
)

// The session that interrupted a turn is taken from an aborted turn's end
// only, and only when it is a session name.
func TestTheInterrupterDecodesOnlyFromAnAbortedEnd(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{`{"plugin_event":"turn.complete","turn_id":"t1","reason":"aborted","by":"main-claude"}`, "main-claude"},
		{`{"plugin_event":"turn.complete","turn_id":"t1","reason":"aborted"}`, ""},
		{`{"plugin_event":"turn.complete","turn_id":"t1","reason":"answer","by":"main-claude"}`, ""},
		{`{"plugin_event":"turn.complete","turn_id":"t1","reason":"aborted","by":"Main Claude\n"}`, ""},
		{`{"plugin_event":"turn.start","turn_id":"t1","by":"main-claude"}`, ""},
	} {
		event, ok := DecodePlugin([]byte(tc.raw))
		if !ok || event.By != tc.want {
			t.Errorf("DecodePlugin(%s) = %+v, %v; want by %q", tc.raw, event, ok, tc.want)
		}
	}
}

// A turn a main interrupted is published as stopped with the main's name
// instead of a person's, and the next notice is to say so once.
func TestATurnAMainInterruptedSaysWho(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	collector := NewCollector(path)
	var out published
	collector.ReportTurns(out.handler())
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	if by, _ := collector.Interrupter(); by != "" {
		t.Fatalf("an interrupter before any turn: %q", by)
	}
	Send(path, Event{Kind: TurnStart, Turn: "t1", At: 1})
	Send(path, Event{Kind: TurnComplete, Turn: "t1", Reason: ReasonAborted, By: "lead", At: 2})
	waitFor(t, "the stopped outcome", func() bool { return len(out.all()) == 1 })
	got := out.all()[0]
	if got.Kind != inbox.Stopped || got.Text != "lead interrupted this turn with rewake interrupt" || got.ID != "claude/t1" {
		t.Fatalf("published %+v", got)
	}
	by, mark := collector.Interrupter()
	if by != "lead" {
		t.Fatalf("interrupter %q", by)
	}
	collector.Told(mark)
	if by, _ := collector.Interrupter(); by != "" {
		t.Fatalf("told once and still %q", by)
	}

	// A person's Esc is the person's, and says nothing about a main.
	Send(path, Event{Kind: TurnStart, Turn: "t2", At: 3})
	Send(path, Event{Kind: TurnComplete, Turn: "t2", Reason: ReasonAborted, At: 4})
	waitFor(t, "the second outcome", func() bool { return len(out.all()) == 2 })
	if got := out.all()[1]; got.Text != StoppedText {
		t.Fatalf("a person's stop reads %q", got.Text)
	}
	if by, _ := collector.Interrupter(); by != "" {
		t.Fatalf("a person's stop set %q", by)
	}
}

// The line speaks of the previous turn: a turn that starts before any notice
// lays it aside, and a notice carrying an older mark does not clear a newer one.
func TestTheInterruptLineIsForTheNextNoticeOnly(t *testing.T) {
	path := filepath.Join(socketDir(t), "s.obs")
	collector := NewCollector(path)
	if err := collector.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer collector.Close()
	interrupter := func() string { by, _ := collector.Interrupter(); return by }

	Send(path, Event{Kind: TurnComplete, Turn: "t1", Reason: ReasonAborted, By: "lead", At: 1})
	waitFor(t, "the first mark", func() bool { return interrupter() == "lead" })
	_, first := collector.Interrupter()
	Send(path, Event{Kind: TurnStart, Turn: "t2", At: 2})
	waitFor(t, "a typed turn laying it aside", func() bool { return interrupter() == "" })

	Send(path, Event{Kind: TurnComplete, Turn: "t2", Reason: ReasonAborted, By: "lead", At: 3})
	waitFor(t, "the second mark", func() bool { return interrupter() == "lead" })
	collector.Told(first)
	if interrupter() != "lead" {
		t.Fatal("an older notice cleared a newer interruption")
	}
}
