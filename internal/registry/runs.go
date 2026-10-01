package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/state"
)

// A run of this build records itself before its session record is published,
// under its boot and epoch, and the record outlives the registry, which keeps
// only the current run of a name: which build a run followed has to be known
// after it has ended (docs/protocol-cutover.md). Run records and the successor
// record are never swept: one small file per launch, and one per name.

// RunRecord is what a run of this build keeps of itself.
type RunRecord struct {
	Name  string `json:"name"`
	Boot  string `json:"boot"`
	Epoch string `json:"epoch"`
	Build string `json:"build"`
	// Started is when the run started on the boot clock.
	Started int64 `json:"started"`
	// PIDNamespace is the namespace its pid means something in: its wrapper's
	// life is judged only from there.
	PIDNamespace string `json:"pidNamespace"`
}

func runsPath(dir, name string) string { return filepath.Join(dir, "runs", name) }

func runRecordPath(dir, name, boot, epoch string) string {
	return filepath.Join(runsPath(dir, name), boot, epoch)
}

// WriteRunRecord records a run of this build.
func WriteRunRecord(dir string, record RunRecord) error {
	if !state.ValidName(record.Name) {
		return fmt.Errorf("%w: %q", ErrUnusableName, record.Name)
	}
	if _, _, boot, ok := ParseRun(record.Epoch); !ok || boot == "" || boot != record.Boot {
		return fmt.Errorf("a run record needs an epoch of this build, not %q", record.Epoch)
	}
	for _, directory := range []string{filepath.Join(dir, "runs"), runsPath(dir, record.Name), filepath.Join(runsPath(dir, record.Name), record.Boot)} {
		if err := state.EnsureSubdir(directory); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return state.WriteAtomic(runRecordPath(dir, record.Name, record.Boot, record.Epoch), raw)
}

// ReadRunRecord reads the record of a run of this build. A run named by its
// epoch alone is an earlier build's and is never looked up here, where after
// a restart of the machine its epoch could match another run's; found is
// false for it. A run that names a boot and has no readable record is unknown,
// an error naming the path: never read as the earlier build's.
func ReadRunRecord(dir, name, epoch string) (RunRecord, bool, error) {
	_, _, boot, ok := ParseRun(epoch)
	if !ok || boot == "" || !state.ValidName(name) {
		return RunRecord{}, false, nil
	}
	path := runRecordPath(dir, name, boot, epoch)
	raw, err := os.ReadFile(path)
	if err != nil {
		return RunRecord{}, false, fmt.Errorf("the run record %s cannot be read, so the run's build is unknown: %w", path, err)
	}
	var record RunRecord
	if err := json.Unmarshal(raw, &record); err != nil || record.Epoch != epoch {
		return RunRecord{}, false, fmt.Errorf("the run record %s is not readable, so the run's build is unknown", path)
	}
	return record, true, nil
}

func successorPath(dir, name string) string { return filepath.Join(runsPath(dir, name), "successor") }

type successorRecord struct {
	Boot  string `json:"boot"`
	Epoch string `json:"epoch"`
}

// BindSuccessor makes the run epoch the successor of the name's earlier-build
// runs, only if no run is bound yet: the first run of this build to do so is
// the successor for good, and a later run never replaces it, even once it has
// ended. It answers whether this call bound it. A successor record that is
// there and cannot be read is an error: the reports held for the name wait on
// it, and a launch that went on would leave them unknown for good.
func BindSuccessor(dir, name, epoch string) (bool, error) {
	_, _, boot, ok := ParseRun(epoch)
	if !ok || boot == "" || !state.ValidName(name) {
		return false, fmt.Errorf("a successor needs a run of this build, not %q", epoch)
	}
	for _, directory := range []string{filepath.Join(dir, "runs"), runsPath(dir, name)} {
		if err := state.EnsureSubdir(directory); err != nil {
			return false, err
		}
	}
	raw, err := json.Marshal(successorRecord{Boot: boot, Epoch: epoch})
	if err != nil {
		return false, err
	}
	err = state.PublishExclusive(successorPath(dir, name), raw)
	if errors.Is(err, state.ErrNameTaken) {
		_, err = readSuccessor(dir, name)
		return false, err
	}
	return err == nil, err
}

// readSuccessor reads the successor record, which must name a run of this
// build; os.ErrNotExist when none is bound.
func readSuccessor(dir, name string) (successorRecord, error) {
	raw, err := os.ReadFile(successorPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return successorRecord{}, err
	}
	var record successorRecord
	if err == nil {
		err = json.Unmarshal(raw, &record)
	}
	if err == nil {
		if _, _, boot, ok := ParseRun(record.Epoch); !ok || boot == "" || boot != record.Boot {
			err = errors.New("it names no run of this build")
		}
	}
	if err != nil {
		return successorRecord{}, fmt.Errorf("the successor record %s is not readable: %w", successorPath(dir, name), err)
	}
	return record, nil
}

// SuccessorState is where the successor of a name's earlier-build runs stands.
type SuccessorState int

const (
	// SuccessorUnknown says the successor or its wrapper cannot be read.
	SuccessorUnknown SuccessorState = iota
	// SuccessorNone says no run of this build bound itself yet.
	SuccessorNone
	// SuccessorStarting says it is bound and its wrapper is alive, with no
	// session record of it published yet.
	SuccessorStarting
	// SuccessorReady says its session record names it and its wrapper lives.
	SuccessorReady
	// SuccessorGone says its boot is not the current one, or its wrapper ended.
	SuccessorGone
)

// Successor tells the name's successor apart, as a sender's barrier needs it:
// only gone makes a report held for its predecessors moot, and a missing
// session record is never taken for an ended run, since between binding and
// publishing it is missing by design. The epoch is the successor's, when one
// is bound; the error says why its state is unknown.
func Successor(dir, name string) (SuccessorState, string, error) {
	record, err := readSuccessor(dir, name)
	if errors.Is(err, os.ErrNotExist) {
		return SuccessorNone, "", nil
	}
	if err != nil {
		return SuccessorUnknown, "", err
	}
	return successorOf(dir, name, record.Epoch)
}

func successorOf(dir, name, epoch string) (SuccessorState, string, error) {
	if _, _, boot, _ := ParseRun(epoch); !isCurrentBoot(boot) {
		if _, err := currentBoot(); err != nil {
			return SuccessorUnknown, epoch, fmt.Errorf("this machine's boot id cannot be read: %w", err)
		}
		return SuccessorGone, epoch, nil
	}
	record, _, err := ReadRunRecord(dir, name, epoch)
	if err != nil {
		return SuccessorUnknown, epoch, err
	}
	if here := namespace(); here == "" || record.PIDNamespace != here {
		return SuccessorUnknown, epoch, fmt.Errorf("the successor %s runs in a pid namespace this process cannot judge", epoch)
	}
	switch ObserveRun(epoch) {
	case proc.IdentityEnded:
		return SuccessorGone, epoch, nil
	case proc.IdentityUnknown:
		return SuccessorUnknown, epoch, fmt.Errorf("whether the successor %s runs cannot be read", epoch)
	}
	session, err := Load(dir, name)
	switch {
	case errors.Is(err, ErrNotFound):
		return SuccessorStarting, epoch, nil
	case err != nil:
		return SuccessorUnknown, epoch, err
	case session.Epoch() == epoch:
		return SuccessorReady, epoch, nil
	}
	return SuccessorStarting, epoch, nil
}
