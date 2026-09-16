package wrap

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/proc"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// MainTakenError refuses an explicit main while another live session holds it.
type MainTakenError struct{ Room, Name string }

func (e *MainTakenError) Error() string {
	return fmt.Sprintf("room %q already has live main session %q; stop or restart that session, or launch without --main to join as a worker", e.Room, e.Name)
}

// claimName elects a role and publishes while holding one room lock. A pending
// launch is already alive through its wrapper pid, so a second launch cannot
// also elect itself main while the first prepares its harness.
func claimName(request Request, self int, selfStart uint64, cwd string) (registry.Session, error) {
	var session registry.Session
	err := state.WithRoomLock(request.Dir, func() error {
		sessions, err := registry.List(request.Dir)
		if err != nil {
			return err
		}
		main := ""
		for _, candidate := range sessions {
			if candidate.Role == role.Main.ID {
				main = candidate.Name
				break
			}
		}
		chosen := request.Role
		reason := "selected explicitly with --" + chosen.ID
		if chosen.ID == "" {
			chosen = role.Main
			reason = "selected automatically because this room has no live main session"
			if main != "" {
				chosen = role.Worker
				reason = fmt.Sprintf("selected automatically because %q is this room's live main", main)
			}
		} else if chosen.ID == role.Main.ID && main != "" {
			return &MainTakenError{Room: filepath.Base(request.Dir), Name: main}
		}
		for attempt := 0; attempt < 16; attempt++ {
			name, err := registry.ChooseName(request.Dir, request.Name, request.Harness.ID())
			if err != nil {
				return err
			}
			session = registry.Session{
				Name: name, Room: filepath.Base(request.Dir), Harness: request.Harness.ID(),
				ServicePID: self, ServiceStart: selfStart, PIDNamespace: proc.Namespace(),
				Role: chosen.ID, RoleReason: reason, CWD: cwd, StartedAt: time.Now(),
			}
			err = registry.Publish(request.Dir, session)
			if err == nil {
				return nil
			}
			var taken *registry.NameTakenError
			if request.Name == "" && errors.As(err, &taken) {
				continue
			}
			return err
		}
		return fmt.Errorf("could not claim a name for this session: every candidate was taken while starting")
	})
	return session, err
}
