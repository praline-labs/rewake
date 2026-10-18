// Package spacehost runs a space: a worker that main started with rewake
// space open, in a pseudo-terminal of its own, in main's room, living no
// longer than main's run (docs/rules/space.md).
//
// The open re-executes rewake twice. The launcher, in a session of its own,
// starts the space host and exits, so the space host is reparented away from
// main and is no descendant of main's wrapper. The space host records itself,
// shows it is outside main's run, opens the pseudo-terminal, starts an
// ordinary launch of the worker on it, and checks main's run once a second;
// once that run has ended it stops the worker's process group, records what
// it observed and exits. It serves the worker's screen to a host mode's views.
package spacehost

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/praline-labs/rewake/internal/host/terminal"
)

// Env carries the spec from rewake space open to the launcher, and from the
// launcher to the space host. rewake reads it before anything else and runs
// the stage it names; a worker never inherits it.
const Env = "REWAKE_SPACE_HOST"

// The stages a spec names.
const (
	stageLaunch = "launch"
	stageServe  = "serve"
)

// Spec is what the space host needs beyond the record: where the room is,
// what to run and where.
type Spec struct {
	Stage string `json:"stage"`
	// Base, Room and Dir name main's room: its state directory's base, the
	// room, and the room's directory.
	Base string `json:"base"`
	Room string `json:"room"`
	Dir  string `json:"dir"`
	// ID is the space's record.
	ID string `json:"id"`
	// Args are the worker's launch, the words after rewake.
	Args []string `json:"args"`
	// Cwd is where the worker's launch starts: main's working directory.
	Cwd string `json:"cwd"`
	// Strip are the variables no worker inherits beyond rewake's own: every
	// harness's EnvNotInherited, main's markers among them.
	Strip []string `json:"strip,omitempty"`
	// Worktree says the launch makes a worktree, which the record names once
	// the worker's session claims it.
	Worktree bool          `json:"worktree,omitempty"`
	Size     terminal.Size `json:"size"`
	// Launcher is the launcher's pid, which the space host waits to see gone
	// before it shows itself outside main's run.
	Launcher int `json:"launcher,omitempty"`
}

func (s Spec) encode() (string, error) {
	raw, err := json.Marshal(s)
	return string(raw), err
}

func decode(raw string) (Spec, error) {
	var spec Spec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		return Spec{}, fmt.Errorf("%s does not read: %w", Env, err)
	}
	if spec.ID == "" || spec.Dir == "" || spec.Base == "" || spec.Room == "" || len(spec.Args) == 0 {
		return Spec{}, errors.New(Env + " names no space")
	}
	return spec, nil
}

// withSpec is env with the spec as its one value of Env.
func withSpec(env []string, spec Spec) ([]string, error) {
	raw, err := spec.encode()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, Env+"=") {
			out = append(out, kv)
		}
	}
	return append(out, Env+"="+raw), nil
}

// Stage runs the stage the spec in env names and returns the process's exit
// code: the launcher's, which starts the space host, or the space host's own.
// A stage writes nothing to the terminal: it has none, and what it observed
// goes to the record.
func Stage(raw string) int {
	spec, err := decode(raw)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "rewake: "+err.Error())
		return 2
	}
	switch spec.Stage {
	case stageLaunch:
		return launch(spec)
	case stageServe:
		return serve(spec)
	}
	_, _ = fmt.Fprintf(os.Stderr, "rewake: %s names no stage %q\n", Env, spec.Stage)
	return 2
}
