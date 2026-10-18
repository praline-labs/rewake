//go:build rewaketest

package decisionrig

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/boottime"
)

// A tool call's bound reaches the wrapper with the question and is checked
// there after both lock waits, just before the record: the wrapper's own
// limit is far longer, so a call whose deadline passes while the transaction
// waits for the sender's mailbox, or the recipient's, is refused expired with
// nothing written, and the same question under a bound of its own decides.
func TestTheWrapperWritesNoDecisionPastTheCallsDeadline(t *testing.T) {
	w := newWorld(t)
	web := w.launch("web", "--write", "")
	for _, held := range []struct {
		name    string
		mailbox func(report) string
	}{
		{"the sender's mailbox", func(r report) string { return r.sender }},
		{"the recipient's mailbox", func(report) string { return web.name }},
	} {
		r := w.unknownReport(web)
		unlock := lockMailbox(t, w.room(), held.mailbox(r))
		j := w.decide(r)
		j.Ask.DeadlineBoot = boottime.Now() + int64(300*time.Millisecond)
		out := web.command(terminalHalf, command{Op: opAsk, Job: j})
		time.Sleep(time.Second)
		unlock()
		got := w.await(out)
		if _, written := w.decision(r); got.Kind != string(mail.DecisionExpired) || written {
			t.Fatalf("%s held past the deadline: answered %+v; a decision written %v", held.name, got, written)
		}
		again := w.decide(r)
		again.Ask.DeadlineBoot = boottime.Now() + int64(30*time.Second)
		got = web.do(terminalHalf, command{Op: opAsk, Job: again})
		if d, written := w.decision(r); got.Kind != kindDecided || !written || d.ID != got.ID || !verified(d.By, web) {
			t.Fatalf("%s: asked again under a bound of its own: %+v; a decision written %v: %+v", held.name, got, written, d)
		}
	}
}
