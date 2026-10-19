package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host"
	"github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// handleLaunch starts one harness as a rewake session. It returns only when the
// harness exits, and with its exit code: rewake sits in the middle and should
// not hide what the program said.
func handleLaunch(h adapter.Harness) func(*Context, Call) error {
	return func(ctx *Context, call Call) error {
		if err := refuseNestedLaunch(call); err != nil {
			return err
		}
		program, err := launchProgram(call)
		if err != nil {
			return err
		}
		base, err := state.Base()
		if err != nil {
			// An unusable state directory is something about this call and its
			// environment, not about a target that refused: code 2, so a caller
			// branching on the code tries to fix the call.
			return &UsageError{Message: err.Error()}
		}
		id, err := identifyLaunch(call)
		if err != nil {
			return err
		}
		if err := refuseReportingRole(h, id.part); err != nil {
			return err
		}
		inShell, err := enterShell(h, id)
		if err != nil {
			return err
		}
		// The build lease before anything of the room is made or locked, and
		// before the worktree; held until the wrapper and its cleanup are done
		// (docs/v2/stage3-state.md#one-build-per-room).
		lease, err := leaseError(call.Command, func() (*build.Lease, error) { return build.Launch(base, id.room) })
		if err != nil {
			return err
		}
		defer lease.Release()
		root, err := state.Root()
		if err != nil {
			return &UsageError{Message: err.Error()}
		}
		dir, err := state.RoomDir(root, id.room)
		if err != nil {
			return &UsageError{Command: call.Command, Message: err.Error()}
		}
		if id.directory != "" {
			if err := state.RecordRoomDirectory(dir, id.directory); err != nil {
				return &FailedError{Message: "the room's directory could not be recorded: " + err.Error()}
			}
		}
		// Last of the preparations: every refusal above leaves no checkout
		// behind.
		args, checkout, err := takeWorktree(h, call, ctx.Stderr)
		if err != nil {
			return err
		}
		var onClaimed func(registry.Session) error
		if checkout != nil {
			onClaimed = checkout.claimed(dir)
		}

		code, err := host.Run(context.Background(), host.Request{
			OnTurn: func(ctx context.Context, self registry.Session, result adapter.TurnEnd) error {
				return ReportCompletion(ctx, dir, self, result)
			},
			OnConfirm: func(ctx context.Context, self registry.Session, result adapter.TurnEnd) (string, error) {
				return ConfirmCompletion(ctx, dir, self, result)
			},
			Harness:    h,
			Dir:        dir,
			Name:       id.name,
			Args:       args,
			Intro:      !call.Switch("no-intro"),
			Shell:      inShell,
			Role:       id.part,
			RoleReason: id.reason,
			Template:   id.template,
			Command:    program,
			OnClaimed:  onClaimed,
			Lease:      lease,
		})
		if checkout != nil {
			checkout.settle(err != nil || code != 0)
		}
		if err != nil {
			var taken *host.MainTakenError
			if errors.As(err, &taken) {
				return mainTaken(id, taken)
			}
			var nameTaken *registry.NameTakenError
			if errors.As(err, &nameTaken) {
				return &UsageError{Command: call.Command, Message: nameTaken.Error()}
			}
			if errors.Is(err, registry.ErrUnusableName) {
				return &UsageError{Command: call.Command, Message: err.Error()}
			}
			return &FailedError{Message: err.Error()}
		}
		if code != 0 {
			return &ExitCodeError{Code: code}
		}
		return nil
	}
}

// refuseNestedLaunch refuses a launch from a shell inside a rewake session
// (docs/launch.md#no-session-inside-a-session). A worker's rewake commands may
// run without asking the person, and a launch among them would start an agent
// with none of the worker's limits — a harness told to skip its own
// permission prompts, say. The environment alone decides: a live
// record is not required, since a worker could remove or rewrite its own, and
// refusing a shell whose session has ended costs a new shell. A variable set
// to nothing counts too: emptying it is as deliberate as removing it, and
// only removing it is the documented way out.
func refuseNestedLaunch(call Call) error {
	name, named := os.LookupEnv(state.SessionEnv)
	_, run := os.LookupEnv(state.EpochEnv)
	if !named && !run {
		return nil
	}
	inside := "a rewake session"
	if name != "" {
		inside = "rewake session " + name
	}
	return &UsageError{Command: call.Command, Message: fmt.Sprintf(
		"this shell runs inside %s (%s or %s is set), and a session does not start other sessions. Start %s from a shell outside any rewake session; main starts a worker with rewake space open.",
		inside, state.SessionEnv, state.EpochEnv, call.Command.Name)}
}

// launchProgram checks the program named with --command before anything is
// launched: a name must be found on PATH, a path with a slash is taken as
// given, and either must be an executable file. A wrapper that is not there
// would otherwise surface as a harness that "is not installed", after the name
// was already claimed. rewake does not run it to see whether it is the harness
// it claims to be: the harness word says that, and a wrapper need not answer
// --version.
func launchProgram(call Call) (string, error) {
	program, given := call.Flags[launch.CommandFlag]
	if !given {
		return "", nil
	}
	if program == "" {
		return "", &UsageError{Command: call.Command, Message: "--command needs a program; omit it to start " + call.Command.Name + " itself."}
	}
	if _, err := exec.LookPath(program); err != nil {
		where := "on PATH"
		if strings.Contains(program, "/") {
			where = "at that path"
		}
		return "", &UsageError{Command: call.Command, Message: fmt.Sprintf(
			"--command %s: no executable file %s (%v). Name a program on PATH, give a path to an executable file, or omit --command to start %s itself.",
			program, where, err, call.Command.Name)}
	}
	if strings.Contains(program, "/") {
		// Fixed against the launch directory now. A harness may start a part
		// of itself in another directory, and a relative path would resolve
		// there — to another file of the same name, or to none.
		absolute, err := filepath.Abs(program)
		if err != nil {
			return "", &UsageError{Command: call.Command, Message: fmt.Sprintf("--command %s: %v", program, err)}
		}
		program = absolute
	}
	return program, nil
}

// roleSummary keeps the guide on the same catalog as the briefings.
func roleSummary() string {
	var descriptions []string
	for _, candidate := range role.All() {
		descriptions = append(descriptions, candidate.ID+": "+candidate.Summary)
	}
	return "A person starts only main: rewake <harness> is main in the room of its directory, and a second main there is refused. Workers are raised by main alone, with rewake space open --alias <alias>, from a template of rewake workers. " + strings.Join(descriptions, " ")
}
