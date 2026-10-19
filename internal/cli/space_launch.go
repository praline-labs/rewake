package cli

import (
	"fmt"
	"slices"

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
	// template is the alias the worker was raised from.
	template string
}

func (ctx *Context) spaceTarget(call Call) (workerTarget, error) {
	refuse := func(format string, args ...any) (workerTarget, error) {
		return workerTarget{}, &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	if checkout, given := call.Flags["worktree"]; given && checkout == "" {
		return refuse("--worktree= needs a name; use --worktree for a generated name.")
	}
	// A worker is raised only from a template (the owner's decision,
	// October 19, 2026): the alias names its harness and its role.
	alias, byAlias := call.Flags["alias"]
	if !byAlias || alias == "" {
		return refuse("space open needs --alias <alias>; rewake workers lists them.")
	}
	if err := ctx.launchAliases().Err(); err != nil {
		return refuse("%v", err)
	}
	template, err := ctx.launchAliases().Template(alias, ctx.table.catalog)
	if err != nil {
		return refuse("%v", err)
	}
	return workerTarget{
		harness: template.Harness, part: template.Role, command: template.Command, template: alias,
		args: launch.MergeArguments(template.Args, slices.Clone(call.Positionals[2:]), template.Harness.SingleUseFlags()),
	}, nil
}

// workerLaunch builds the complete ordinary launch, including settings and
// the fresh conversation id, before the detached host receives it. The space
// owns name, role and room, which its record carries.
func workerLaunch(target workerTarget, call Call, conversation string) ([]string, error) {
	words, err := launch.WorkerArgs(target.harness, launch.Worker{
		Command: target.command, NoIntro: target.noIntro, Args: target.args,
		Worktree: call.Flag("worktree", ""), Conversation: conversation,
	})
	if err != nil {
		return nil, &UsageError{Command: call.Command, Message: err.Error()}
	}
	return words, nil
}
