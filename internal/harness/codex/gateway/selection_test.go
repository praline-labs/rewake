package gateway

import (
	"context"
	"fmt"
	"slices"
	"testing"
)

// steps records what a state tells of its selections.
func steps(s *state) *[]string {
	var told []string
	s.selection = func(step Selection) { told = append(told, step.Step+":"+step.Thread) }
	return &told
}

// Each step of the terminal's choice of a conversation is told: the
// admission with the target it names, the answer that sets the primary, and
// every way the primary is left empty; another thread's request is told as
// such (docs/mail-bridge-channel-codex.md#selecting-a-conversation).
func TestEverySelectionStepIsTold(t *testing.T) {
	s := newState("epoch", 1)
	told := steps(&s)
	expect := func(want ...string) {
		t.Helper()
		if !slices.Equal(*told, want) {
			t.Fatalf("told %q, want %q", *told, want)
		}
		*told = nil
	}
	req(t, &s, startA)
	expect("admitted:")
	answer(t, &s, "1", "A")
	expect("selected:A")
	resume := func(id int, thread string) string {
		return fmt.Sprintf(`{"id":%d,"method":"thread/resume","params":{"threadId":%q,"config":{},"runtimeWorkspaceRoots":[]}}`, id, thread)
	}
	req(t, &s, resume(2, "B"))
	expect("admitted:B")
	s.response(metadata(t, `{"id":2,"error":{"code":1,"message":"no"}}`))
	expect("failed:")
	req(t, &s, resume(3, "B"))
	req(t, &s, resume(4, "C"))
	expect("admitted:B", "admitted:C")
	answer(t, &s, "3", "B")
	expect()
	answer(t, &s, "4", "C")
	expect("selected:C")
	req(t, &s, `{"id":"tui-dynamic-1","method":"thread/start","params":{"threadSource":"user","runtimeWorkspaceRoots":[]}}`)
	expect("thread:")
	req(t, &s, `{"id":5,"method":"thread/resume","params":{"threadId":"A","config":{}}}`)
	expect("failed:", "thread:A")
	req(t, &s, resume(6, "A"))
	s.response(metadata(t, `{"id":6,"result":{"thread":{"id":"A","canAcceptDirectInput":false}}}`))
	expect("admitted:A", "failed:")
}

// A fork the terminal makes primary is a selection whose target is known
// only at its answer.
func TestAForkIsASelection(t *testing.T) {
	s := newState("epoch", 1)
	req(t, &s, startA)
	answer(t, &s, "1", "A")
	told := steps(&s)
	req(t, &s, `{"id":2,"method":"thread/fork","params":{"threadId":"A","threadSource":"user","config":{},"runtimeWorkspaceRoots":[]}}`)
	answer(t, &s, "2", "B")
	req(t, &s, `{"id":3,"method":"thread/unsubscribe","params":{"threadId":"A"}}`)
	s.response(metadata(t, `{"id":3,"result":{"status":"unsubscribed"}}`))
	if !slices.Equal(*told, []string{"admitted:", "selected:B"}) || s.Thread != "B" {
		t.Fatalf("told %q, primary %q", *told, s.Thread)
	}
}

// Whenever the primary is set a selection is told with it, and whenever it
// is emptied a step that ends the old one is: the channel's conversation is
// never another than the gateway's.
func TestThePrimaryNeverMovesUntold(t *testing.T) {
	s := newState("epoch", 1)
	var last Selection
	s.selection = func(step Selection) { last = step }
	requests := []string{
		startA,
		`{"id":2,"method":"thread/resume","params":{"threadId":"B","config":{},"runtimeWorkspaceRoots":[]}}`,
		`{"id":3,"method":"turn/start","params":{"threadId":"Z"}}`,
		`{"id":4,"method":"thread/futureControl","params":{}}`,
		`{"id":5,"method":"thread/archive","params":{"threadId":"B"}}`,
	}
	for i, raw := range requests {
		req(t, &s, raw)
		answer(t, &s, fmt.Sprint(i+1), []string{"A", "B", "", "", ""}[i])
		switch {
		case s.Ready && (last.Step != SelectionSelected || last.Thread != s.Thread):
			t.Fatalf("request %d: primary %q, last told %+v", i+1, s.Thread, last)
		case !s.Ready && last.Step == SelectionSelected:
			t.Fatalf("request %d: the primary was emptied untold", i+1)
		}
	}
}

// Only the connection that holds the primary tells its steps: a picker's or a
// second terminal's state selects nothing for the channel, and neither does
// the owner once a second owner makes the primary ambiguous or its
// connection has ended.
func TestOnlyThePrimarysConnectionTellsItsSteps(t *testing.T) {
	var told []string
	g := New(Config{Selection: func(step Selection) { told = append(told, step.Step+":"+step.Thread) }})
	connect := func() (*connection, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		c := &connection{owner: g, ctx: ctx, cancel: cancel, state: newState("epoch", 1)}
		g.conns[c] = true
		return c, cancel
	}
	owner, ended := connect()
	other, _ := connect()
	g.current, g.owners[owner] = owner, true
	g.selections(other)(Selection{Step: SelectionAdmitted, Thread: "B"})
	g.selections(owner)(Selection{Step: SelectionAdmitted, Thread: "A"})
	g.owners[other] = true
	g.selections(owner)(Selection{Step: SelectionSelected, Thread: "A"})
	delete(g.owners, other)
	ended()
	g.selections(owner)(Selection{Step: SelectionFailed})
	if !slices.Equal(told, []string{"admitted:A"}) {
		t.Fatalf("told %q, want only the owner's admission", told)
	}
}
