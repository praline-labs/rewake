//go:build rewakefixture

package decisionrig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/grant/authority"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/mail/mailtest"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// report is a report whose outcome is unknown, addressed to a session's run:
// what a decision on it asks about.
type report struct {
	sender, id string
	subject    mail.Subject
}

// unknownReport leaves a report of a new sender to the session's run unknown, as
// a turn end whose mark of publication does not read leaves it: the sender's
// run, which never ran, owed the session an answer, its journal carries the
// report, the mark is damaged and the barrier stops on it with an occurrence
// open. Written under the room's lease, as any writer of the room writes.
func (w *world) unknownReport(to *session) report {
	w.t.Helper()
	w.reports++
	sender := fmt.Sprintf("sender%02d", w.reports)
	room := w.room()
	release := holdLease(w.t, room)
	defer release()
	epoch := sender + "-epoch"
	if err := mailtest.Owe(room, sender, epoch, to.name, to.epoch, "question-"+sender); err != nil {
		w.t.Fatal(err)
	}
	waiters, err := mail.ReadWaiters(room, sender, epoch)
	if err != nil || len(waiters) != 1 {
		w.t.Fatalf("the wait: %v %v", waiters, err)
	}
	letter := mail.Message{
		ID: mail.ReportID(sender, epoch, waiters[0]), From: sender, FromEpoch: epoch, To: to.name, ToEpoch: to.epoch,
		Kind: mail.Finished, Text: "the answer", InReplyTo: []string{"question-" + sender}, CreatedAt: time.Now(),
	}
	if err := mail.WriteJournal(room, sender, "end", mail.TurnJournal{Epoch: epoch, Reports: []mail.Message{letter}, Clear: waiters}); err != nil {
		w.t.Fatal(err)
	}
	mark := mailtest.OncePath(room, to.name, to.epoch, letter.ID)
	if err := os.MkdirAll(filepath.Dir(mark), 0o700); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(mark, []byte("damaged"), 0o600); err != nil {
		w.t.Fatal(err)
	}
	if err := state.WithMailboxLock(context.Background(), room, sender, func() error {
		return mail.Reconcile(context.Background(), room, sender)
	}); err == nil {
		w.t.Fatal("the barrier went past a mark that does not read")
	}
	subject, err := mail.FindSubject(room, sender, letter.ID)
	if err != nil {
		w.t.Fatalf("the report has no subject: %v", err)
	}
	return report{sender: sender, id: letter.ID, subject: subject}
}

// holdLease takes the room's lease shared for this process until release.
func holdLease(t *testing.T, room string) func() {
	t.Helper()
	lock, err := state.OpenLeaseLock(room)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Share(5 * time.Second); err != nil {
		lock.Close()
		t.Fatal(err)
	}
	if err := state.Authorize(lock); err != nil {
		lock.Close()
		t.Fatal(err)
	}
	return lock.Close
}

// path is where the decision on the report lies.
func (w *world) path(r report) string { return mail.DecisionPath(w.room(), r.sender, r.id) }

// decision is the decision on the report, if one is there.
func (w *world) decision(r report) (mail.Decision, bool) {
	w.t.Helper()
	raw, err := os.ReadFile(w.path(r))
	if errors.Is(err, os.ErrNotExist) {
		return mail.Decision{}, false
	}
	if err != nil {
		w.t.Fatal(err)
	}
	var d mail.Decision
	if err := json.Unmarshal(raw, &d); err != nil {
		w.t.Fatalf("the decision does not read: %v", err)
	}
	return d, true
}

// decide is the job of asking the session's wrapper about the report through
// the product's client, as the report's subject leads to it.
func (w *world) decide(r report) job {
	run, _ := authority.RunOf(r.subject.RecipientRun)
	return job{
		Mode: modeDecide, Address: state.AuthorityAddress(w.room(), r.subject.RecipientRun), Run: run,
		Ask:  mail.DecideRequest{Sender: r.sender, Report: r.id, Choice: mail.ChoiceArrived, Expect: r.subject},
		Path: w.path(r),
	}
}

// verified says whether a decision was made by the session's run, verified.
func verified(by mail.DecidedBy, s *session) bool {
	return by.Kind == mail.ByRecipient && by.Name == s.name && by.Run == s.epoch && by.Verified
}
