package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/launch"
)

type workerTarget struct {
	harness adapter.Harness
	part    role.Role
	args    []string
	command string
	noIntro bool
}

func (ctx *Context) spaceTarget(call Call) (workerTarget, error) {
	refuse := func(format string, args ...any) (workerTarget, error) {
		return workerTarget{}, &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	if checkout, given := call.Flags["worktree"]; given && checkout == "" {
		return refuse("--worktree= needs a name; use --worktree for a generated name.")
	}
	_, byHarness := call.Flags["harness"]
	alias, byAlias := call.Flags["alias"]
	if byHarness == byAlias {
		return refuse("space open needs exactly one of --harness or --alias; use rewake workers to choose an alias.")
	}
	target := workerTarget{part: role.Write, args: slices.Clone(call.Positionals[2:])}
	if byAlias {
		if err := ctx.launchAliases().Err(); err != nil {
			return refuse("%v", err)
		}
		view, expanded, err := ctx.workerAlias(alias)
		if err != nil {
			return refuse("%v", err)
		}
		if view.Main {
			return refuse("alias %q is main's, not a worker; choose one from rewake workers.", alias)
		}
		target.harness = expanded.Command.Harness
		target.part, _ = role.Find(view.Role)
		target.command = expanded.Flag(launch.CommandFlag, "")
		target.noIntro = expanded.Switch("no-intro")
		target.args = launch.MergeArguments(expanded.Raw, target.args, target.harness.SingleUseFlags())
	} else {
		var found bool
		target.harness, found = ctx.table.catalog.Find(call.Flag("harness", ""))
		if !found {
			return refuse("space open needs --harness, one of: %s.", strings.Join(ctx.table.catalog.IDs(), ", "))
		}
	}
	if selected, given := call.Flags["role"]; given {
		var found bool
		target.part, found = role.Find(selected)
		if !found || target.part.Spaces {
			return refuse("--role takes a worker's role, not %q: %s or %s.", selected, role.Write.ID, role.General.ID)
		}
	}
	return target, nil
}

// workerLaunch builds the complete ordinary launch, including settings and
// the fresh conversation id, before the detached host receives it. The space
// owns name and room; an alias's copies cannot move its worker elsewhere.
func workerLaunch(target workerTarget, call Call, name, room, conversation string) ([]string, error) {
	words, err := launch.WorkerArgs(target.harness, launch.Worker{
		Name: name, Room: room, Role: target.part.ID,
		Command: target.command, NoIntro: target.noIntro, Args: target.args,
		Worktree: call.Flag("worktree", ""), Conversation: conversation,
	})
	if err != nil {
		return nil, &UsageError{Command: call.Command, Message: err.Error()}
	}
	return words, nil
}
