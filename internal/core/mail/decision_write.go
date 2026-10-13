package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/boottime"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Decide is the decision's final transaction
// (docs/v2/stage3-decision-recovery.md#the-decisions-record): the one writer
// of a decision. It is run by the process that holds the authority — the
// recipient run's own wrapper, through the handler its host supplies, or the
// person's own process — never by a shell on a wrapper's word, so no verified
// decision is written after the run that vouched for it ended (Q2 of
// docs/v2/stage3-decision-build.md).
//
// The caller holds the room's build lease, outermost (Q1). Under it: the
// sender's mailbox lock, with a bounded wait; a decision already there first —
// the same choice runs its installation and the barrier again, the opposite is
// refused, one that does not read is no free name; otherwise the subject, read
// on its own through a capture; then, unless the recipient's mailbox is the
// sender's own, the recipient's lock with S3's wait, inside which the report's
// final evidence is read through the same capture and the decision is written
// only while the report is still unknown. The recipient's lock goes before the
// installation and the barrier, which run under the sender's.

// DecideRequest asks for one decision.
type DecideRequest struct {
	Sender string
	Report string
	Choice string
	// By is who decides: the recipient's run, verified, which must be the
	// run of this very process; or the person, whom the caller classified.
	By DecidedBy
	// Expect is the subject the caller found before it asked (FindSubject):
	// one read otherwise under the lock is refused.
	Expect Subject
	// DeadlineBoot is when the call that asked stops wanting the answer, on
	// the boot clock; zero for a caller with no such bound. Past it no record
	// is written (docs/mail-bridge-cli.md, rule 3).
	DeadlineBoot int64
}

// DecideAnswer is a decision made, or found made with the same choice.
type DecideAnswer struct {
	Decision Decision
	Path     string
	// Before says the decision was there already.
	Before bool
	// Barrier is what the barrier after the installation answered: the
	// mailbox stays stopped on other causes, or a publication waits. It
	// fails nothing the decision fixed.
	Barrier error
	// Owed are the notes not told yet, by the run they tell; a later barrier
	// tells them.
	Owed []string
}

// senderLockWait bounds the wait for the sender's mailbox: a decision on a
// busy mailbox answers busy and writes nothing.
var senderLockWait = 5 * time.Second

// selfRun is the run this process would be as a wrapper: a verified
// decision is written only by the wrapper of the run it names.
var selfRun = sync.OnceValues(func() (string, error) {
	pid := os.Getpid()
	start, err := proc.StartTime(pid)
	if err != nil {
		return "", err
	}
	boot, err := registry.CurrentBoot()
	if err != nil {
		return "", err
	}
	return registry.RunEpoch(pid, start, boot), nil
})

// Decide makes the decision req asks for, or answers why not.
func Decide(ctx context.Context, dir string, req DecideRequest) (DecideAnswer, error) {
	if req.Choice != ChoiceArrived && req.Choice != ChoiceNotArrived {
		return DecideAnswer{}, refuseDecision(DecisionNoSubject, "", "a decision says %s or %s, not %q", ChoiceArrived, ChoiceNotArrived, req.Choice)
	}
	if !state.ValidName(req.Sender) || !boundedID(req.Report) {
		return DecideAnswer{}, refuseDecision(DecisionNoSubject, "", "a decision names a session and a report")
	}
	if err := checkDecider(req.By); err != nil {
		return DecideAnswer{}, err
	}
	if err := req.inTime(); err != nil {
		return DecideAnswer{}, err
	}
	var answer DecideAnswer
	wait, cancel := context.WithTimeout(ctx, senderLockWait)
	defer cancel()
	err := live(dir).lock(wait, req.Sender, func() error {
		var err error
		answer, err = decideLocked(ctx, dir, req)
		return err
	})
	var refusal *DecisionError
	if err != nil && !errors.As(err, &refusal) {
		if errors.Is(err, state.ErrMailboxBusy) || isLockUnusable(err) {
			return DecideAnswer{}, &DecisionError{Outcome: DecisionBusy, Path: state.InboxPath(dir, req.Sender), Err: fmt.Errorf("the mailbox of %s is busy, so nothing was decided: %w", req.Sender, err)}
		}
	}
	return answer, err
}

func isLockUnusable(err error) bool {
	var unusable *state.LockUnusableError
	return errors.As(err, &unusable)
}

