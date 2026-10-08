//go:build rewakefixture

package toolrig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness/fixture"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
)

// The order test's second generated part, rebuilt from
// bridge/server/order_call_test.go on the fixture's transport: one call of
// turn T taken apart into its steps — the harness's record of it, its
// request, the child's confirmation of the ticket, the child's exit, the
// result recorded once and again — against T's end taken apart into its
// capture, the turn's end as the harness reports it, and the journal. The
// endpoint is held before it starts the child — the old rig's server, held
// there — and the child after it confirmed, so each step lands where the
// order puts it.

// callEvents is the one list these orders are generated from.
var callEvents = []orderEvent{
	{"observe", nil},
	{"request", nil},
	{"confirm", []string{"observe", "request"}},
	{"exit", []string{"confirm"}},
	{"completion", []string{"exit"}},
	{"repeat", []string{"completion"}},
	{"capture", []string{"observe"}},
	{"turn completed", []string{"capture"}},
	{"journal", []string{"capture"}},
}

// How the first acknowledgment of the call ends.
const (
	ackLands  = "it lands"
	ackBudget = "its budget runs out"
	ackFails  = "its first write fails"
)

func TestEveryOrderOfACallAgainstItsTurnsEnd(t *testing.T) {
	t.Parallel()
	generated := orders(callEvents)
	type orderCase struct {
		order []string
		first string
	}
	var cases []orderCase
	for _, order := range generated {
		cases = append(cases, orderCase{order, ackLands})
		// The first acknowledgment fails differently only where it would
		// otherwise write: its call ticketed and its result recorded
		// before the end was captured.
		if before(order, "completion", "capture") {
			cases = append(cases, orderCase{order, ackBudget}, orderCase{order, ackFails})
		}
	}
	lost := orders(without(callEvents, "completion", "repeat"))
	for _, order := range lost {
		cases = append(cases, orderCase{order, ackLands})
	}
	t.Logf("%d orders generated, %d with the result lost, %d cases", len(generated), len(lost), len(cases))
	for _, c := range cases {
		t.Run(c.first+": "+strings.Join(c.order, ", "), func(t *testing.T) {
			t.Parallel()
			runCallOrder(t, c.order, c.first)
		})
	}
}

func without(events []orderEvent, names ...string) []orderEvent {
	var kept []orderEvent
	for _, event := range events {
		if !contains(names, event.name) {
			kept = append(kept, event)
		}
	}
	return kept
}

// before says whether a comes before b in order; false when either is absent.
func before(order []string, a, b string) bool {
	for _, event := range order {
		switch event {
		case a:
			return contains(order, b)
		case b:
			return false
		}
	}
	return false
}

