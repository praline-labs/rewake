package inbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/state"
)

// Reconcile is the barrier every record of a mailbox passes before any turn
// end reads a wait and before adoption takes one over
// (docs/turn-end-recovery.md#reconciliation). The caller holds the mailbox
// lock. It plans first: it reads every record, then runs every effect left to
// make read-only (plan), so whatever an effect will decide by is read before
// the first one, and changes nothing while one of them is unknown. Every
// cause found is an occurrence of a stop, and only evidence resolves one
// (stop.go): the plan resolves those it found once it decides their
// operation, and the barrier those an effect met once it has run every
// effect through. Then the same code runs for real: it completes every
// unfinished journal, whichever run wrote it: an ended run's reports are out,
// and its waits may be taken over by the next run, which must not report on
// them again. An error stops whatever the caller was about to change.
func Reconcile(ctx context.Context, dir, name string) error {
	open, err := stopState(dir, name)
	if err != nil {
		return err
	}
	look := lookAtStop(dir, name, open)
	if look.holdsEffects() {
		return look.answer()
	}
	afterReading()
	w := live(dir)
	w.held = heldReports(look.open)
	if err := w.reconcileRecords(ctx, name, look.records); err != nil {
		return failedEffect(dir, name, err)
	}
	return ranThrough(dir, name, look)
}

// failedEffect records the stop of an effect that failed. An unknown an
// effect met is the effect's stop before anything else is looked at, recorded
// at once so that a crash cannot lose it. The plan after it may find a cause,
// which has not shown that cause to be the effect's, so it is an occurrence
// beside the effect's, never in its place; and an occurrence that could not
// be written is tried once more. The answer names both causes and every
// write that failed: when none went through, it is the one place the
// effect's cause is left. A plain failure the plan explains becomes the
// plan's stop; one it does not is answered as it is, and stops nothing.
func failedEffect(dir, name string, err error) error {
	w := live(dir)
	var unknown *UnknownRecordError
	met := errors.As(err, &unknown)
	var unwritten []stopCause
	var failed []error
	if met {
		open, _ := w.stopState(name)
		for _, cause := range causesOf(dir, err, causeEffect) {
			if cause.Kind != causeEffect || slices.ContainsFunc(open, func(stop openStop) bool { return stop.key == cause.key() }) {
				continue
			}
			if _, early := w.recordOccurrence(name, cause); early != nil {
				failed = append(failed, early)
				unwritten = append(unwritten, cause)
			}
		}
	}
	afterEffect()
	open, stateErr := w.stopState(name)
	if stateErr != nil {
		return errors.Join(err, stateErr)
	}
	after := lookAtStop(dir, name, open)
	if after.found == nil && !met {
		return err
	}
	for _, cause := range unwritten {
		if _, final := w.recordOccurrence(name, cause); final != nil {
			failed = append(failed, final)
		}
	}
	answer := err
	switch {
	case after.found != nil && met:
		answer = errors.Join(after.found, fmt.Errorf("before it, an effect met: %w", err))
	case after.found != nil:
		answer = after.found
	}
	return errors.Join(answer, errors.Join(after.failed...), errors.Join(failed...))
}

// ranThrough resolves the occurrences an effect met, once the barrier has run
// every effect through: the journal each is about is completed, which is
// evidence of what its operation did. One whose journal is neither completed
// now nor on record as completed stays open.
func ranThrough(dir, name string, look stopLook) error {
	w := live(dir)
	var left []openStop
	var failed []error
	for _, stop := range look.open {
		journal, _, _ := strings.Cut(stop.record.Op, "/")
		evidence := w.completedOf(name, stop)
		if journal != "" && slices.Contains(look.records.journals, journal) {
			evidence = fmt.Sprintf("the barrier ran the turn journal %s through to its completion", journal)
		}
		if evidence == "" {
			left = append(left, stop)
			continue
		}
		if err := w.resolveStop(name, stop, []string{evidence}); err != nil {
			failed = append(failed, err)
			left = append(left, stop)
		}
	}
	return errors.Join(recordedStopError(dir, name, left), errors.Join(failed...))
}

// completedOf is the evidence that the journal an occurrence is about is
// completed, empty when it is not: an operation done is decided, whatever its
// effects have since removed.
func (w world) completedOf(name string, stop openStop) string {
	journal, _, _ := strings.Cut(stop.record.Op, "/")
	if journal == "" {
		return ""
	}
	done := filepath.Join(JournalPath(w.dir, name), journal+doneSuffix)
	if _, err := w.stat(done); err != nil {
		return ""
	}
	return fmt.Sprintf("the turn journal %s is completed: %s", journal, done)
}