// checkDecider holds who decides to its shape, and a verified decider to
// this process: only the recipient run's own wrapper writes as it.
func checkDecider(by DecidedBy) error {
	switch by.Kind {
	case ByPerson:
		if by.Verified || by.Name != "" || by.Run != "" {
			return refuseDecision(DecisionUnauthorized, "", "the person decides unverified and names no run")
		}
		return nil
	case ByRecipient:
		self, err := selfRun()
		if err != nil {
			return refuseDecision(DecisionUnreadable, "", "this process cannot name its own run: %v", err)
		}
		if !by.Verified || by.Run != self || !state.ValidName(by.Name) {
			return refuseDecision(DecisionUnauthorized, "", "a verified decision is written only by the wrapper of the run that decides, and this process is run %s", self)
		}
		return nil
	}
	return refuseDecision(DecisionUnauthorized, "", "a decision is made by the recipient's run or by the person, not %q", by.Kind)
}

// authorize says whether by may decide subject: the person may decide any;
// the recipient only as the subject's recipient run.
func authorize(by DecidedBy, subject Subject) error {
	if by.Kind == ByRecipient && (by.Name != subject.Recipient || by.Run != subject.RecipientRun) {
		return refuseDecision(DecisionUnauthorized, "", "report %s of %s is decided by %s (run %s) or by the person, not by run %s of %s",
			subject.Report, subject.Sender, subject.Recipient, subject.RecipientRun, by.Run, by.Name)
	}
	return nil
}

// decideLocked runs under the sender's lock.
func decideLocked(ctx context.Context, dir string, req DecideRequest) (DecideAnswer, error) {
	w := live(dir)
	path := DecisionPath(dir, req.Sender, req.Report)
	existing, err := readDecision(w.files, path)
	switch {
	case err == nil:
		return decidedBefore(ctx, dir, req, path, existing)
	case !errors.Is(err, os.ErrNotExist):
		return DecideAnswer{}, &DecisionError{Outcome: DecisionUnreadable, Path: path, Err: fmt.Errorf("a decision on report %s is there and does not read, so it is no free name: %w", req.Report, err)}
	}
	files := newCapture(accessFor(dir, false), dir)
	seen := world{dir: dir, files: files, plan: true}
	plan, err := seen.subjectOf(req.Sender, req.Report)
	if err != nil {
		return DecideAnswer{}, err
	}
	if plan.subject != req.Expect {
		return DecideAnswer{}, refuseDecision(DecisionChanged, "", "the subject of report %s is not the one found before: %+v, where %+v was found", req.Report, plan.subject, req.Expect)
	}
	if err := authorize(req.By, plan.subject); err != nil {
		return DecideAnswer{}, err
	}
	d := Decision{
		ID: NewID(), Choice: req.Choice, By: req.By, At: time.Now(), Delivery: deliveryUnproven,
		Subject: plan.subject, Parent: plan.subject.parentOf(),
	}
	for _, stop := range plan.about {
		d.Disposes = append(d.Disposes, Disposal{Key: stop.key, Occurrence: stop.id})
	}
	if d.Notes, err = decisionNotes(dir, d); err != nil {
		return DecideAnswer{}, err
	}
	if d.Choice == ChoiceNotArrived {
		if d.Resend, err = resendFor(d, plan.report); err != nil {
			return DecideAnswer{}, err
		}
	}
	err = w.recipientSection(ctx, req.Sender, plan.subject.Recipient, func() error {
		return w.commitDecision(seen, files, plan, path, &d, req.inTime)
	})
	if err != nil {
		var refusal *DecisionError
		if errors.As(err, &refusal) {
			return DecideAnswer{}, err
		}
		return DecideAnswer{}, &DecisionError{Outcome: DecisionBusy, Path: state.InboxPath(dir, plan.subject.Recipient), Err: err}
	}
	return installDecided(ctx, dir, path, d, false)
}