func runCallOrder(t *testing.T, order []string, first string) {
	r := newRig(t)
	child := newHold(t)
	r.fault = child.specAt("child", "confirmed")
	r.start()
	start := r.holdStep("start")
	id := r.letter("a task from web")
	started := boottime.Now()
	c := toolCall{id: "call-1", turn: r.nextTurn(), words: []string{"inbox"}}
	answered := make(chan callResult, 1)
	t.Cleanup(child.free)

	var observed, requested, asked, issued, confirmed, captured, acknowledged bool
	var boundary *inbox.ReadBoundary
	readAt := uint64(0)
	// ticket waits, once the call was both heard and asked for, for the
	// endpoint to be held before its child, or for its refusal.
	ticket := func() {
		if !observed || !requested || asked {
			return
		}
		asked = true
		issued = start.heldOr(t, answered, &c.result)
	}
	for _, event := range order {
		switch event {
		case "observe":
			r.observe(c.turn, c.id, c.words)
			observed = true
			ticket()
		case "request":
			tool, arguments := toolOf(c.words)
			go func() {
				answer, _ := r.ask(command{Op: "request", Turn: c.turn, Call: c.id, Tool: tool, Arguments: arguments})
				answered <- callResult{Texts: answer.Texts, IsError: answer.IsError}
			}()
			requested = true
			ticket()
		case "confirm":
			if issued {
				start.free()
				confirmed = child.heldOr(t, answered, &c.result)
			}
		case "exit":
			if confirmed {
				child.release(t)
				c.result = awaitAnswer(t, answered)
				if c.result.IsError || !strings.Contains(c.result.text(), "a task from web") {
					t.Fatalf("T's read: %+v", c.result)
				}
			}
		case "completion":
			wasUnread := r.unread(id)
			err := completeFirst(r, c, first)
			switch {
			case captured && wasUnread && !r.unread(id):
				t.Fatal("an acknowledgment began writing after T's end was noted")
			case !captured && confirmed && first == ackLands && (err != nil || r.unread(id)):
				t.Fatalf("a whole result inside T read nothing: %v", err)
			case first != ackLands && (err == nil || !r.unread(id)):
				t.Fatalf("an acknowledgment that %s answered %v and read the letter: %v", first, err, !r.unread(id))
			}
			if !captured && confirmed && first == ackLands {
				acknowledged = true
				readAt = r.clock.Snapshot().Through
			}
		case "repeat":
			wasUnread := r.unread(id)
			if err := r.complete(c, true); err != nil {
				t.Fatalf("the repeat: %v", err)
			}
			var started error
			select {
			case err := <-r.acknowledged:
				started = fmt.Errorf("it answered %v", err)
			case <-time.After(300 * time.Millisecond):
			}
			if wasUnread != r.unread(id) {
				t.Fatalf("a repeated result changed what was read (%v)", started)
			}
			if started != nil {
				t.Fatalf("a repeated result started an acknowledgment: %v", started)
			}
		case "capture":
			boundary, _ = r.endCapture()
			captured = true
			if acknowledged && boundary.Through < readAt {
				t.Fatalf("T's boundary %d leaves out the acknowledgment at %d", boundary.Through, readAt)
			}
		case "turn completed":
			// The harness reports the end the rig already captured: the
			// adapter takes that capture and tells the endpoint.
			r.endTurn()
		case "journal":
			if err := r.journal(boundary, started); err != nil {
				t.Fatalf("T's journal: %v", err)
			}
		}
	}
	// A ticket says its call belongs to a turn still open: issued exactly
	// when the call was heard and asked for before the end was captured.
	if want := before(order, "observe", "capture") && before(order, "request", "capture"); issued != want {
		t.Fatalf("ticket issued %v, the order allows %v", issued, want)
	}
	if !issued && (!c.result.IsError || !strings.Contains(c.result.text(), "issued no ticket")) {
		t.Fatalf("a call refused its ticket answered: %+v", c.result)
	}
	if acknowledged == r.unread(id) {
		t.Fatalf("acknowledged in T %v, unread %v", acknowledged, r.unread(id))
	}
	got := report(r)
	switch {
	case contains(order, "journal") && acknowledged && got != inbox.Finished:
		t.Fatalf("T read the task and its end reported %q", got)
	case !acknowledged && got != "":
		t.Fatalf("T read nothing and its end reported %q", got)
	}
}

// completeFirst records the call's result the first time, its
// acknowledgment ending as first says.
func completeFirst(r *rig, c toolCall, first string) error {
	r.t.Helper()
	switch first {
	case ackBudget:
		token, err := receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(fixture.Transport, thread, c.id))
		if err != nil {
			r.t.Fatalf("the call's binding: %v", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		release, err := receipt.Lock(ctx, r.dir, "api", r.self.Epoch(), token)
		if err != nil {
			r.t.Fatal(err)
		}
		defer release()
	case ackFails:
		r.plan(&wrapperPlan{fail: 1})
		defer r.plan(&wrapperPlan{})
	}
	return r.complete(c, true)
}

// heldOr waits until the process is held, or until the call answers without
// reaching the hold: false then, with the answer in result.
func (h hold) heldOr(t *testing.T, answered chan callResult, result *callResult) bool {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(string(h), "held")); err == nil {
			return true
		}
		select {
		case *result = <-answered:
			return false
		default:
		}
	}
	t.Fatal("the process was neither held nor answered")
	return false
}

func awaitAnswer(t *testing.T, answered chan callResult) callResult {
	t.Helper()
	select {
	case result := <-answered:
		return result
	case <-time.After(30 * time.Second):
		t.Fatal("the call did not answer")
		return callResult{}
	}
}
