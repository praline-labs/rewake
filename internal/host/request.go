package host

import (
	"context"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
	"github.com/praline-labs/rewake/internal/infra/build"
)

// Request is one launch.
type Request struct {
	// Harness is what to run.
	Harness adapter.Harness
	// Dir is the room-scoped state directory.
	Dir string
	// Name is the requested prefix; empty uses the role and picks a free address.
	Name string
	// Args are the caller's arguments for the harness.
	Args []string
	// Intro asks for the briefing that tells the agent it runs under rewake.
	Intro bool
	// Command is the program to start in place of the harness's own.
	Command string
	// Shell says the harness runs in rewake's shell, on the shell's
	// terminal (docs/shell.md): an ordinary run here, drawn by the shell,
	// its stops continued at once.
	Shell bool

	// Role is explicit when its ID is set; empty always uses general.
	Role role.Role
	// Template is the alias the worker's space was opened with
	// (adapter.LaunchRequest.Template).
	Template string
	// WorktreeEntry says that template lets the worker enter a checkout
	// (adapter.LaunchRequest.WorktreeEntry).
	WorktreeEntry bool
	// Mirror asks for rewake's own face (adapter.LaunchRequest.Mirror).
	Mirror bool
	// Mode is the runtime work mode the session runs in
	// (adapter.LaunchRequest.Mode), its text included.
	Mode  workmode.Mode
	Modes workmode.Config
	// Migrations are the obsolete worker settings the launch's catalog
	// skipped rather than refused (adapter.LaunchRequest.Migrations): the
	// briefing carries them so main can repair what it starts under.
	Migrations []string
	// RoleReason says why the launch has its role, for the record and the
	// briefing; empty says the role was selected with its flag.
	RoleReason string
	// OnTurn publishes a turn's end the run's TurnBoundary reported, and
	// answers what the published end said.
	OnTurn func(context.Context, registry.Session, adapter.TurnEnd) (mail.Outcome, error)
	// OnConfirm runs an end the adapter may hold open, beside OnTurn
	// (adapter.TurnHandler.Confirm); a held one answers its reason in Held.
	OnConfirm func(context.Context, registry.Session, adapter.TurnEnd) (mail.Outcome, error)
	// OnClaimed hears the session once its name is claimed, before the
	// harness is planned: whatever was prepared for the launch learns whose
	// it is. An error ends the launch.
	OnClaimed func(registry.Session) error
	// Lease is the room's build lease the caller took for this launch and
	// keeps until Run returns; nil has Run take its own. Run adds a hold of
	// its own either way, which lasts until the run's workers have stopped.
	Lease *build.Lease
	// Handover is the run an earlier image of this wrapper handed over
	// (reexec.go): Run adopts it instead of claiming and starting.
	Handover *Handover
}
