//go:build rewakefixture

package toolrig

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/harness/fixture"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/receipt"
)

// One call's steps as the program takes them, and what the rig reads of them.

// callResult is the answer a call got: its texts, and whether it failed.
type callResult struct {
	Texts   []string
	IsError bool
}

// text is the whole answer: stdout, stderr and the exit status in turn.
func (c callResult) text() string { return strings.Join(c.Texts, "") }

// toolCall is one call as the program makes it, and what came of it.
type toolCall struct {
	id, turn string
	words    []string
	result   callResult
	ended    bool
}

// call makes one tool call of the current turn: the program's record of it,
// its request, the answer.
func (r *rig) call(words ...string) toolCall {
	r.t.Helper()
	r.calls++
	c := toolCall{id: "call-" + strconv.Itoa(r.calls), turn: r.turnID(), words: words}
	if c.ended = !r.observed(c.turn, c.id, words); c.ended {
		return c
	}
	c.result, c.ended = r.request(c.turn, c.id, words)
	return c
}

// observe has the program report a call it observed.
func (r *rig) observe(turn, id string, words []string) {
	r.t.Helper()
	if !r.observed(turn, id, words) {
		r.t.Fatal("the call's observation: the program ended before it answered")
	}
}

// observed is observe for a program that may end first: false then.
func (r *rig) observed(turn, id string, words []string) bool {
	r.t.Helper()
	tool, arguments := toolOf(words)
	answer, err := r.ask(command{Op: "observe", Turn: turn, Call: id, Tool: tool, Arguments: arguments})
	if errors.Is(err, errEnded) {
		return false
	}
	if err != nil || answer.Error != "" {
		r.t.Fatalf("the call's observation: %v %s", err, answer.Error)
	}
	return true
}

// request has the program ask the endpoint to run a call; ended says the
// program ended before it had the answer.
func (r *rig) request(turn, id string, words []string) (callResult, bool) {
	r.t.Helper()
	tool, arguments := toolOf(words)
	answer, ended := r.requestWith(command{Op: "request", Turn: turn, Call: id, Tool: tool, Arguments: arguments})
	if !ended && answer.Error != "" {
		r.t.Fatalf("the request of %v: %s", words, answer.Error)
	}
	return callResult{Texts: answer.Texts, IsError: answer.IsError}, ended
}

func (r *rig) requestWith(c command) (reply, bool) {
	r.calling.Store(true)
	defer r.calling.Store(false)
	answer, err := r.ask(c)
	if errors.Is(err, errEnded) {
		return reply{}, true
	}
	if err != nil {
		r.t.Fatal(err)
	}
	return answer, false
}

// complete has the program report the call's result as it handed it the
// model, and waits for the acknowledgment it starts, if the call holds a
// record. A program that ended reports nothing.
func (r *rig) complete(c toolCall, succeeded bool) error {
	r.t.Helper()
	texts := c.result.Texts
	if !succeeded {
		texts = nil
	}
	return r.report(c, command{Op: "result", Call: c.id, Texts: texts, IsError: !succeeded})
}

// report sends a result and waits for what it starts.
func (r *rig) report(c toolCall, result command) error {
	r.t.Helper()
	// Whether the endpoint will acknowledge is read first: while the result
	// is handled, the reads of this process are the observer's, and a plan
	// may fail them (wrapperPlan.during).
	bound := r.acknowledges(c)
	r.observing.Store(true)
	defer r.observing.Store(false)
	answer, err := r.ask(result)
	if errors.Is(err, errEnded) {
		// The program may have ended while its result was handled: an
		// acknowledgment it started still runs, and is waited for here so
		// that no later call takes its outcome for its own.
		if bound {
			select {
			case <-r.acknowledged:
			case <-time.After(2 * time.Second):
			}
		}
		return errors.New("the program ended: no result was reported")
	}
	if err != nil || answer.Error != "" {
		r.t.Fatalf("the result: %v %s", err, answer.Error)
	}
	if !bound {
		return nil
	}
	wait := 10 * time.Second
	if r.blind {
		// An observer that cannot read the binding acknowledges nothing.
		wait = 2 * time.Second
	}
	select {
	case err := <-r.acknowledged:
		return err
	case <-time.After(wait):
		if !r.blind {
			r.t.Fatal("the acknowledgment did not run")
		}
		return errors.New("no acknowledgment ran")
	}
}

// acknowledges says whether the next result of c starts an acknowledgment:
// its first, of a call that holds a record.
func (r *rig) acknowledges(c toolCall) bool {
	first := !r.completed[c.id]
	r.completed[c.id] = true
	_, err := receipt.Bound(r.dir, "api", r.self.Epoch(), bridge.CallKey(fixture.Transport, thread, c.id))
	return first && err == nil
}

// unread says whether api's letter id still waits.
func (r *rig) unread(id string) bool {
	r.t.Helper()
	messages, err := inbox.PeekUnread(r.dir, "api", r.self.Epoch())
	if err != nil {
		r.t.Fatal(err)
	}
	for _, message := range messages {
		if message.ID == id {
			return true
		}
	}
	return false
}
