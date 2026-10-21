package cli

import (
	"fmt"
	"slices"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/worktype"
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
	// worktree is the worktree the worker starts in: "" for none, "true"
	// for one of a generated name, else its name. Resolved once, here, for
	// both the worker's launch and the space host (spaceWorktree).
	worktree string
	// enter says the worker may enter a checkout mid-session: its template
	// says enter_worktree = true and it starts without one.
	enter bool
	// work is the kind of work the worker is raised for, empty Name for none.
	work worktype.Type
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
	checkout, aliasArgs, typedArgs, err := spaceWorktree(call, template)
	if err != nil {
		return workerTarget{}, err
	}
	work, err := ctx.spaceType(call, template)
	if err != nil {
		return workerTarget{}, err
	}
	return workerTarget{
		harness: template.Harness, part: template.Role, command: template.Command, template: alias, worktree: checkout,
		enter: template.EnterWorktree && checkout == "", work: work,
		args: launch.MergeArguments(aliasArgs, typedArgs, template.Harness.SingleUseFlags()),
	}, nil
}

// spaceType resolves the worker's kind of work: a typed --type, else none
// for --no-type, else the template's type, else none. One that is not there,
// or is turned off, is refused here, before anything starts.
func (ctx *Context) spaceType(call Call, template launch.Template) (worktype.Type, error) {
	refuse := func(format string, args ...any) (worktype.Type, error) {
		return worktype.Type{}, &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	typed, given := call.Flags["type"]
	no := call.Switch("no-type")
	switch {
	case given && no:
		return refuse("--type and --no-type ask opposite things; give one.")
	case given && typed == "":
		return refuse("--type needs a name; rewake workers lists the types.")
	case no:
		return worktype.Type{}, nil
	case given:
		value, err := ctx.launchAliases().Type(typed)
		if err != nil {
			return refuse("%v", err)
		}
		return value, nil
	case template.Type != "":
		value, err := ctx.launchAliases().Type(template.Type)
		if err != nil {
			return refuse("alias %q names type = %q: %v; give --type or --no-type", template.Name, template.Type, err)
		}
		return value, nil
	}
	return worktype.Type{}, nil
}

// spaceWorktree resolves the worker's worktree from what asks for one: a
// typed --worktree or --worktree=<name> on space open, or the harness's flag
// typed after --, wins; else --no-worktree leaves the template's out; else the
// template's worktree = true, or the flag in its args, stands. Two typed
// requests, or a typed one beside --no-worktree, are refused: a worker has one
// worktree. The flag is taken out of both argument lists, so the launch
// carries the one answer the space host also gets.
func spaceWorktree(call Call, template launch.Template) (string, []string, []string, error) {
	aliasArgs, typedArgs := slices.Clone(template.Args), slices.Clone(call.Positionals[2:])
	refuse := func(format string, args ...any) (string, []string, []string, error) {
		return "", nil, nil, &UsageError{Command: call.Command, Message: fmt.Sprintf(format, args...)}
	}
	open, typed := call.Flags["worktree"]
	no := call.Switch("no-worktree")
	taker, ok := template.Harness.(adapter.WorktreeHarness)
	if !ok {
		if typed {
			return refuse("%s makes no worktree through rewake; open the space without --worktree.", template.Harness.Title())
		}
		return "", aliasArgs, typedArgs, nil
	}
	flag := taker.WorktreeFlag()
	fromAlias, err := takeWorktreeFlag(call, &aliasArgs, flag, taker.WorktreeNameSpaced())
	if err != nil {
		return refuse("alias %q: %v", template.Name, err)
	}
	afterDashes, err := takeWorktreeFlag(call, &typedArgs, flag, taker.WorktreeNameSpaced())
	if err != nil {
		return "", nil, nil, err
	}
	switch {
	case typed && afterDashes != "":
		return refuse("--worktree is given both to space open and after --; a worker has one worktree, so give it once.")
	case no && (typed || afterDashes != ""):
		return refuse("--no-worktree and --worktree ask opposite things; give one.")
	case afterDashes != "":
		return afterDashes, aliasArgs, typedArgs, nil
	case typed:
		return open, aliasArgs, typedArgs, nil
	case no:
		return "", aliasArgs, typedArgs, nil
	case template.Worktree:
		return "true", aliasArgs, typedArgs, nil
	}
	return fromAlias, aliasArgs, typedArgs, nil
}

// takeWorktreeFlag takes the harness's worktree flag out of args, read as a
// launch reads it (worktreeRequest): "" when absent, "true" for a generated
// name, else the name.
func takeWorktreeFlag(call Call, args *[]string, flag string, spaced bool) (string, error) {
	name, given, rest, err := worktreeRequest(Call{Command: call.Command, Raw: *args}, flag, spaced)
	if err != nil || !given {
		return "", err
	}
	*args = rest
	if name == "" {
		return "true", nil
	}
	return name, nil
}

// workerLaunch builds the complete ordinary launch, including settings and
// the fresh conversation id, before the detached host receives it. The space
// owns name, role and room, which its record carries.
func workerLaunch(target workerTarget, call Call, conversation string) ([]string, error) {
	words, err := launch.WorkerArgs(target.harness, launch.Worker{
		Command: target.command, NoIntro: target.noIntro, Args: target.args,
		Worktree: target.worktree, Conversation: conversation,
	})
	if err != nil {
		return nil, &UsageError{Command: call.Command, Message: err.Error()}
	}
	return words, nil
}
