package spacehost

import (
	"errors"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/session"
	"github.com/praline-labs/rewake/internal/core/space"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Panel is what the host offers a main run's adapter of the spaces that run
// opened (docs/v2/workers-panel.md): it reads the records and the workers'
// snapshots as rewake list does, and writes nothing.
type Panel struct {
	// Dir is the room's directory.
	Dir string
	// Main is the main run's epoch, which its spaces' records name.
	Main string
}

// Workers are the spaces Main opened that are not over, in the order they
// opened, each with what its worker's snapshot says.
func (p Panel) Workers() ([]adapter.SpaceWorker, error) {
	records, _, err := space.List(p.Dir)
	if err != nil {
		return nil, err
	}
	workers := []adapter.SpaceWorker{}
	for _, r := range records {
		if r.Main.Run != p.Main || r.State.Final() {
			continue
		}
		name := r.Worker.Name + "-" + r.Worker.Harness
		worker := adapter.SpaceWorker{Space: r.ID, Name: r.Name, Session: name, Harness: r.Worker.Harness, State: string(r.State), Percent: -1}
		if current, err := registry.LookupReadOnly(p.Dir, name); err == nil {
			snapshot := session.Load(p.Dir, name, current.Epoch())
			worker.Activity = text(snapshot.Activity)
			worker.Model = text(snapshot.Model)
			worker.Effort = text(snapshot.Effort)
			if snapshot.FilledPercent != nil {
				worker.Percent = *snapshot.FilledPercent
			}
		}
		workers = append(workers, worker)
	}
	return workers, nil
}

// View connects to the space's screen.
func (p Panel) View(id string) (adapter.ScreenView, error) {
	if !space.ValidID(id) {
		return nil, errors.New("no space " + id)
	}
	return OpenView(state.SpaceSocketPath(p.Dir, id))
}

func text(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
