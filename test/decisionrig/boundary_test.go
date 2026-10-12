//go:build rewakefixture

package decisionrig

import (
	"context"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A verified decision is written by the recipient run's wrapper while it
// lives, and never once it has ended (docs/v2/stage3-decision.md#authority):
// the wrapper is killed with a question at each point of its way — held by
// the client after checking the wrapper, admitted by the wrapper, waiting in
// the transaction for the sender's mailbox, committed with its answer not yet
// sent — and the asker is told the outcome is unknown, with the record named
// where one may be, while a record is there only if the wrapper wrote it
// before it died.
func TestDecisionAuthorityEndsAtCommitBoundary(t *testing.T) {
	t.Run("held by the client after checking the wrapper", func(t *testing.T) {
		w := newWorld(t)
		web := w.launch("web", "--write", "")
		r := w.unknownReport(web)
		held := newPause(t)
		j := w.decide(r)
		j.Pause = string(held)
		out := web.command(terminalHalf, command{Op: opAsk, Job: j})
		held.waitHeld(t, "the client")
		web.kill()
		held.release(t)
		w.expectUnknown(t, w.await(out), r, false)
	})
	t.Run("admitted by the wrapper", func(t *testing.T) {
		w := newWorld(t)
		held := newPause(t)
		web := w.launch("web", "--write", "other:holdat=decision admitted@"+string(held))
		r := w.unknownReport(web)
		out := web.command(terminalHalf, command{Op: opAsk, Job: w.decide(r)})
		held.waitHeld(t, "the wrapper")
		web.kill()
		w.expectUnknown(t, w.await(out), r, false)
	})
	t.Run("waiting in the transaction for the sender's mailbox", func(t *testing.T) {
		w := newWorld(t)
		held := newPause(t)
		web := w.launch("web", "--write", "other:holdat=decision admitted@"+string(held))
		r := w.unknownReport(web)
		out := web.command(terminalHalf, command{Op: opAsk, Job: w.decide(r)})
		held.waitHeld(t, "the wrapper")
		unlock := lockMailbox(t, w.room(), r.sender)
		held.release(t)
		// The transaction waits for the lock for seconds; it is well into
		// that wait when the wrapper dies.
		time.Sleep(time.Second)
		web.kill()
		unlock()
		w.expectUnknown(t, w.await(out), r, false)
	})
	t.Run("committed with its answer not yet sent", func(t *testing.T) {
		w := newWorld(t)
		held := newPause(t)
		web := w.launch("web", "--write", "other:holdat=decision answered@"+string(held))
		r := w.unknownReport(web)
		out := web.command(terminalHalf, command{Op: opAsk, Job: w.decide(r)})
		held.waitHeld(t, "the wrapper")
		web.kill()
		w.expectUnknown(t, w.await(out), r, true)
		if d, _ := w.decision(r); !verified(d.By, web) {
			t.Fatalf("the decision written before the wrapper died is %+v", d.By)
		}
	})
}

// expectUnknown fails unless the asker was told the outcome is unknown and
// named the record, and the record is there as the wrapper left it — and
// stays so after the wrapper is gone.
func (w *world) expectUnknown(t *testing.T, got result, r report, written bool) {
	t.Helper()
	_, there := w.decision(r)
	time.Sleep(300 * time.Millisecond)
	_, later := w.decision(r)
	if got.Kind != string(mail.DecisionUncertain) || got.Path != w.path(r) || there != written || later != there {
		t.Fatalf("answered %+v; a decision there %v, later %v, want %v", got, there, later, written)
	}
}

// lockMailbox holds a session's mailbox lock from this process until unlock.
func lockMailbox(t *testing.T, room, name string) func() {
	t.Helper()
	release := holdLease(t, room)
	locked, unlock, done := make(chan error, 1), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		err := state.WithMailboxLock(context.Background(), room, name, func() error {
			locked <- nil
			<-unlock
			return nil
		})
		if err != nil {
			locked <- err
		}
	}()
	if err := <-locked; err != nil {
		release()
		t.Fatal(err)
	}
	return func() { close(unlock); <-done; release() }
}

// The harness ends while the wrapper holds a decision it admitted: the run
// takes no new one from that moment (docs/v2/stage3-state.md#one-build-per-room),
// and it keeps its hold on the room's lease until the one it admitted is
// done, so the decision either lands while the run still holds the lease or
// not at all.
func TestTheRunEndsAfterTheDecisionItAdmitted(t *testing.T) {
	w := newWorld(t)
	held := newPause(t)
	web := w.launch("web", "--write", "other:holdat=decision admitted@"+string(held))
	first, second := w.unknownReport(web), w.unknownReport(web)
	out := web.command(terminalHalf, command{Op: opAsk, Job: w.decide(first)})
	held.waitHeld(t, "the wrapper")
	web.command(terminalHalf, command{Op: opExit})
	if !waitFor(10*time.Second, func() bool { return syscall.Kill(web.harness, 0) != nil }) {
		t.Fatal("the harness did not end")
	}
	time.Sleep(300 * time.Millisecond)
	// Refused at the door, it is answered while the admitted one is still
	// held; taken in, it would wait behind that one's step.
	late, answered := w.awaitWithin(web.command(serverHalf, command{Op: opAsk, Job: w.decide(second)}), 15*time.Second)
	if !answered || late.Kind != string(mail.DecisionBusy) {
		t.Errorf("a question after the harness ended: answered %v, %+v", answered, late)
	}
	if exclusive(t, w.room()) {
		t.Error("the run let go of the room's lease with a decision admitted")
	}
	held.release(t)
	got := w.await(out)
	web.waitExit()
	d, written := w.decision(first)
	if written != (got.Kind == kindDecided) && got.Kind != string(mail.DecisionUncertain) || written && !verified(d.By, web) {
		t.Errorf("the admitted decision answered %+v; written %v: %+v", got, written, d.By)
	}
	if _, late := w.decision(second); late {
		t.Error("a decision asked after the harness ended was written")
	}
	if !waitFor(10*time.Second, func() bool { return exclusive(t, w.room()) }) {
		t.Error("the room's lease stayed held after the run ended")
	}
	t.Logf("the admitted decision answered %s, written %v", got.Kind, written)
}

// exclusive says whether this process could take the room's lease alone:
// no run holds it.
func exclusive(t *testing.T, room string) bool {
	t.Helper()
	lock, err := state.OpenLeaseLock(room)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	alone, err := lock.TryExclusive()
	if err != nil {
		t.Fatal(err)
	}
	return alone
}
