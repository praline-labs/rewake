package space

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// The writes of a record. Each one runs under the room's space lock and
// through the state package, so it refuses without the room's lease and in a
// view; each reads the record, checks the step against its state and writes
// the whole record in one replacement, so a crash between two steps leaves
// the earlier one, which the next start can close from what it observes.

// maxRecordBytes bounds a record; a larger one is not read.
const maxRecordBytes = 16 << 10

// ErrNotFound is a space no record names.
var ErrNotFound = errors.New("no such space")

// NameTakenError is an open under a name a space that is not over holds.
type NameTakenError struct{ Name, ID string }

func (e *NameTakenError) Error() string {
	return fmt.Sprintf("the space %s is not over (record %s); close it or choose another name", e.Name, e.ID)
}

// StepError is a step the record's state does not allow.
type StepError struct {
	Name  string
	State State
	Step  string
}

func (e *StepError) Error() string {
	return fmt.Sprintf("the space %s is %s, and %s is not a step from there", e.Name, e.State, e.Step)
}

// Create writes the record of a space about to open, in state opening, with
// the worker it will start; nothing of the worker runs before it returns
// (SP2). The id and the name are the caller's; the name must be free among
// the room's spaces that are not over.
func Create(dir string, record Record) error {
	if record.State == "" {
		record.State = Opening
	}
	if record.State != Opening || record.End != nil || record.Worker.Run != "" || record.Closing != "" {
		return fmt.Errorf("a space's record begins opening, with no run of its worker and no end")
	}
	if err := record.check(); err != nil {
		return err
	}
	raw, err := encode(record)
	if err != nil {
		return err
	}
	return state.WithSpaceLock(dir, func() error {
		records, problems, err := List(dir)
		if err != nil {
			return err
		}
		// A record that does not read may hold the name: unknown is not
		// taken for free.
		if len(problems) > 0 {
			return fmt.Errorf("cannot tell whether the name %s is free: %s: %s", record.Name, problems[0].Path, problems[0].Why)
		}
		for _, other := range records {
			if other.Name == record.Name && !other.State.Final() {
				return &NameTakenError{Name: other.Name, ID: other.ID}
			}
		}
		if err := state.PublishExclusive(state.SpacePath(dir, record.ID), raw); err != nil {
			if errors.Is(err, state.ErrNameTaken) {
				return fmt.Errorf("a space record %s exists already", record.ID)
			}
			return err
		}
		return nil
	})
}

// SetHost records the space host's run, once it is detached and before it
// starts anything: a worker always has a record naming what closes it.
func SetHost(dir, id, hostRun string) (Record, error) {
	return change(dir, id, "recording the space host", func(r *Record) error {
		if r.State != Opening || r.Host != "" || hostRun == "" {
			return errStep
		}
		r.Host = hostRun
		return nil
	})
}

// SetCheckout records the worktree the worker's launch made, once its
// session is seen there; once recorded it does not change.
func SetCheckout(dir, id string, checkout Checkout) (Record, error) {
	return change(dir, id, "recording the checkout", func(r *Record) error {
		if (r.State != Opening && r.State != Open) || checkout.Path == "" || (r.Checkout.Path != "" && r.Checkout != checkout) {
			return errStep
		}
		r.Checkout = checkout
		return nil
	})
}

// Opened records the worker's run once the space host started its wrapper,
// before the open is answered.
func Opened(dir, id, workerRun string) (Record, error) {
	return change(dir, id, "opening", func(r *Record) error {
		if r.State != Opening || r.Host == "" || workerRun == "" {
			return errStep
		}
		r.State, r.Worker.Run = Open, workerRun
		return nil
	})
}

// BeginClosing records that the space is closing and why, before anything
// is stopped. A space already closing stays as it is.
func BeginClosing(dir, id, why string) (Record, error) {
	return change(dir, id, "closing", func(r *Record) error {
		switch {
		case r.State == Closing:
			return nil
		case r.State != Opening && r.State != Open, why == "":
			return errStep
		}
		r.State, r.Closing = Closing, why
		return nil
	})
}

// Close records what closing observed: the worker's end and the checkout as
// found after it. The space is kept when its checkout was found there or
// could not be looked at, and closed only when it was seen gone or never
// made. A record is closed only from an observation (SP2), so an end that
// observed nothing is refused; any state that is not over may close, since an
// open can fail at any step.
func Close(dir, id string, end End) (Record, error) {
	return change(dir, id, "closing from what was observed", func(r *Record) error {
		if r.State.Final() {
			return errStep
		}
		r.State, r.End = Closed, &end
		if end.Checkout.Presence != Gone {
			r.State = Kept
		}
		return nil
	})
}

// Cleared records that a kept checkout is gone, observed here and now: the
// space is then closed and no longer reminded (SP7). A checkout still there,
// or one whose presence cannot be told, leaves the record kept.
func Cleared(dir, id string) (Record, error) {
	return change(dir, id, "clearing the kept checkout", func(r *Record) error {
		if r.State != Kept {
			return errStep
		}
		if present := CheckoutPresence(*r); present != Gone {
			return fmt.Errorf("the checkout of space %s at %s is %s, not gone", r.Name, r.Checkout.Path, present)
		}
		now := time.Now().UTC()
		r.State, r.End.Cleared = Closed, &now
		return nil
	})
}

// errStep is a step's refusal, which change names with the record's state.
var errStep = errors.New("not a step from here")

// change runs one step on a record under the room's space lock.
func change(dir, id, step string, apply func(*Record) error) (Record, error) {
	if !ValidID(id) {
		return Record{}, fmt.Errorf("%w: %q is no space id", ErrNotFound, id)
	}
	if _, err := os.Lstat(state.SpacesPath(dir)); errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	var changed Record
	err := state.WithSpaceLock(dir, func() error {
		record, err := Load(dir, id)
		if err != nil {
			return err
		}
		if err := apply(&record); errors.Is(err, errStep) {
			return &StepError{Name: record.Name, State: record.State, Step: step}
		} else if err != nil {
			return err
		}
		if err := record.check(); err != nil {
			return err
		}
		raw, err := encode(record)
		if err != nil {
			return err
		}
		if err := state.WriteAtomic(state.SpacePath(dir, id), raw); err != nil {
			return err
		}
		changed = record
		return nil
	})
	return changed, err
}

func encode(record Record) ([]byte, error) {
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	if len(raw) > maxRecordBytes {
		return nil, fmt.Errorf("the record of space %s is past %d bytes", record.Name, maxRecordBytes)
	}
	return append(raw, '\n'), nil
}

// Load reads one record, as a view may: it writes nothing and takes no lock.
// A record that does not read, or says what no writer may, is an error and
// never a record half believed.
func Load(dir, id string) (Record, error) {
	if !ValidID(id) {
		return Record{}, fmt.Errorf("%w: %q is no space id", ErrNotFound, id)
	}
	path := state.SpacePath(dir, id)
	raw, err := state.ReadRegular(path, maxRecordBytes)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	// A record outlives the build that wrote it, so a field a later build
	// adds is read past, not refused.
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil {
		return Record{}, fmt.Errorf("not a space record: %w", err)
	}
	if record.ID != id {
		return Record{}, fmt.Errorf("the record of %s names the space %q", id, record.ID)
	}
	if err := record.check(); err != nil {
		return Record{}, err
	}
	return record, nil
}
