package inbox

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// A pending mark says the turn now running is not the end of the work. It
// belongs to the turn it was made in and to no other, and the tie is time, on
// the machine's boot clock (internal/boottime): the mark records when the
// `rewake pending` process started, and a turn end honors it only if that falls
// between the start and the end of the ending turn, as the harness side saw
// them. Everything else falls to the safe side, which is the report:
//
//   - a mark older than the ending turn's start was made in an earlier turn
//     whose end never reached rewake — interrupted, or lost on the way — and
//     is removed unused;
//   - a mark newer than the ending turn's end was made in a later turn, while
//     this one's end was still being published, and is left for that turn;
//   - a turn whose start is not known cannot be tied to the mark: the turn end
//     is a report, and the mark is removed if it is not a later turn's.
//
// The mark changes only under the mailbox lock: `rewake pending` makes it
// under it, and the end of a turn takes it under it.

type pendingMark struct {
	Epoch string `json:"epoch"`
	// At is when the marking process started, on the boot clock.
	At   int64  `json:"at"`
	Text string `json:"text"`
}

// pendingDir holds the mark apart from the messages: everything directly in a
// mailbox is mail.
func pendingDir(dir, name string) string { return filepath.Join(state.InboxPath(dir, name), "pending") }

func pendingPath(dir, name string) string { return filepath.Join(pendingDir(dir, name), "mark.json") }

// MarkPending records that the turn running at `at` ends before the work
// does. The caller holds the mailbox lock. A second mark replaces the first:
// the latest word on what is being waited for is the one sent.
func MarkPending(dir, name, epoch, text string, at int64) error {
	if err := state.EnsureSubdir(pendingDir(dir, name)); err != nil {
		return err
	}
	raw, err := json.Marshal(pendingMark{Epoch: epoch, At: at, Text: text})
	if err != nil {
		return err
	}
	return state.WriteAtomic(pendingPath(dir, name), raw)
}

// TakePending is asked once for each turn end, under the mailbox lock, with
// the turn's start and end on the boot clock (zero where unknown). It answers
// the mark's text when the mark was made within that turn, and removes every
// mark that cannot belong to a later one.
func TakePending(dir, name, epoch string, started, ended int64) (string, bool, error) {
	raw, err := os.ReadFile(pendingPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	var mark pendingMark
	if json.Unmarshal(raw, &mark) == nil && mark.Epoch == epoch && (ended == 0 || mark.At > ended) {
		// A later turn's, or this turn's end is unknown and so cannot be
		// compared: kept, and this turn end is a report.
		return "", false, nil
	}
	if err := os.Remove(pendingPath(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}
	valid := mark.Epoch == epoch && mark.Text != "" && mark.At != 0 && started != 0 && started <= mark.At
	if !valid {
		return "", false, nil
	}
	return mark.Text, true, nil
}
