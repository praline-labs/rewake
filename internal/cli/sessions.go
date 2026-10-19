package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// listModel is the machine form of the session list.
type listModel struct {
	Directory string `json:"directory"`
	Room      string `json:"room"`
	// RoomDirectory is the launch directory the room is named for; empty
	// for a room named otherwise.
	RoomDirectory string        `json:"roomDirectory,omitempty"`
	Sessions      []sessionView `json:"sessions"`
	// Unknown are the sessions whose record cannot be read: whether they
	// run is unknown, so they are shown rather than left out.
	Unknown []unknownSessionView `json:"unknown,omitempty"`
}

// unknownSessionView is a session whose record cannot be read.
type unknownSessionView struct {
	Name  string `json:"name"`
	State string `json:"state"`
	Error string `json:"error"`
}

func handleList(ctx *Context, _ Call) error {
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Message: err.Error()}
	}
	// A view: a dead record is left for a change form to prune.
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return failf("could not read the sessions in %s: %v", dir, err)
	}
	// Read-only: a record that does not read is never pruned on a guess.
	unreadable, _ := registry.UnreadableRecords(dir)
	var unknown []unknownSessionView
	for _, record := range unreadable {
		unknown = append(unknown, unknownSessionView{Name: record.Name, State: "unknown", Error: record.Err.Error()})
	}

	visible := canSeeSessionState(dir)
	var views []sessionView
	room := filepath.Base(dir)
	for index := range sessions {
		sessions[index].Room = room
		sessions[index].Role = role.Of(sessions[index].Role).ID
		view := sessionView{Session: sessions[index]}
		if !visible {
			view.MessagingReadyAt = nil
		}
		if visible {
			view.Telemetry = sessionSnapshot(dir, view.Name, view.Epoch())
		}
		views = append(views, view)
	}
	base, err := state.BaseForRoom(dir)
	if err != nil {
		return failf("%v", err)
	}
	model := listModel{Directory: base, Room: room, RoomDirectory: state.RoomDirectory(dir), Sessions: views, Unknown: unknown}
	label := room
	if model.RoomDirectory != "" {
		label += " (" + model.RoomDirectory + ")"
	}
	return printValue(ctx, model, func() []string {
		var lines []string
		switch {
		case len(sessions) > 0:
			lines = sessionTable(label, views, visible)
		case len(unknown) > 0:
			lines = []string{"Room: " + label}
		default:
			return []string{
				fmt.Sprintf("No sessions are running in room %s.", label),
				"Start main in this room's directory: rewake " + launchExample(ctx.table.catalog) + "; main raises the workers.",
			}
		}
		for _, session := range unknown {
			lines = append(lines, fmt.Sprintf("%s  state unknown: %s", session.Name, session.Error))
		}
		return lines
	})
}

// whoamiModel is the machine form of the session's own identity.
type whoamiModel struct {
	Name      string `json:"name,omitempty"`
	Room      string `json:"room"`
	Role      string `json:"role,omitempty"`
	Harness   string `json:"harness,omitempty"`
	Directory string `json:"directory"`
	Managed   bool   `json:"managed"`
}

func handleWhoami(ctx *Context, _ Call) error {
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Message: err.Error()}
	}

	base, err := state.BaseForRoom(dir)
	if err != nil {
		return failf("%v", err)
	}
	name := os.Getenv(state.SessionEnv)
	model := whoamiModel{Name: name, Room: filepath.Base(dir), Directory: base, Managed: name != ""}
	if name != "" {
		own, _, err := ownRunIn(ctx.access, dir)
		if err != nil {
			return failf("cannot identify this session in room %s: %v", model.Room, err)
		}
		model.Harness, model.Role = own.Harness, role.Of(own.Role).ID
	}

	return printValue(ctx, model, func() []string {
		if name == "" {
			return []string{
				"This shell is not part of a rewake session. Room: " + model.Room + ".",
				"Others cannot address it; start an agent with rewake to give it a name.",
			}
		}
		line := name + "  room=" + model.Room + "  (" + model.Role + ")"
		if model.Harness != "" {
			line += "  " + model.Harness
		}
		return []string{line, "Others reach you with: rewake send " + name + " \"text\""}
	})
}

// age renders a duration the way the list shows it: short and rounded, because
// the exact second of a session that has run for hours is noise.
func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// unknownSessionFor refuses a name no running session answers to, with the
// syntax of the command that was called.
func unknownSessionFor(command *Command, dir, name string) error {
	message := fmt.Sprintf("No session named %q is running.", name)
	if names := registry.Names(dir); len(names) > 0 {
		message += " Running now: " + strings.Join(names, ", ") + "."
	} else {
		message += " No sessions are running. Run rewake for launch commands, then use the exact address from rewake list."
	}
	return &UsageError{Command: command, Message: message}
}

// assumedLine says which gates a run's launch took as closed: such a run
// rests on an assumption the live checks are there to test.
