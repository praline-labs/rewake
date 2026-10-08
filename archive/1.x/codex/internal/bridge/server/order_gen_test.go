package server_test

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/inbox"
)

// The order test's generated part (docs/mail-bridge-checks.md): every order
// of one list of events over two turns, each run in a fresh rig. Turn T reads
// a task from web through the tool; its completion comes once, never, or
// twice; T's end is captured and journaled; turn T+1 reads from the shell or
// through the tool; and a restarted server asks for T's call again. The
// written-out cases, which pause one process inside another's step, are in
// order_test.go and order_marks_test.go.

// orderEvent is one kind of event, and the events that must come before it.
type orderEvent struct {
	name  string
	after []string
}

// orderEvents is the one list the orders are generated from: an event kind
// added here is in the next run.
var orderEvents = []orderEvent{
	{"call", nil},
	{"completion", []string{"call"}},
	{"end", []string{"call"}},
	{"journal", []string{"end"}},
	{"shell read", []string{"end"}},
	{"next call", []string{"end"}},
	{"restart", []string{"call"}},
}

// orders lists every order of events that keeps each one after those it
// needs.
func orders(events []orderEvent) [][]string {
	var all [][]string
	var walk func(done []string, left []orderEvent)
	walk = func(done []string, left []orderEvent) {
		if len(left) == 0 {
			all = append(all, append([]string(nil), done...))
			return
		}
		for i, event := range left {
			ready := true
			for _, need := range event.after {
				ready = ready && contains(done, need)
			}
			if !ready {
				continue
			}
			rest := append(append([]orderEvent(nil), left[:i]...), left[i+1:]...)
			walk(append(done, event.name), rest)
		}
	}
	walk(nil, events)
	return all
}

func contains(list []string, name string) bool {
	for _, item := range list {
		if item == name {
			return true
		}
	}
	return false
}

// withCompletion is an order with its completion lost, or repeated last.
func withCompletion(order []string, times int) []string {
	var kept []string
	for _, event := range order {
		if event != "completion" || times > 0 {
			kept = append(kept, event)
		}
	}
	if times == 2 {
		kept = append(kept, "completion")
	}
	return kept
}

func TestEveryOrderOfTwoTurnsKeepsTheEndsBoundary(t *testing.T) {
	t.Parallel()
	generated := orders(orderEvents)
	var cases [][]string
	for _, order := range generated {
		cases = append(cases, order, withCompletion(order, 2))
		if order[1] == "completion" {
			// A lost completion is the same whatever comes after it;
			// one order of each is enough.
			cases = append(cases, withCompletion(order, 0))
		}
	}
	t.Logf("%d orders generated, %d cases with lost and repeated completions", len(generated), len(cases))
	for _, order := range cases {
		t.Run(strings.Join(order, ", "), func(t *testing.T) {
			t.Parallel()
			runOrder(t, order)
		})
	}
}

func runOrder(t *testing.T, order []string) {
	r := newRig(t, bridge.CodexTransport)
	r.start()
	id := r.letter("a task from web")
	started := boottime.Now()
	r.nextTurn()
	var first toolCall
	var boundary *inbox.ReadBoundary
	acknowledged, completions := false, 0
	readAt := uint64(0)
	for _, event := range order {
		switch event {
		case "call":
			first = r.call("inbox")
			if first.ended || first.result.IsError || !strings.Contains(first.result.text(), "a task from web") {
				t.Fatalf("T's read: %+v", first.result)
			}
		case "completion":
			completions++
			wasUnread := r.unread(id)
			err := r.complete(first, true)
			switch {
			case boundary == nil && completions == 1 && err != nil:
				t.Fatalf("a completion inside T: %v", err)
			case boundary != nil && completions == 1 && err == nil && wasUnread:
				t.Fatal("an acknowledgment began writing after T's end was noted")
			}
			if boundary == nil && completions == 1 {
				acknowledged = true
				readAt = r.clock.Snapshot().Through
			}
		case "end":
			boundary = r.endTurn()
			if acknowledged && boundary.Through < readAt {
				t.Fatalf("T's boundary %d leaves out the acknowledgment at %d", boundary.Through, readAt)
			}
		case "journal":
			if err := r.journal(boundary, started); err != nil {
				t.Fatalf("T's journal: %v", err)
			}
		case "shell read":
			if out, err := r.shell("inbox"); err != nil {
				t.Fatalf("T+1's shell read: %v: %s", err, out)
			}
		case "next call":
			r.nextTurn()
			next := r.call("inbox")
			if next.ended || next.result.IsError {
				t.Fatalf("T+1's read: %+v", next.result)
			}
			_ = r.complete(next, true)
		case "restart":
			again := startServer(t, r.env())
			again.initialize(t)
			result, _ := again.call(t, first.words, r.meta(first.turn, first.id))
			if !result.IsError || !strings.Contains(result.text(), "issued no ticket") {
				t.Fatalf("a restarted server ran T's call again: %+v", result)
			}
		}
	}
	// Whatever read the letter after T's end, T's end answers web only if T
	// read it: no read of a later mailbox holder extends the boundary.
	readLater := contains(order, "shell read") || contains(order, "next call")
	switch {
	case acknowledged && r.unread(id), !acknowledged && !readLater && !r.unread(id):
		t.Fatalf("acknowledged in T %v, read later %v, unread %v", acknowledged, readLater, r.unread(id))
	case readLater && r.unread(id):
		t.Fatal("T+1 did not read the letter T left unread")
	}
	if !acknowledged && readAt != 0 {
		t.Fatal("a boundary taken without an acknowledgment")
	}
	if got := report(r); contains(order, "journal") && acknowledged && got != inbox.Finished {
		t.Fatalf("T read the task and its end reported %q", got)
	}
	if got := report(r); !acknowledged && got != "" && !contains(order, "next call") {
		t.Fatalf("T read nothing and its end reported %q", got)
	}
}
