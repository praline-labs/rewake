package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/registry"
)

// interimRecord is a run's last word on whether the work goes on
// (docs/turn-end-recovery.md#the-interim-record): the operation that wrote it,
// the time its turn ended on the boot clock, and either the pending line or
// Settled. A settling end writes Settled rather than removing the record, so a
// late end cannot bring back a line an end after it settled. An earlier
// build's record carries no operation or time, and is another run's by the
// time this build reads it.
type interimRecord struct {
	Epoch   string `json:"epoch"`
	Op      string `json:"op,omitempty"`
	Ended   int64  `json:"ended,omitempty"`
	Text    string `json:"text,omitempty"`
	Settled bool   `json:"settled,omitempty"`
}

func interimPath(dir, name string) string {
	return filepath.Join(pendingDir(dir, name), "interim.json")
}

// readInterim answers the interim record whichever run wrote it, nil when
// there is none, and an error naming the path when it cannot be read.
func readInterim(dir, name string) (*interimRecord, error) { return live(dir).readInterim(name) }

func (w world) readInterim(name string) (*interimRecord, error) {
	dir := w.dir
	raw, err := w.readFile(interimPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, unknownRecord(interimPath(dir, name), fmt.Errorf("the interim record %s cannot be read: %w", interimPath(dir, name), err))
	}
	var record interimRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, unknownRecord(interimPath(dir, name), fmt.Errorf("the interim record %s is not readable: %w", interimPath(dir, name), err))
	}
	return &record, nil
}

// LastInterim answers the pending line of this run's last word, when that
// word is an interim end.
func LastInterim(dir, name, epoch string) (string, bool, error) {
	record, err := readInterim(dir, name)
	if err != nil || record == nil || record.Epoch != epoch || record.Settled {
		return "", false, err
	}
	return record.Text, true, nil
}

// recordInterim is the step of a turn end's journal that records the end's
// word: line for an interim end, settles for a finished or failed one; a stop
// says nothing of the work and records nothing. It writes only over a record
// that is absent, another run's, or ended earlier than this end, and orders
// two of the same time without regard to the order of retries: interim wins
// over settled, since it only makes the next unmarked end ask once, and
// between two of a kind the operation whose name sorts last. Ended is
// compared only within one run: a boot clock says nothing across runs.
//
// A journal of a run that no longer holds the name, completed by the next
// run's barrier, leaves that run's record alone: only the run holding the
// name is ever asked about its interim end.
func recordInterim(dir, name, epoch, op string, ended int64, line *string, settles bool) error {
	return live(dir).recordInterim(name, epoch, op, ended, line, settles)
}

func (w world) recordInterim(name, epoch, op string, ended int64, line *string, settles bool) error {
	if line == nil && !settles {
		return nil
	}
	dir := w.dir
	record, err := w.readInterim(name)
	if err != nil {
		return err
	}
	if record != nil && record.Epoch == epoch {
		switch {
		case record.Op == op:
			return nil
		case record.Ended > ended:
			return nil
		case record.Ended == ended:
			mine, theirs := line != nil, !record.Settled
			if mine != theirs && !mine || mine == theirs && op < record.Op {
				return nil
			}
		}
	} else if record != nil && !registry.OwnsName(dir, name, epoch) {
		return nil
	}
	fresh := interimRecord{Epoch: epoch, Op: op, Ended: ended, Settled: line == nil}
	if line != nil {
		fresh.Text = *line
	}
	if err := w.ensureDir(pendingDir(dir, name)); err != nil {
		return err
	}
	raw, err := json.Marshal(fresh)
	if err != nil {
		return err
	}
	return w.writeFile(interimPath(dir, name), raw)
}