// planned is what a plan read and found.
type planned struct {
	records mailboxRecords
	found   error
	decided decidedOps
}

// plan runs the barrier's own code read-only: the reading of every record,
// then every effect left to make, which read what they decide by and write
// nothing (access.go). It answers what the reading found, every cause of it,
// and the operations it ran through. A journal that fails does not hide the
// next: each cause is an occurrence of its own.
func plan(dir, name string, held map[string]openStop) planned {
	w := planning(dir)
	w.held, w.decided = held, decidedOps{}
	records, err := w.inspectMailbox(name)
	if err == nil {
		err = w.reconcileRecords(context.Background(), name, records)
	}
	return planned{records: records, found: err, decided: w.decided}
}

// afterReading runs between the barrier's plan and its first effect, and
// afterEffect between a failed effect and the plan after it; a test changes
// a record there, which only an effect then meets.
var afterReading, afterEffect = func() {}, func() {}

// reconcileRecords completes every unfinished journal, each named as the
// operation its errors come from. The effects stop at the first that fails;
// a plan goes on to the next and answers them all.
func (w world) reconcileRecords(ctx context.Context, name string, records mailboxRecords) error {
	var found []error
	for _, id := range records.journals {
		err := inOp(id, w.finishJournal(ctx, name, id))
		switch {
		case err == nil:
			w.decide(id)
		case !w.plan:
			return err
		default:
			found = append(found, err)
		}
	}
	return errors.Join(found...)
}

// decide notes, in a plan, that it ran op through.
func (w world) decide(op string) {
	if w.decided != nil {
		w.decided[op] = true
	}
}

// MailboxStopped answers the stop a mailbox is in, nil when it is not stopped. A
// call that would change the mailbox asks it first, and answers the stop
// instead. It looks as the barrier does — the plan, its causes recorded, the
// ones it decided resolved (stop.go) — but runs no effect, so an occurrence
// only an effect met stays open until the barrier runs the effect through.
func MailboxStopped(dir, name string) error {
	open, err := stopState(dir, name)
	if err != nil {
		return err
	}
	if look := lookAtStop(dir, name, open); look.stopped() {
		return look.answer()
	}
	return nil
}

// mailboxRecords is what the barrier read before any effect.
type mailboxRecords struct {
	// journals names the unfinished journals.
	journals []string
}

// inspectMailbox is step 1 of the plan: it reads every file of the mailbox
// by its kind (records.go), then what the barrier weighs — every unfinished
// journal and every wait record — and changes nothing. One that cannot be
// read stops the mailbox, naming it.
func (w world) inspectMailbox(name string) (mailboxRecords, error) {
	var records mailboxRecords
	dir := w.dir
	if err := w.readMailbox(name); err != nil {
		return records, err
	}
	entries, err := w.readDir(JournalPath(dir, name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return records, fmt.Errorf("the turn journals %s cannot be listed: %w", JournalPath(dir, name), err)
	}
	for _, entry := range entries {
		// A dot is a write that never finished (state.WriteAtomic), not a
		// journal.
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), doneSuffix) {
			continue
		}
		if _, err := w.readJournalFile(filepath.Join(JournalPath(dir, name), entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			return records, fmt.Errorf("the turn journal %s cannot be read, so what it would do is unknown: %w", filepath.Join(JournalPath(dir, name), entry.Name()), err)
		}
		records.journals = append(records.journals, entry.Name())
	}
	return records, w.readEveryWait(name)
}

// readEveryWait reads the wait records of every run of the mailbox: one
// that cannot be read may name a wait any end would answer, so it stops the
// mailbox.
func (w world) readEveryWait(name string) error {
	runs, err := w.readDir(state.AwaitingPath(w.dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("the waits of %s cannot be listed: %w", name, err)
	}
	for _, run := range runs {
		if !run.IsDir() || strings.HasPrefix(run.Name(), ".") {
			continue
		}
		if _, err := w.readWaiters(name, run.Name()); err != nil {
			return fmt.Errorf("the waits of %s's run %s cannot be read, so what is owed is unknown: %w", name, run.Name(), err)
		}
	}
	return nil
}
