//go:build rewakefixture

package toolrig

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/cli"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
)

// Two orders of an acknowledgment against its turn's end that the generated
// orders cannot isolate: an end that overtakes an acknowledgment whose result
// was already accepted, and an end on record that this endpoint's gate never
// heard of.

// An acknowledgment held after its result was accepted but before it entered
// the gate loses to an end captured meanwhile (T7, T8): the letter stays
// unread, the end reports nothing of it, and a later turn's read claims it
// once.
func TestAnEndCapturedBeforeAnAcknowledgmentsCheckLeavesTheLetterUnread(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	const text = "the letter an end overtakes"
	id := r.letter(text)
	started := boottime.Now()
	r.nextTurn()
	c := r.call("inbox")
	if c.ended || !strings.Contains(c.result.text(), text) {
		t.Fatalf("the read: %+v", c)
	}
	held := r.holdStep("acknowledge")
	acknowledged := make(chan error, 1)
	go func() { acknowledged <- r.complete(c, true) }()
	select {
	case <-held.reachedC:
	case <-time.After(10 * time.Second):
		t.Fatal("the acknowledgment was never held before its check")
	}
	boundary := r.endTurn()
	if boundary == nil {
		t.Fatal("the end captured no boundary")
	}
	held.free()
	if err := <-acknowledged; !errors.Is(err, cli.ErrTurnEnded) {
		t.Fatalf("the acknowledgment after the end: %v", err)
	}
	if !r.unread(id) {
		t.Fatal("an acknowledgment the end overtook read the letter")
	}
	if err := r.journal(boundary, started); err != nil {
		t.Fatal(err)
	}
	if got := report(r); got != "" {
		t.Fatalf("the end reported %q of a letter it did not read", got)
	}

	started = boottime.Now()
	r.nextTurn()
	again := r.call("inbox")
	if !strings.Contains(again.result.text(), text) {
		t.Fatalf("the next turn's read: %+v", again.result)
	}
	if err := r.complete(again, true); err != nil || r.unread(id) {
		t.Fatalf("the next turn's acknowledgment: %v, unread %v", err, r.unread(id))
	}
	if err := r.journal(r.endTurn(), started); err != nil {
		t.Fatal(err)
	}
	if got := report(r); got != inbox.Finished {
		t.Fatalf("the next turn's end reported %q", got)
	}
	r.nextTurn()
	if text := r.call("inbox").result.text(); strings.Contains(text, "the letter an end overtakes") {
		t.Fatalf("a letter read once shows again: %s", text)
	}
}

// An end recorded by another publisher, never noted by this endpoint's gate,
// refuses the acknowledgment by the record alone (T7): the gate knows only the
// ends captured through it.
func TestAnEndOnRecordTheGateNeverHeardRefusesTheAcknowledgment(t *testing.T) {
	t.Parallel()
	r := newRig(t)
	r.start()
	id := r.letter("a letter an end on record keeps unread")
	started := boottime.Now()
	r.nextTurn()
	c := r.call("inbox")
	if c.ended || c.result.IsError {
		t.Fatalf("the read: %+v", c)
	}
	if err := cli.ReportCompletion(context.Background(), r.dir, r.self, harness.Completion{
		ID: "an-end-elsewhere", Thread: thread, Kind: inbox.Finished,
		Text: "finished elsewhere", Started: started, Ended: boottime.Now(), Boundary: r.clock.Snapshot(),
	}); err != nil {
		t.Fatal(err)
	}
	if r.endpoint.Gate().Noted() != 0 {
		t.Fatal("the end on record reached the endpoint's gate")
	}
	if err := r.complete(c, true); !errors.Is(err, cli.ErrTurnEnded) {
		t.Fatalf("the acknowledgment after an end on record: %v", err)
	}
	if !r.unread(id) {
		t.Fatal("an acknowledgment after an end on record read the letter")
	}
}
