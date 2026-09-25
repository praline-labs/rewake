package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// Pending is a compaction whose command answered started or requested and
// whose letter has not gone yet. The command leaves it in the asking main's
// state; main's own wrapper closes it exactly once — with the outcome from the
// worker's snapshot, with the worker's departure, or at a bound — so the
// answer's promise of a letter holds even when the worker's side never records
// an outcome (docs/remote-control.md). The worker is kept whole: whether its
// run is gone is judged from the processes its record names.
type Pending struct {
	ID         string           `json:"id"`
	Worker     registry.Session `json:"worker"`
	AskerEpoch string           `json:"askerEpoch"`
	AskedAt    time.Time        `json:"askedAt"`
}

// Remember leaves a pending compaction for asker's wrapper, held until
// release is called: the command holds it while it asks, since its answer
// decides whether the record stays, and the wrapper leaves a held record
// alone.
func Remember(dir, asker string, pending Pending) (release func(), err error) {
	if !ValidID.MatchString(pending.ID) || !state.ValidName(asker) {
		return nil, fmt.Errorf("invalid pending compaction")
	}
	path := state.LettersPath(dir, asker)
	if err := state.EnsureSubdir(path); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(pending)
	if err != nil {
		return nil, err
	}
	held, err := state.WriteAtomicHeld(filepath.Join(path, pending.ID+".json"), encoded)
	if err != nil {
		return nil, err
	}
	return func() { _ = held.Close() }, nil
}

// Hold takes a pending compaction for closing: false while the command that
// left it still holds it, still asking, or when it is gone. The command's
// hold dies with its process, so a command killed outright leaves a record
// its wrapper can close. The record is read again under the hold, since the
// one read before it may have been closed meanwhile.
func Hold(dir, asker, id string) (pending Pending, release func(), ok bool) {
	if !ValidID.MatchString(id) || !state.ValidName(asker) {
		return Pending{}, nil, false
	}
	held, ok := state.TryHold(filepath.Join(state.LettersPath(dir, asker), id+".json"))
	if !ok {
		return Pending{}, nil, false
	}
	raw, err := io.ReadAll(held)
	if pending, ok = parsePending(raw, id); err != nil || !ok {
		_ = held.Close()
		return Pending{}, nil, false
	}
	return pending, func() { _ = held.Close() }, true
}

// PendingOf lists the compactions pending for asker.
func PendingOf(dir, asker string) []Pending {
	if !state.ValidName(asker) {
		return nil
	}
	path := state.LettersPath(dir, asker)
	if state.Verify(path) != nil {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	var found []Pending
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !ValidID.MatchString(id) {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(path, entry.Name()))
		if pending, ok := parsePending(raw, id); err == nil && ok {
			found = append(found, pending)
		}
	}
	return found
}

// parsePending reads the record of request id. One that does not parse, or
// names another request than its own, is none: it was written by nothing of
// rewake's.
func parsePending(raw []byte, id string) (Pending, bool) {
	var pending Pending
	if json.Unmarshal(raw, &pending) != nil || pending.ID != id || !state.ValidName(pending.Worker.Name) {
		return Pending{}, false
	}
	return pending, true
}

// Forget closes a pending compaction once its letter has gone.
func Forget(dir, asker, id string) error {
	if !ValidID.MatchString(id) || !state.ValidName(asker) {
		return fmt.Errorf("invalid pending compaction")
	}
	err := os.Remove(filepath.Join(state.LettersPath(dir, asker), id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
