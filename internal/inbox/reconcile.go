package inbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/state"
)

// Reconcile is the barrier every record of a mailbox passes before any turn
// end reads a wait and before adoption takes one over
// (docs/turn-end-recovery.md#reconciliation). The caller holds the mailbox
// lock. It plans first: it reads every record, then runs every effect left to
// make read-only (plan), so whatever an effect will decide by is read before
// the first one, and changes nothing while one of them is unknown. The stop
// found is recorded (stop.go); a plan that finds nothing lifts one the reading
// found, and one an effect met is lifted only once every effect has run
// through. Then the same code runs for real: it completes every unfinished
// journal, whichever run wrote it: an ended run's reports are out, and its
// waits may be taken over by the next run, which must not report on them
// again. An error stops whatever the caller was about to change: a record
// that could not be read, which a later look may settle.
func Reconcile(ctx context.Context, dir, name string) error {
	recorded, err := recordedStop(dir, name)
	if err != nil {
		return err
	}
	// A stop an effect met stands until that effect has run through; a plan
	// that finds nothing proves only that the reading's causes left.
	effect := recorded != nil && recorded.Effect
	records, err := plan(dir, name)
	if err != nil && !isStopped(err) {
		return keepStop(dir, name, err)
	}
	if recorded != nil && !effect {
		if err := liftStop(dir, name); err != nil {
			return err
		}
	}
	afterReading()
	if err := live(dir).reconcileRecords(ctx, name, records); err != nil {
		// An unknown an effect met is the effect's stop before anything else
		// is looked at, recorded at once so that a crash cannot lose it. The
		// plan after it may stop on some cause, which has not shown that cause
		// to be the effect's, so it goes beside the effect's, never in its
		// place; and the stop is recorded again with all this call knows, since
		// the first record may not have been written. The answer names both
		// causes, as the record does: when neither write went through, it is
		// the one place the effect's cause is left. A plain failure the plan
		// explains becomes the plan's stop.
		var met string
		var early error
		var unknown *UnknownRecordError
		if errors.As(err, &unknown) {
			met = err.Error()
			early = recordStop(dir, name, err, met)
		}
		afterEffect()
		cause := err
		if _, again := plan(dir, name); again != nil && !isStopped(again) {
			cause = again
		} else if met == "" {
			return err
		}
		answer := cause
		if met != "" && cause != err {
			answer = errors.Join(cause, fmt.Errorf("before it, an effect met: %w", err))
		}
		if final := recordStop(dir, name, cause, met); final != nil {
			return errors.Join(answer, early, final)
		}
		return answer
	}
	if effect {
		return liftStop(dir, name)
	}
	return nil
}

// plan runs the barrier's own code read-only: the reading of every record,
// then every effect left to make, which read what they decide by and write
// nothing (access.go). It answers what the reading found.
func plan(dir, name string) (mailboxRecords, error) {
	w := planning(dir)
	records, err := w.inspectMailbox(name)
	if err != nil {
		return records, err
	}
	return records, w.reconcileRecords(context.Background(), name, records)
}

// isStopped says err is a stop on record, which keepStop answers as it is.
func isStopped(err error) bool {
	var stopped *RecordedStopError
	return errors.As(err, &stopped)
}

// afterReading runs between the barrier's plan and its first effect, and
// afterEffect between a failed effect and the plan after it; a test changes
// a record there, which only an effect then meets.
var afterReading, afterEffect = func() {}, func() {}

func (w world) reconcileRecords(ctx context.Context, name string, records mailboxRecords) error {
	for _, id := range records.journals {
		if err := w.finishJournal(ctx, name, id); err != nil {
			return err
		}
	}
	return nil
}

// MailboxStopped answers the stop a mailbox is in, nil when it is not stopped. A
// call that would change the mailbox asks it first, and answers the stop
// instead. A stop an effect met answers as recorded, since only the barrier
// can meet it again; otherwise the barrier's own plan decides, and what it
// finds is recorded in turn, or it lifts the stop on record (stop.go).
func MailboxStopped(dir, name string) error {
	recorded, err := recordedStop(dir, name)
	if err != nil {
		return err
	}
	if recorded != nil && recorded.Effect {
		return recorded.err(name)
	}
	if _, planned := plan(dir, name); planned != nil && !isStopped(planned) {
		return keepStop(dir, name, planned)
	}
	if recorded != nil {
		return liftStop(dir, name)
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
