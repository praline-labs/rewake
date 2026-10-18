package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A turn end is recorded done as its last step (publishTurnContext), at
// journal/<op>.done, named by its operation — the run and the event
// (docs/turn-end-recovery.md) — so a retry of the same event finds its own
// and answers from it rather than run again. The record is kept for as long
// as the run that wrote it: however late a retry comes it learns its end was
// completed, and the next end's window opens after this one
// (TurnWindowStart). The sweep of the live run removes those of ended runs
// (sweepTurnRecords).

// TurnJournal is the record of a turn end done: its run and operation, when it
// ended, and the end held earlier whose kept answer it took.
type TurnJournal struct {
	Epoch string
	// Op names the operation, from its run and its event.
	Op string `json:",omitempty"`
	// Ended is when the turn ended on the boot clock. Zero for an end whose
	// time is not known — the outcomes the gateway reports with none — which
	// has no place among its run's ends.
	Ended int64 `json:",omitempty"`
	// Held is the operation of the end whose kept answer this one took: an
	// end held earlier, which a retry may confirm again, learning it was
	// published with its continuation (HeldEndTaken).
	Held string `json:",omitempty"`
	Done bool   `json:",omitempty"`
}

// JournalPath holds the turn journals of a mailbox, one per turn end.
func JournalPath(dir, name string) string {
	return filepath.Join(state.InboxPath(dir, name), "journal")
}

// writeDoneJournal records the turn end op of name as done.
func writeDoneJournal(dir, name, op string, journal TurnJournal) error {
	if err := state.EnsureSubdir(JournalPath(dir, name)); err != nil {
		return err
	}
	return writeJournalFile(filepath.Join(JournalPath(dir, name), op+doneSuffix), journal)
}

func writeJournalFile(path string, journal TurnJournal) error {
	raw, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	return state.WriteAtomic(path, raw)
}

func readJournalFile(path string) (TurnJournal, error) {
	raw, err := live("").readFile(path)
	if err != nil {
		return TurnJournal{}, err
	}
	return parseJournal(path, raw)
}

// parseJournal reads a journal by the fields it still needs: a field it does
// not know — one an earlier build wrote — is passed over. One that does not
// parse may be this run's, and is an error.
func parseJournal(path string, raw []byte) (TurnJournal, error) {
	var journal TurnJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		return TurnJournal{}, fmt.Errorf("the turn journal %s is not readable: %w", path, err)
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

// HeldEndTaken says whether a journal of run epoch took the kept answer of the
// end held as op: that end was published with its continuation. A journal
// that cannot be read may be that one, and is an error.
func HeldEndTaken(dir, name, epoch, op string) (bool, error) {
	entries, err := os.ReadDir(JournalPath(dir, name))
	if errors.Is(err, os.ErrNotExist) || op == "" {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name()[0] == '.' {
			continue
		}
		journal, err := readJournalFile(filepath.Join(JournalPath(dir, name), entry.Name()))
		if err != nil {
			return false, err
		}
		if journal.Epoch == epoch && journal.Held == op {
			return true, nil
		}
	}
	return false, nil
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
