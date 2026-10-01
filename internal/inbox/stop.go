package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// A stop, once found, is a record of the mailbox (8-stop), which every later
// call that would change the mailbox answers first, whatever call found it.
// How it is lifted depends on what found it. One the plan found — a file
// the list does not know, or one that does not read (records.go), or what an
// effect left to make decides by (reconcile.go) — is lifted by the next plan
// that finds nothing, by any call: the plan is the same for every call, so a
// cause it no longer finds is gone. One an effect met, which the plan did not
// show, is lifted only by the barrier, once every effect has run through: a
// call that is not the barrier cannot find it again, and a barrier that was
// canceled, failed, or met another stop has not shown the effect's cause
// gone, so none of them may forget it.

// stopFile holds the stop a mailbox is in.
const stopFile = "stopped"

type stopRecord struct {
	Cause string `json:"cause"`
	// Effect says an effect met a cause the reading did not show; Met is
	// that cause, kept under any later one until the barrier runs the effect
	// through.
	Effect bool      `json:"effect,omitempty"`
	Met    string    `json:"met,omitempty"`
	At     time.Time `json:"at"`
}

// UnknownRecordError is a record an effect needed and could not read as what
// it is: an unknown, not a failure a retry may clear.
type UnknownRecordError struct {
	Path string
	Err  error
}

func (e *UnknownRecordError) Error() string { return e.Err.Error() }

func (e *UnknownRecordError) Unwrap() error { return e.Err }

// unknownRecord marks err as the unknown a record at path leaves.
func unknownRecord(path string, err error) error {
	return &UnknownRecordError{Path: path, Err: err}
}

func stopPath(dir, name string) string { return filepath.Join(state.InboxPath(dir, name), stopFile) }

// RecordedStopError is a stop some earlier call found and recorded.
type RecordedStopError struct {
	Name, Cause string
}

func (e *RecordedStopError) Error() string {
	return fmt.Sprintf("%s stopped changing its mailbox: %s; the next turn end looks again", e.Name, e.Cause)
}

// recordedStop answers the stop on record, nil when there is none. One that
// cannot be read is a stop too, with its cause unknown, which only the
// barrier lifts: an effect may have found it.
func recordedStop(dir, name string) (*stopRecord, error) {
	raw, err := live(dir).readFile(stopPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	var record stopRecord
	if err == nil {
		err = json.Unmarshal(raw, &record)
	}
	if err != nil {
		return &stopRecord{Effect: true, Cause: fmt.Sprintf("the stop record %s cannot be read (%v), so why it stopped is unknown", stopPath(dir, name), err)}, nil
	}
	return &record, nil
}

func (r *stopRecord) err(name string) error {
	cause := r.Cause
	if r.Met != "" && r.Met != r.Cause {
		cause += "; before it, an effect met: " + r.Met
	}
	return &RecordedStopError{Name: name, Cause: cause}
}

// keepStop records the stop the plan found, cause, and answers it. It does
// not replace a stop an effect met: that stands until the barrier runs the
// effect through, whatever else a retry meets. The stop stands whether or not
// its record could be written; a record that could not is named with it.
func keepStop(dir, name string, cause error) error {
	var recorded *RecordedStopError
	if errors.As(cause, &recorded) {
		return cause
	}
	if err := recordStop(dir, name, cause, ""); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

// recordStop writes the stop cause is. met is the cause an effect met in the
// calling barrier, empty when none did: the caller holds it, so it is never
// taken from the record, which a write this same call failed may not hold.
// A stop the plan found keeps an earlier call's effect stop under it.
func recordStop(dir, name string, cause error, met string) error {
	record := stopRecord{Cause: cause.Error(), Effect: met != "", Met: met, At: time.Now()}
	if met == "" {
		if prior, _ := recordedStop(dir, name); prior != nil && prior.Effect {
			record.Effect, record.Met = true, prior.Met
			if record.Met == "" {
				record.Met = prior.Cause
			}
		}
	}
	raw, err := json.Marshal(record)
	w := live(dir)
	if err == nil {
		err = w.ensureDir(state.InboxPath(dir, name))
	}
	if err == nil {
		err = w.writeFile(stopPath(dir, name), raw)
	}
	if err != nil {
		return fmt.Errorf("could not record the stop: %w", err)
	}
	return nil
}

// liftStop removes the stop on record: one the reading found, once a reading
// finds nothing unknown; one an effect met, once every effect ran through.
func liftStop(dir, name string) error {
	w := live(dir)
	err := w.files.Remove(stopPath(dir, name))
	if err == nil {
		return w.syncDir(state.InboxPath(dir, name))
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return fmt.Errorf("could not lift the stop of %s: %w", name, err)
}
