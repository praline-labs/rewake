package wrap

import (
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

// MainTakenError refuses an explicit main while another live session holds it.
type MainTakenError struct{ Room, Name string }

func (e *MainTakenError) Error() string {
	return fmt.Sprintf("room %q already has live main session %q; stop or restart that session, or launch without --main to join as general", e.Room, e.Name)
}

// claimName resolves a role and publishes while holding one room lock. A pending
// launch is already alive through its wrapper pid, so a second launch cannot
// also claim explicit main while the first prepares its harness.
//
// Publishing is the fourth step of the launch order
// (docs/protocol-cutover.md#the-launch): writers answers whether an
// earlier-build writer of the chosen name is not proven stopped; then the run
// record, then the binding as the name's successor, each before the session
// record, so that a sender's barrier never finds a successor without its run
// record, nor a ready run it could not tell the build of.
//
// Everything before that proof only reads: the listing that finds main, the
// choice of the name, the look whether a live run holds it. A pruning read
// would remove the dead record of the name — one of the earlier build names
// no pid namespace, and the proof refuses on it as out of sight — and the
// proof would pass on evidence the launch itself erased.
func claimName(request Request, self int, selfStart uint64, boot, cwd string, writers func(dir, name string) error) (registry.Session, error) {
	var session registry.Session
	err := state.WithRoomLock(request.Dir, func() error {
		sessions, err := registry.ListReadOnly(request.Dir)
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
			chosen = role.General
			reason = "default general because no role flag was supplied"
		} else if chosen.ID == role.Main.ID && main != "" {
			return &MainTakenError{Room: filepath.Base(request.Dir), Name: main}
		}
		base, err := launchName(request.Name, chosen.ID, request.Harness.ID())
		if err != nil {
			return err
		}
		explicit := ""
		if request.Name != "" {
			explicit = base
		}
		for attempt := 0; attempt < 16; attempt++ {
			name, err := registry.ChooseName(request.Dir, explicit, base)
			if err != nil {
				return err
			}
			session = registry.Session{
				Name: name, Room: filepath.Base(request.Dir), Harness: request.Harness.ID(),
				ServicePID: self, ServiceStart: selfStart, PIDNamespace: proc.Namespace(),
				Boot: boot, Build: registry.BuildStamp,
				Role: chosen.ID, RoleReason: reason, CWD: cwd, StartedAt: time.Now(),
			}
			if err := prepareRun(request.Dir, session, writers); err != nil {
				return err
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

// prepareRun is the launch order's first three steps for a name no live run
// holds. A name some live run holds is left to Publish, which refuses it:
// bound as the successor, a launch that never ran would make every report
// held for the name moot once it exits.
func prepareRun(dir string, session registry.Session, writers func(dir, name string) error) error {
	if err := writers(dir, session.Name); err != nil {
		return err
	}
	if _, err := registry.LookupReadOnly(dir, session.Name); err == nil {
		return nil
	}
	record := registry.RunRecord{
		Name: session.Name, Boot: session.Boot, Epoch: session.Epoch(), Build: registry.BuildStamp,
		Started: boottime.ProcessStarted, PIDNamespace: session.PIDNamespace,
	}
	if err := registry.WriteRunRecord(dir, record); err != nil {
		return fmt.Errorf("could not record this run of %s: %w", session.Name, err)
	}
	if _, err := registry.BindSuccessor(dir, session.Name, session.Epoch()); err != nil {
		return fmt.Errorf("could not bind this run as the successor of %s: %w", session.Name, err)
	}
	return nil
}
