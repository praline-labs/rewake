package registry

import (
	"errors"
	"slices"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// HasLive says whether the run has proven a capability. Every decision another
// session makes about this one — may a task be sent to it, may it be
// compacted — asks this, never what its harness offers
// (docs/v2/design-api.md#two-levels-offered-and-live).
func (s Session) HasLive(capability string) bool { return slices.Contains(s.Live, capability) }

// Modify changes the record of a session that still owns its name, under the
// name's lock, on the record as it stands then: two writers changing different
// fields — the live set as the run proves or loses a capability, the harness's
// process once it starts — never undo each other, as two whole records
// written from copies read earlier would.
func Modify(dir, name, epoch string, change func(*Session)) error {
	if !state.ValidName(name) {
		return ErrUnusableName
	}
	return state.WithNameLock(dir, name, func() error {
		existing, err := Load(dir, name)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return ErrNotFound
			}
			return err
		}
		if existing.Epoch() != epoch {
			return ErrNotFound
		}
		change(&existing)
		encoded, err := encode(existing)
		if err != nil {
			return err
		}
		return state.WriteAtomic(state.SessionPath(dir, name), encoded)
	})
}
