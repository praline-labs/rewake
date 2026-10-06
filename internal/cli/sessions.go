package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/grant"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/sessionstate"
	"github.com/praline-labs/rewake/internal/state"
)

// listModel is the machine form of the session list.
type listModel struct {
	Directory string        `json:"directory"`
	Room      string        `json:"room"`
	Sessions  []sessionView `json:"sessions"`
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

	visible := canSeeSessionState(dir)
	var views []sessionView
	room := filepath.Base(dir)
	for index := range sessions {
		sessions[index].Room = room
		sessions[index].Role = role.Of(sessions[index].Role).ID
		view := sessionView{Session: sessions[index], Grants: grant.Load(dir, sessions[index].Name, sessions[index].Epoch())}
		if !visible {
			view.MessagingReadyAt = nil
		}
		if visible {
			view.Telemetry = sessionSnapshot(dir, view.Name, view.Epoch())
		}
		views = append(views, view)
	}
	return printValue(ctx, listModel{Directory: state.RootForRoom(dir), Room: room, Sessions: views}, func() []string {
		if len(sessions) == 0 {
			return []string{
				fmt.Sprintf("No sessions are running in room %s.", room),
				"Start one: rewake --name api claude",
			}
		}
		return sessionTable(room, views, visible)
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
	// MailChannel is how this call reached the mail: "tool" through the
	// rewake tool, "shell" otherwise; empty outside a session.
	MailChannel string `json:"mailChannel,omitempty"`
	// Mail is how this run's mail travels, as its wrapper records it
	// (docs/mail-bridge-channel.md#what-is-shown); Channel the record.
	Mail    string          `json:"mail,omitempty"`
	Channel *channel.Record `json:"channel,omitempty"`
	// AssumedGates are the gates this run's launch took as closed, for
	// verification only.
	AssumedGates []string `json:"assumedGates,omitempty"`
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
		model.MailChannel = "shell"
		if ctx.scope != nil {
			model.MailChannel = "tool"
		}
		model.Channel = sessionstate.Load(dir, name, os.Getenv(state.EpochEnv)).Channel
		model.Mail, model.AssumedGates = channel.Label(model.Channel), session.AssumedGates
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
		lines := []string{line, "Others reach you with: rewake send " + name + " \"text\""}
		if ctx.scope != nil {
			lines = append(lines, "This call came through the rewake tool.")
		}
		lines = append(lines, model.Mail)
		if len(model.AssumedGates) > 0 {
			lines = append(lines, assumedLine(model.AssumedGates))
		}
		return lines
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
	return unknownSessionFor(findCommand("send"), dir, name)
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
func assumedLine(gates []string) string {
	return "Gates taken as closed for verification: " + strings.Join(gates, ", ") + "."
}
