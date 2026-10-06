package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/praline-labs/rewake/internal/state"
)

// A turn end publishes its reports, takes the kept answer they carry, clears
// the waits they answer, and records whether it was interim. These cannot be
// one step, and between them the mailbox says a task is still owed that was
// reported already: a task read since from the same sender joins that wait,
// and the next report answers the old task a second time. So the whole
// operation is written down before its first effect, and nothing but the
// journal publishes: whoever finds it unfinished completes it under the
// mailbox lock before a wait is read again (Reconcile). A journal is named by
// its operation — the run and the event (docs/turn-end-recovery.md) — so a
// retry that finds it finds its own.
//
// A completed journal is kept, emptied to its operation and its Ended and
// marked done, for as long as the run that wrote it: a retry, however late,
// learns that its end was completed rather than prepare it again, and the
// next end's window opens after this one (TurnWindowStart). It is renamed
// with doneSuffix, so the barrier does not read it again; the sweep of the
// live run removes those of ended runs (sweepTurnRecords).

// TurnJournal is what one turn end is about to do: publish Reports, take the
// kept answer of version Kept when it is set, then clear Clear from the waits
// of the run Epoch and record whether the end was interim.
type TurnJournal struct {
	Epoch string
	// Op names the operation, from its run and its event.
	Op string `json:",omitempty"`
	// Ended is when the turn ended on the boot clock, kept after the journal
	// is done. Zero for an end whose time is not known — the outcomes the
	// gateway reports with none — which has no place among its run's ends.
	Ended   int64     `json:",omitempty"`
	Reports []Message `json:",omitempty"`
	Clear   []Waiter  `json:",omitempty"`
	// Kept is the version of the kept answer the reports carry; nil when they
	// carry none.
	Kept *string `json:",omitempty"`
	// Mark is the pending mark that made the end interim. Using it is no
	// effect: it stays for its run's life.
	Mark *Mark `json:",omitempty"`
	// Interim is the pending line of an interim end, recorded as this run's
	// last word for the next unmarked end to be asked about; Settles records
	// that the end settled the work.
	Interim *string `json:",omitempty"`
	Settles bool    `json:",omitempty"`
	// Published lists the reports this journal has put in their mailboxes.
	// The recipient keeps the proof as well (publishMarked), which covers a
	// report put there whose entry here was never written.
	Published []string `json:",omitempty"`
	// Moot lists the reports that can reach nobody: their run ended.
	Moot []string `json:",omitempty"`
	Done bool     `json:",omitempty"`
}

// JournalPath holds the turn journals of a mailbox, one per turn end.
func JournalPath(dir, name string) string {
	return filepath.Join(state.InboxPath(dir, name), "journal")
}

// WriteJournal records a turn end's operation under id before any of it is
// done.
func WriteJournal(dir, name, id string, journal TurnJournal) error {
	if err := state.EnsureSubdir(JournalPath(dir, name)); err != nil {
		return err
	}
	return writeJournalFile(filepath.Join(JournalPath(dir, name), id), journal)
}

func writeJournalFile(path string, journal TurnJournal) error {
	return world{files: osAccess{}}.writeJournalFile(path, journal)
}

func (w world) writeJournalFile(path string, journal TurnJournal) error {
	raw, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	return w.writeFile(path, raw)
}

func readJournalFile(path string) (TurnJournal, error) {
	return world{files: osAccess{}}.readJournalFile(path)
}

func (w world) readJournalFile(path string) (TurnJournal, error) {
	raw, err := w.readFile(path)
	if err != nil {
		return TurnJournal{}, err
	}
	return parseJournal(path, raw)
}

func parseJournal(path string, raw []byte) (TurnJournal, error) {
	var journal TurnJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return TurnJournal{}, unknownRecord(path, fmt.Errorf("the turn journal %s is not readable: %w", path, err))
	}
	return journal, nil
}

// doneSuffix names a completed journal.
const doneSuffix = ".done"

// JournalRecorded says whether a journal was written under id, completed or
// not.
func JournalRecorded(dir, name, id string) (bool, error) {
	for _, file := range []string{id, id + doneSuffix} {
		_, err := os.Stat(filepath.Join(JournalPath(dir, name), file))
		if !errors.Is(err, os.ErrNotExist) {
			return err == nil, err
		}
	}
	return false, nil
}

// finishJournal completes the operation recorded under id and marks it done;
// an error leaves the rest for the next attempt. A journal done already, or
// never written, needs nothing. Only the barrier runs it, after its plan
// (Reconcile).
func (w world) finishJournal(ctx context.Context, name, id string) error {
	path := filepath.Join(JournalPath(w.dir, name), id)
	journal, err := w.readJournalFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if journal.Done {
		return w.retireJournal(path)
	}
	save := func() error { return w.writeJournalFile(path, journal) }
	if err := w.completeJournal(ctx, name, &journal, save); err != nil {
		return err
	}
	// Done in place first: an end that dies before the rename leaves a journal
	// that reads as done, not one whose operation would be run again.
	if err := w.writeJournalFile(path, TurnJournal{Epoch: journal.Epoch, Op: journal.Op, Ended: journal.Ended, Done: true}); err != nil {
		return err
	}
	return w.retireJournal(path)
}

// completeJournal runs every step of a journal not done yet, saving its
// progress after each report.
func (w world) completeJournal(ctx context.Context, name string, journal *TurnJournal, save func() error) error {
	for _, report := range journal.Reports {
		if slices.Contains(journal.Published, report.ID) || slices.Contains(journal.Moot, report.ID) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.deliverReport(ctx, name, save, journal, report); err != nil {
			return err
		}
	}
	return w.finishSteps(ctx, name, *journal)
}

// finishSteps takes the kept answer, clears the waits and records the
// interim end.
func (w world) finishSteps(ctx context.Context, name string, journal TurnJournal) error {
	if journal.Kept != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.dropKeptVersion(name, journal.Epoch, *journal.Kept); err != nil {
			return err
		}
	}
	for _, waiter := range journal.Clear {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := w.clearAwaiting(name, journal.Epoch, waiter); err != nil {
			return fmt.Errorf("could not clear what %s was owed: %w", waiter.Name, err)
		}
	}
	return w.recordInterim(name, journal.Epoch, journal.Op, journal.Ended, journal.Interim, journal.Settles)
}

func (w world) retireJournal(path string) error {
	if err := w.rename(path, path+doneSuffix); err != nil {
		return err
	}
	return w.syncDir(filepath.Dir(path))
}

// TurnWindowStart is where the window of a turn end of run epoch opens
// (docs/turn-end-recovery.md#pending-marks): the later of the turn's start the
// event carries and the latest Ended of this run that a journal records below
// ended; with neither, zero, which is before the run's first mark. So a start
// that failed to be recorded widens nothing: a mark an earlier end of the run
// used, or could have, lies at or before that end. A journal that cannot be
// read may be this run's, and is an error.
func TurnWindowStart(dir, name, epoch string, started, ended int64) (int64, error) {
	entries, err := os.ReadDir(JournalPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return started, nil
	}
	if err != nil {
		return 0, err
	}
	start := started
	for _, entry := range entries {
		if entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}
		journal, err := readJournalFile(filepath.Join(JournalPath(dir, name), entry.Name()))
		if err != nil {
			return 0, err
		}
		if journal.Epoch == epoch && journal.Ended < ended {
			start = max(start, journal.Ended)
		}
	}
	return start, nil
}
