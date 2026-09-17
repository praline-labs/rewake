package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

// listModel is the machine form of the session list.
type listModel struct {
	Directory string             `json:"directory"`
	Room      string             `json:"room"`
	Sessions  []registry.Session `json:"sessions"`
}

func handleList(ctx *Context, _ Call) error {
	dir, err := state.Dir()
	if err != nil {
		return &UsageError{Message: err.Error()}
	}
	sessions, err := registry.List(dir)
	if err != nil {
		return failf("could not read the sessions in %s: %v", dir, err)
	}

	room := filepath.Base(dir)
	for index := range sessions {
		sessions[index].Room = room
		sessions[index].Role = role.Of(sessions[index].Role).ID
	}
	return printValue(ctx, listModel{Directory: state.RootForRoom(dir), Room: room, Sessions: sessions}, func() []string {
		if len(sessions) == 0 {
			return []string{
				fmt.Sprintf("No sessions are running in room %s.", room),
				"Start one: rewake --name api claude",
			}
		}
		rows := make([]column, 0, len(sessions))
		for _, session := range sessions {
			text := fmt.Sprintf("%-7s %-8s %s  room=%s  (%s)", session.Harness, age(session.Age()), session.CWD, room, session.Role)
			rows = append(rows, column{Name: session.Name, Text: text})
		}
		return printColumns(rows, "")
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

	name := os.Getenv(state.SessionEnv)
	model := whoamiModel{Name: name, Room: filepath.Base(dir), Directory: state.RootForRoom(dir), Managed: name != ""}
	if name != "" {
		session, _, err := ownRun(dir)
		if err != nil {
			return failf("cannot identify this session in room %s: %v", model.Room, err)
		}
		model.Harness, model.Role = session.Harness, role.Of(session.Role).ID
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

// unknownSessionError refuses a name nobody answers to, and says who does.
func unknownSessionError(dir, name string) error {
	message := fmt.Sprintf("No session named %q is running.", name)
	if names := registry.Names(dir); len(names) > 0 {
		message += " Running now: " + strings.Join(names, ", ") + "."
	} else {
		message += " No sessions are running. Run rewake for launch commands, then use the exact address from rewake list."
	}
	return &UsageError{Command: findCommand("send"), Message: message}
}