// commitDecision runs in the recipient's section: the report's final
// evidence, then the record, only while the report is still unknown and the
// asker still wants the answer — checked last, after both lock waits.
func (w world) commitDecision(seen world, files *capture, plan subjectPlan, path string, d *Decision, inTime func() error) error {
	if proof := seen.finalEvidence(plan); proof != "" {
		return refuseDecision(DecisionProven, "", "report %s of %s is %s since its stop, so the barrier settles it by evidence and nothing is decided", plan.subject.Report, plan.subject.Sender, proof)
	}
	observations, err := files.Observations()
	if err != nil {
		return &DecisionError{Outcome: DecisionUnreadable, Err: err}
	}
	d.Observations = observations
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	// The record is read back as every reader will read it before it is
	// written: one its own reader refuses gives no authority anyway.
	if _, err := parseDecision(path, raw); err != nil {
		return fmt.Errorf("the decision would not read: %w", err)
	}
	if err := inTime(); err != nil {
		return err
	}
	if err := w.ensureDir(filepath.Dir(path)); err != nil {
		return &DecisionError{Outcome: DecisionUnreadable, Path: path, Err: fmt.Errorf("the decisions of %s cannot be written: %w", plan.subject.Sender, err)}
	}
	if err := w.publish(path, raw); err != nil {
		return &DecisionError{Outcome: DecisionUncertain, Path: path, Err: fmt.Errorf("the decision on report %s may or may not be written, and is not removed or replaced: %w", plan.subject.Report, err)}
	}
	return nil
}

// inTime refuses a decision whose asker no longer wants the answer: a record
// written then is one the asker never learns of, and nothing it fixes may
// follow from a call that is over. Nothing is written, and the same words
// again, under a bound of their own, decide.
func (req DecideRequest) inTime() error {
	if req.DeadlineBoot == 0 || boottime.Now() < req.DeadlineBoot {
		return nil
	}
	return refuseDecision(DecisionExpired, "", "the call that asked for the decision on report %s stopped waiting for its answer before the record was written, so nothing was decided", req.Report)
}

// finalEvidence is the proof of the report's outcome the barrier would
// settle it by, read in the recipient's section through the capture; "" while
// it stays unknown. It reads as the barrier does, so the two never disagree: a
// decision is refused on what the barrier settles, and made on what it cannot.
// Every read is made, whatever the earlier ones gave, so the decision keeps
// them all.
//
// The occurrences about the report are read as the barrier weighs them
// (lookAtStop, holdsEffects): one a plan found resolves only on evidence that
// reads, none of its paths removed; one only an effect met is run through by
// the barrier unless a path it names was removed, and what the effect will
// meet is the report's own evidence, read next. The report is then read as
// the barrier delivers a held one (publishWith): with its run live, as
// publishMarked reads it — a mark that does not read, or a letter's place that
// cannot be searched, stops it, since the barrier writes nothing over either;
// with its run ended, as endedEvidence settles it (endedSettlement), where
// the letter found is proof whatever the mark says and no mark at all is none.
func (w world) finalEvidence(plan subjectPlan) string {
	report := plan.report
	unknown := false
	for _, stop := range plan.about {
		_, removed, err := w.evidenceOf(stop)
		if len(removed) > 0 || err != nil && stop.record.Kind != causeEffect {
			unknown = true
		}
	}
	mark, markErr := "", error(nil)
	path, marked := oncePath(w.dir, report.To, report.ToEpoch, report.ID)
	if marked {
		mark, markErr = w.readOnceMark(path)
	}
	found, presentErr := w.present(report.To, report.ID)
	peer, err := registry.LookupReadOnly(w.dir, report.To)
	if err != nil && !errors.Is(err, registry.ErrNotFound) || !marked || unknown {
		return ""
	}
	if err != nil || peer.Epoch() != report.ToEpoch {
		published, err := endedSettlement(found, presentErr, func() (string, error) { return mark, markErr })
		switch {
		case err != nil:
			return ""
		case published:
			return "proven published"
		}
		return "proven not published"
	}
	if presentErr != nil && !found || markErr != nil {
		return ""
	}
	if found || mark == oncePublished {
		return "proven published"
	}
	return "proven not published"
}

// resendFor is the resend journal a decision "did not arrive" holds: the
// report sent again under a fresh id naming it and the decision, as an
// operation of its own with none of the original's tail (Q8).
func resendFor(d Decision, report Message) (json.RawMessage, error) {
	copied := report
	copied.ID, copied.Resends, copied.Decision = NewID(), report.ID, d.ID
	return json.Marshal(TurnJournal{Epoch: d.Subject.SenderRun, Op: ResendJournal(d.ID), Reports: []Message{copied}})
}
