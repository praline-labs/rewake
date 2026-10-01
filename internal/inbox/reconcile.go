package inbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/registry"
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
// through. Then the same code runs for real: it meets the earlier build's
// receipts through one conversion journal, and completes every unfinished
// journal of this build, whichever run wrote it: an ended run's reports are
// out, and its waits may be taken over by the next run, which must not
// report on them again. An error stops whatever the caller was about to
// change: a record that could not be read, which a later look may settle, or
// a *StoppedError, which a person settles.
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
		// A conversion stopped on reports nothing can decide is the plan's own
		// outcome, reached once every effect before it ran through: that
		// stop, recorded with main told, is what the mailbox answers now, not
		// an effect's cause no effect met this time.
		if isStopped(err) {
			if effect {
				if lift := liftStop(dir, name); lift != nil {
					return lift
				}
			}
			return err
		}
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
// nothing (access.go). It answers what the reading found. A conversion that
// stops on reports nothing can decide is a planned outcome, not an unknown:
// the effects record it.
func plan(dir, name string) (mailboxRecords, error) {
	w := planning(dir)
	records, err := w.inspectMailbox(name)
	if err != nil {
		return records, err
	}
	dry := records
	dry.conversion = records.conversion.clone()
	return records, w.reconcileRecords(context.Background(), name, dry)
}

func isStopped(err error) bool {
	var stopped *StoppedError
	return errors.As(err, &stopped)
}

// afterReading runs between the barrier's plan and its first effect, and
// afterEffect between a failed effect and the plan after it; a test changes
// a record there, which only an effect then meets.
var afterReading, afterEffect = func() {}, func() {}

