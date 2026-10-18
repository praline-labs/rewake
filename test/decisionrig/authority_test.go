//go:build rewaketest

package decisionrig

import (
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	snapshot "github.com/praline-labs/rewake/internal/core/session"
)

// The decision on a report is the recipient run's, verified, and nobody
// else's (docs/v2/stage3-decision.md#authority): a command below that run's
// wrapper is answered whatever its environment says, and every other asker is
// refused with nothing written — another worker's command, with the
// recipient's variables or without, through the client or written straight to
// the address; the recipient's own command in a namespace of its own; a
// listener other than the run's wrapper; and, once the run is replaced, its
// successor under the same name. A compaction and a dropped link leave the
// run what it was, and so its authority.
//
// Each question is about a report of its own, so a refusal is read as nothing
// written rather than as a decision made before it.
func TestDecisionAuthorityWithProcesses(t *testing.T) {
	w := newWorld(t)
	web := w.launch("web", "--write", "")
	peer := w.launch("peer", "--write", "")
	spoofed := []string{"REWAKE_SESSION=" + web.name, "REWAKE_EPOCH=" + web.epoch}
	refused := func(kind string) func(result) bool {
		return func(got result) bool { return got.Kind == kind }
	}
	decided := func(got result) bool { return got.Kind == kindDecided && verified(got.By, web) }
	for _, c := range []struct {
		name   string
		asker  *session
		half   string
		change func(*job)
		want   func(result) bool
	}{
		{"the recipient's child", web, terminalHalf, nil, decided},
		{"the recipient's child with the session's variables unset", web, terminalHalf, func(j *job) {
			j.Unset = []string{"REWAKE_SESSION", "REWAKE_EPOCH"}
		}, decided},
		{"the recipient's grandchild that left its group through setsid", web, terminalHalf, func(j *job) {
			j.Detach = detachSetsid
		}, decided},
		{"another worker's child", peer, terminalHalf, nil, refused(string(mail.DecisionUnauthorized))},
		{"another worker's child with the recipient's variables", peer, terminalHalf, func(j *job) {
			j.Set = spoofed
		}, refused(string(mail.DecisionUnauthorized))},
		{"another worker's child, straight to the address", peer, terminalHalf, func(j *job) {
			j.Mode = modeRaw
		}, refused(string(mail.DecisionUnauthorized))},
		{"another worker's child with the recipient's variables, straight to the address", peer, terminalHalf, func(j *job) {
			j.Mode, j.Set = modeRaw, spoofed
		}, refused(string(mail.DecisionUnauthorized))},
		{"the recipient's child in a namespace of its own, straight to the address", web, terminalHalf, func(j *job) {
			j.Mode, j.Namespace = modeRaw, true
		}, refused(string(mail.DecisionUnauthorized))},
		{"the recipient's child in a namespace of its own, through the client", web, terminalHalf, func(j *job) {
			j.Namespace = true
		}, refused(kindUnconfirmed)},
		{"a listener whose start is not the run's", web, terminalHalf, func(j *job) {
			j.Run.Start++
		}, refused(kindUnconfirmed)},
		{"a run of another boot", web, terminalHalf, func(j *job) {
			j.Run.Boot = "another-boot"
		}, refused(string(mail.DecisionUnauthorized))},
	} {
		r := w.unknownReport(web)
		j := w.decide(r)
		if c.change != nil {
			c.change(&j)
		}
		got := c.asker.do(c.half, command{Op: opAsk, Job: j})
		d, written := w.decision(r)
		wantWritten := got.Kind == kindDecided
		if !c.want(got) || written != wantWritten || written && (d.ID != got.ID || !verified(d.By, web)) {
			t.Errorf("%s: answered %+v; a decision written %v: %+v", c.name, got, written, d)
		}
	}

	// A compaction the program reports is counted, and the run decides after it.
	if got := web.do(serverHalf, command{Op: opCompact}); got.Kind != "done" {
		t.Fatalf("the compaction was not reported: %+v", got)
	}
	if !waitFor(20*time.Second, func() bool {
		count := snapshot.Load(w.room(), web.name, web.epoch).Compactions
		return count != nil && *count >= 1
	}) {
		t.Errorf("the compaction was never counted")
	}
	w.expectDecided(t, "after a compaction", web, terminalHalf)

	// The program's link drops and comes back; the run decides after it,
	// asked from the half whose link it was.
	if got := web.do(serverHalf, command{Op: opDrop}); got.Kind != "done" {
		t.Fatalf("the link did not come back: %+v", got)
	}
	var links int
	if !waitFor(20*time.Second, func() bool { return readJSON(web.ctl+"/"+serverHalf+"/links", &links) == nil && links >= 2 }) {
		t.Errorf("the probes of the second link were never answered: %d", links)
	}
	w.expectDecided(t, "after the link came back", web, serverHalf)

	// The run is replaced under the same name: its report is no longer any
	// wrapper's to decide, neither at the ended run's address nor at the
	// successor's.
	old := w.unknownReport(web)
	web.stop()
	successor := w.launch("web", "--write", "")
	if successor.name != web.name || successor.epoch == web.epoch {
		t.Fatalf("the successor runs as %s %s, the run it replaced as %s %s", successor.name, successor.epoch, web.name, web.epoch)
	}
	atOld := successor.do(terminalHalf, command{Op: opAsk, Job: w.decide(old)})
	atNew := w.decide(old)
	atNew.Mode, atNew.Address = modeRaw, successor.address()
	straight := successor.do(terminalHalf, command{Op: opAsk, Job: atNew})
	_, written := w.decision(old)
	if atOld.Kind != string(mail.DecisionUnauthorized) || straight.Kind != string(mail.DecisionUnauthorized) || written {
		t.Errorf("a report to the replaced run: at its address %+v, at the successor's %+v; a decision written %v", atOld, straight, written)
	}
}

// expectDecided asks about a new report from a half of the session and
// fails unless the session's run decided it, verified.
func (w *world) expectDecided(t *testing.T, what string, s *session, half string) {
	t.Helper()
	r := w.unknownReport(s)
	got := s.do(half, command{Op: opAsk, Job: w.decide(r)})
	d, written := w.decision(r)
	if got.Kind != kindDecided || !verified(got.By, s) || !written || !verified(d.By, s) {
		t.Errorf("%s: answered %+v; a decision written %v: %+v", what, got, written, d)
	}
}
