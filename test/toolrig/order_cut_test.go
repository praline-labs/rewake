//go:build rewakefixture

package toolrig

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/state"
)

// The order test's written-out cases, rebuilt from
// bridge/server/order_cut_test.go on the fixture's transport.

// An acknowledgment cut midway: held between two writes of one answer, past
// both locks and its check, while the end is captured and a shell read of the
// next turn waits for the mailbox. The boundary follows the acknowledgment's
// last commit, whether its writes all landed or one failed; the shell read
// commits above it; a second acknowledgment after the note writes nothing.
func TestAnAcknowledgmentCutMidway(t *testing.T) {
	t.Parallel()
	for _, fails := range []bool{false, true} {
		name := "its writes land"
		if fails {
			name = "its next write fails"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
			r.start()
			first, second := r.letter("the first letter of one answer"), r.letter("the second letter of one answer")
			r.nextTurn()
			c := r.call("inbox")
			if c.result.IsError || !strings.Contains(c.result.text(), "the second letter") {
				t.Fatalf("one answer for both: %+v", c.result)
			}
			later := r.letter("a letter for the next turn")

			held, release := make(chan struct{}), make(chan struct{})
			// Released at the latest when the test ends, so a test that fails
			// while the acknowledgment is held does not hang in its cleanup.
			free := sync.OnceFunc(func() { close(release) })
			t.Cleanup(free)
			plan, writes := &wrapperPlan{}, 0
			plan.pause = func(op, _ string) {
				if op == state.OpRead || op == state.OpStep {
					return
				}
				writes++
				if writes == 2 {
					close(held)
					<-release
				}
			}
			if fails {
				plan.fail = 2
			}
			r.plan(plan)
			acknowledged := make(chan error, 1)
			go func() { acknowledged <- r.complete(c, true) }()
			select {
			case <-held:
			case <-time.After(10 * time.Second):
				t.Fatal("the acknowledgment never reached its second write")
			}
			captured := make(chan *inbox.ReadBoundary, 1)
			go func() { captured <- r.endTurn() }()
			shellRead := make(chan string, 1)
			go func() { out, _ := r.shell("inbox"); shellRead <- out }()
			// Held past the acknowledgment's budget of two seconds: no
			// budget is checked between its writes.
			select {
			case <-captured:
				t.Fatal("the end was captured past a writing acknowledgment")
			case <-shellRead:
				t.Fatal("a shell read went past a writing acknowledgment")
			case <-time.After(2500 * time.Millisecond):
			}
			free()
			ackErr := <-acknowledged
			boundary := <-captured
			out := <-shellRead
			if fails != (ackErr != nil) {
				t.Fatalf("the acknowledgment answered %v", ackErr)
			}
			if !strings.Contains(out, "a letter for the next turn") || r.unread(later) {
				t.Fatalf("the shell read: %q", out)
			}
			after := r.clock.Snapshot()
			if boundary == nil || boundary.Through >= after.Through {
				t.Fatalf("the boundary %+v is not below the shell read's commit %+v", boundary, after)
			}
			// A second record of the same result, after the note, writes
			// nothing: what the first left unread stays so.
			unread := []bool{r.unread(first), r.unread(second)}
			if !fails && (unread[0] || unread[1]) {
				t.Fatalf("a whole acknowledgment left %v unread", unread)
			}
			r.plan(&wrapperPlan{})
			_ = r.complete(c, true)
			if again := []bool{r.unread(first), r.unread(second)}; again[0] != unread[0] || again[1] != unread[1] {
				t.Fatalf("an acknowledgment after the note wrote: %v, then %v", unread, again)
			}
		})
	}
}

// An Esc nobody heard: a pending step of T saved, its child dead before the
// mark, T ended with no end on record; in T+1 the retry of that step, from
// the shell or through the tool, answers not marked, and T+1's end is no
// interim by it.
func TestAnEscNobodyHeard(t *testing.T) {
	t.Parallel()
	for _, through := range []string{"the shell", "the tool"} {
		t.Run(through, func(t *testing.T) {
			t.Parallel()
			r := newRig(t)
			r.start()
			r.letter("a task from web")
			r.nextTurn()
			read := r.call("inbox")
			if err := r.complete(read, true); err != nil {
				t.Fatal(err)
			}
			_, mark := markSteps(t, "child", taskRead, toolMark)
			r.fault = "child:crash=" + strconv.Itoa(mark)
			r.start()
			pending := r.call("pending", "the work goes on")
			token := retryWords.FindStringSubmatch(pending.result.text())
			if token == nil {
				t.Fatalf("the cut pending step: %q", pending.result.text())
			}
			// T ends unheard: nothing is captured or journaled.
			r.fault = ""
			r.start()
			started := boottime.Now()
			r.nextTurn()
			var answer string
			if through == "the shell" {
				answer, _ = r.shell("retry", token[1])
			} else {
				answer = r.call("retry", token[1]).result.text()
			}
			if !strings.Contains(answer, "not marked") {
				t.Fatalf("the retry in T+1: %q", answer)
			}
			boundary := r.endTurn()
			if err := r.journal(boundary, started); err != nil {
				t.Fatal(err)
			}
			if got := report(r); got != inbox.Finished {
				t.Fatalf("T+1's end reported %q", got)
			}
		})
	}
}
