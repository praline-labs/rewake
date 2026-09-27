package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/iiiokojiadbi/rewake/internal/alias"
	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/role"
	"github.com/iiiokojiadbi/rewake/internal/state"
	"github.com/iiiokojiadbi/rewake/internal/wrap"
)

// handleLaunch starts one harness as a rewake session. It returns only when the
// harness exits, and with its exit code: rewake sits in the middle and should
// not hide what the program said.
func handleLaunch(h harness.Harness) func(*Context, Call) error {
	return func(ctx *Context, call Call) error {
		if err := refuseNestedLaunch(call); err != nil {
			return err
		}
		if prefix, present := call.Flags["name"]; present && prefix == "" {
			return &UsageError{Command: call.Command, Message: "--name needs a nonempty prefix; omit --name to use the selected role."}
		}
		if refuser, ok := h.(harness.LaunchRefuser); ok {
			if err := refuser.RefuseLaunch(call.Raw); err != nil {
				return &UsageError{Command: call.Command, Message: err.Error() + "."}
			}
		}
		program, err := launchProgram(call)
		if err != nil {
			return err
		}
		root, err := state.Root()
		if err != nil {
			// An unusable state directory is something about this call and its
			// environment, not about a target that refused: code 2, so a caller
			// branching on the code tries to fix the call.
			return &UsageError{Message: err.Error()}
		}

		dir, err := state.RoomDir(root, call.Flag("room", state.DefaultRoom))
		if err != nil {
			return &UsageError{Command: call.Command, Message: err.Error()}
		}

		part, err := chosenRole(call)
		if err != nil {
			return err
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

		code, err := wrap.Run(context.Background(), wrap.Request{
			OnTurn: func(ctx context.Context, self registry.Session, result harness.Completion) error {
				return ReportCompletion(ctx, dir, self, result)
			},
			Harness: h,
			Dir:     dir,
			Name:    call.Flag("name", ""),
			Args:    args,
			Intro:   !call.Switch("no-intro"),
			Role:    part,
			Command: program,

			OnClaimed: onClaimed,
		})
		if checkout != nil {
			checkout.settle(err != nil || code != 0)
		}
		if err != nil {
			var mainTaken *wrap.MainTakenError
			if errors.As(err, &mainTaken) {
				return &UsageError{Command: call.Command, Message: mainTaken.Error()}
			}
			var taken *registry.NameTakenError
			if errors.As(err, &taken) {
				return &UsageError{Command: call.Command, Message: taken.Error() + " Pick another prefix with --name, or omit --name to get the next free role-based address."}
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
// with none of the worker's limits — rewake claude
// --dangerously-skip-permissions, say. The environment alone decides: a live
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
		"this shell runs inside %s (%s or %s is set), and a session does not start other sessions. Start %s from a shell outside any rewake session; a way for main to start a worker, when there is one, will be its own command.",
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
	program, given := call.Flags[alias.CommandFlag]
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
		// Fixed against the launch directory now. Codex starts its owned
		// server in the directory given with -C, and a relative path would
		// resolve there — to another file of the same name, or to none.
		absolute, err := filepath.Abs(program)
		if err != nil {
			return "", &UsageError{Command: call.Command, Message: fmt.Sprintf("--command %s: %v", program, err)}
		}
		program = absolute
	}
	return program, nil
}

// chosenRole leaves an omitted role unset so the wrapper can distinguish
// default general from an explicit --general in its recorded reason.
func chosenRole(call Call) (role.Role, error) {
	chosen := role.Role{}
	var flags []string
	for _, candidate := range role.All() {
		if call.Switch(candidate.ID) {
			chosen = candidate
			flags = append(flags, "--"+candidate.ID)
		}
	}
	if len(flags) > 1 {
		return role.Role{}, &UsageError{
			Command: call.Command,
			Message: strings.Join(flags, " and ") + " exclude each other: a session has one role.",
		}
	}
	return chosen, nil
}

// roleOptions are the launch flags that choose a role, for the command table.
func roleOptions() []Option {
	var options []Option
	for _, candidate := range role.All() {
		options = append(options, Option{Flag: "--" + candidate.ID, Summary: candidate.Summary})
	}
	return options
}

// roleFlags are the flag names that choose a role. They are what a file
// carried by a directory may not set: the role decides what a session sees and
// what it may ask for, which is more than a working directory should decide
// for whoever launches there.
func roleFlags() []string {
	var flags []string
	for _, candidate := range role.All() {
		flags = append(flags, candidate.ID)
	}
	return flags
}

// projectForbidden are the rewake flags an alias in a project file may not
// set: the role flags, and the program a launch starts.
func projectForbidden() []string {
	return append(roleFlags(), alias.CommandFlag)
}

// singleUseFlags asks a harness which of its flags it takes at most once. The
// answer belongs to the harness — Codex refuses a repeated --model while
// accepting a repeated --add-dir — so this only looks it up.
func singleUseFlags(id string) []harness.Flag {
	for _, candidate := range harness.All() {
		if candidate.ID() == id {
			return candidate.SingleUseFlags()
		}
	}
	return nil
}

// roleSummary keeps the guide on the same catalog as launch flags and help.
func roleSummary() string {
	var descriptions []string
	for _, candidate := range role.All() {
		label := "--" + candidate.ID
		descriptions = append(descriptions, label+": "+candidate.Summary)
	}
	return "Without a role flag, every launch uses general, even in an empty room. Only --main creates main, and it refuses if main is occupied. A name prefix never selects a role. " + strings.Join(descriptions, " ") + " A silent coordinator prevents reports from waking each other indefinitely."
}