func (w world) reconcileRecords(ctx context.Context, name string, records mailboxRecords) error {
	conversion, err := w.convertReceipts(name, records)
	if err != nil {
		return err
	}
	if conversion != nil {
		if err := w.finishConversion(ctx, name, conversion); err != nil {
			return err
		}
	}
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
// finds is recorded in turn, or it lifts the stop on record (stop.go). A
// conversion stopped on reports nothing can decide answers once the barrier
// has recorded it, with main told: until then it is the barrier's outcome to
// come, not an unknown.
func MailboxStopped(dir, name string) error {
	recorded, err := recordedStop(dir, name)
	if err != nil {
		return err
	}
	if recorded != nil && recorded.Effect {
		return recorded.err(name)
	}
	records, planned := plan(dir, name)
	if planned != nil && !isStopped(planned) {
		return keepStop(dir, name, planned)
	}
	if recorded != nil {
		if err := liftStop(dir, name); err != nil {
			return err
		}
	}
	conversion := records.conversion
	if conversion == nil || conversion.Done || len(conversion.Unknown) == 0 {
		return nil
	}
	return conversion.stopped(dir, name)
}

// mailboxRecords is what the barrier read before any effect.
type mailboxRecords struct {
	conversion *conversionJournal
	// receipts are the earlier build's receipts no conversion journal holds
	// yet; leftovers are those one holds, which a conversion that died before
	// removing them left behind.
	receipts  []convertedReceipt
	leftovers []string
	// journals names the unfinished journals of this build.
	journals []string
}

// inspectMailbox is step 1 of the plan: it reads every file of the mailbox
// by its kind (records.go), then what the barrier weighs — the conversion
// journal, the earlier build's receipts, every unfinished journal and every
// wait record — and changes nothing. One that cannot be read stops the
// mailbox, naming it, and so does a receipt that appeared, or changed, once a
// conversion journal is on record: a writer of the earlier build the launch
// did not see (8-cutover).
func (w world) inspectMailbox(name string) (mailboxRecords, error) {
	var records mailboxRecords
	dir := w.dir
	if err := w.readMailbox(name); err != nil {
		return records, err
	}
	conversion, err := w.readConversion(name)
	if err != nil {
		return records, err
	}
	records.conversion = conversion
	receipts, err := w.earlierReceipts(name)
	if err != nil {
		return records, err
	}
	for _, receipt := range receipts {
		switch {
		case conversion == nil:
			records.receipts = append(records.receipts, receipt)
		case conversion.holds(receipt):
			records.leftovers = append(records.leftovers, receipt.File)
		default:
			return records, fmt.Errorf("an earlier build's turn receipt %s appeared or changed after the conversion of this mailbox, from a writer of that build still running; nothing changes in this mailbox until it is stopped and the receipt is removed", filepath.Join(TurnsPath(dir, name), receipt.File))
		}
	}
	entries, err := w.readDir(JournalPath(dir, name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return records, fmt.Errorf("the turn journals %s cannot be listed: %w", JournalPath(dir, name), err)
	}
	for _, entry := range entries {
		// A dot is a write that never finished (state.WriteAtomic), not a
		// journal.
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasSuffix(entry.Name(), doneSuffix) || entry.Name() == conversionFile {
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
// mailbox. So does a wait of a run of this build without its place on the
// read clock: this build writes none such, so a writer of the earlier build
// wrote it after the successor was bound (docs/protocol-cutover.md), and an
// end heard once would answer it without a scope.
func (w world) readEveryWait(name string) error {
	dir := w.dir
	runs, err := w.readDir(state.AwaitingPath(dir, name))
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
		waiters, err := w.readWaiters(name, run.Name())
		if err != nil {
			return fmt.Errorf("the waits of %s's run %s cannot be read, so what is owed is unknown: %w", name, run.Name(), err)
		}
		if _, _, boot, ok := registry.ParseRun(run.Name()); !ok || boot == "" {
			continue
		}
		for _, waiter := range waiters {
			for i := range waiter.Messages {
				if i >= len(waiter.ReadSequences) || waiter.ReadSequences[i] == 0 {
					return fmt.Errorf("the wait of %s's run %s for %s carries no place on the read clock, so a writer of the earlier build wrote it; nothing changes in this mailbox until that writer is stopped and the record %s is removed", name, run.Name(), waiter.Name, filepath.Join(state.AwaitingPath(dir, name), run.Name(), waiter.Name))
				}
			}
		}
	}
	return nil
}

// Settle records a person's decision on one report the mailbox stopped on,
// before any effect, then runs the barrier, which reconciles again with that
// decision among the evidence. The same words again answer the recorded
// decision; the opposite words are refused, naming it; a report no journal
// names, or one that is not unknown, is refused. The caller holds the mailbox
// lock. It answers whether this call recorded the decision.
func Settle(ctx context.Context, dir, name, report string, delivered bool) (bool, error) {
	conversion, err := readConversion(dir, name)
	if err != nil {
		return false, err
	}
	if conversion == nil || !conversion.names(report) {
		return false, &SettleRefusal{Reason: fmt.Sprintf("no journal of %s names the report %s, so it is unknown to rewake", name, report)}
	}
	if recorded, ok := conversion.Settled[report]; ok {
		if recorded != delivered {
			return false, &SettleRefusal{Reason: fmt.Sprintf("the report %s was settled %s already; that decision stands", report, settledWord(recorded))}
		}
		return false, Reconcile(ctx, dir, name)
	}
	if !slices.Contains(conversion.Unknown, report) {
		return false, &SettleRefusal{Reason: fmt.Sprintf("the report %s is not one rewake cannot tell was delivered; only those are settled", report)}
	}
	if conversion.Settled == nil {
		conversion.Settled = map[string]bool{}
	}
	conversion.Settled[report] = delivered
	if err := conversion.save(dir, name); err != nil {
		return false, err
	}
	if err := live(dir).syncDir(JournalPath(dir, name)); err != nil {
		return true, err
	}
	return true, Reconcile(ctx, dir, name)
}

// SettleRefusal is a settle the journal does not allow.
type SettleRefusal struct{ Reason string }

func (e *SettleRefusal) Error() string { return e.Reason }

func settledWord(delivered bool) string {
	if delivered {
		return "delivered"
	}
	return "undelivered"
}

func (c *conversionJournal) names(report string) bool {
	for _, ref := range c.reports() {
		if ref.report.ID == report {
			return true
		}
	}
	return false
}
