package inbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// After an interim turn end, rewake's Stop hook on Claude Code holds the next
// turn end that carries no mark, once, and asks the session whether the work
// is done (docs/turn-outcomes.md#the-confirmation-on-claude-code). Two records
// serve it, beside the pending mark and like it changed only under the
// mailbox lock:
//
//   - interim.json says the last turn end this run published was interim, and
//     with which line. It is what makes an unmarked end worth asking about. An
//     interim end writes it; a finished or failed one removes it, and a stop
//     leaves it, as a stop leaves the waits.
//   - kept.json keeps the answer of the turn end that was held, until the next
//     turn end heard takes it into its own outcome: the continuation's Stop, a
//     StopFailure, or the plugin's stop after an Esc. Without it the report
//     would be the continuation alone, since the harness's second Stop carries
//     only what the model said after the hold.

type confirmRecord struct {
	Epoch string `json:"epoch"`
	Text  string `json:"text"`
}

func interimPath(dir, name string) string {
	return filepath.Join(pendingDir(dir, name), "interim.json")
}

func keptPath(dir, name string) string { return filepath.Join(pendingDir(dir, name), "kept.json") }

func writeConfirmRecord(path, epoch, text string) error {
	if err := state.EnsureSubdir(filepath.Dir(path)); err != nil {
		return err
	}
	raw, err := json.Marshal(confirmRecord{Epoch: epoch, Text: text})
	if err != nil {
		return err
	}
	return state.WriteAtomic(path, raw)
}

// readConfirmRecord answers the record's text when it belongs to this run. A
// record of an earlier run, or one that cannot be read, is none.
func readConfirmRecord(path, epoch string) (string, bool) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var record confirmRecord
	if json.Unmarshal(raw, &record) != nil || record.Epoch != epoch {
		return "", false
	}
	return record.Text, true
}

func removeConfirmRecord(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// NoteInterim records that this run's last turn end was interim, marked with
// line.
func NoteInterim(dir, name, epoch, line string) error {
	return writeConfirmRecord(interimPath(dir, name), epoch, line)
}

// ClearInterim records that this run's last turn end settled the work, or
// failed.
func ClearInterim(dir, name string) error { return removeConfirmRecord(interimPath(dir, name)) }

// LastInterim answers the pending line of this run's last turn end, when that
// end was interim.
func LastInterim(dir, name, epoch string) (string, bool) {
	return readConfirmRecord(interimPath(dir, name), epoch)
}

// KeepAnswer keeps the answer of a turn end that was held.
func KeepAnswer(dir, name, epoch, text string) error {
	return writeConfirmRecord(keptPath(dir, name), epoch, text)
}

// KeptAnswer answers the kept answer of this run's held turn end, and whether
// there is one.
func KeptAnswer(dir, name, epoch string) (string, bool) {
	return readConfirmRecord(keptPath(dir, name), epoch)
}

// DropKeptAnswer forgets the kept answer, once a turn end has published it.
func DropKeptAnswer(dir, name string) error { return removeConfirmRecord(keptPath(dir, name)) }

// MarkedWithin says whether the turn between started and ended made a pending
// mark that its end would honor. It asks what TakePending would answer, and
// takes nothing.
func MarkedWithin(dir, name, epoch string, started, ended int64) bool {
	raw, err := os.ReadFile(pendingPath(dir, name))
	if err != nil {
		return false
	}
	var mark pendingMark
	if json.Unmarshal(raw, &mark) != nil {
		return false
	}
	return mark.Epoch == epoch && mark.Text != "" && mark.At != 0 && started != 0 && started <= mark.At && ended != 0 && mark.At <= ended
}
