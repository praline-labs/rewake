package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// Run executes one invocation and returns the process exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	ctx := &Context{Stdout: stdout, Stderr: stderr}

	result, err := parse(argv)
	if err != nil {
		return report(ctx, err)
	}
	ctx.JSON = result.Call.Switch("json")

	switch {
	case result.Version:
		emit(ctx, "rewake "+Version)
		return ExitOK
	case result.Help && result.Call.Command != nil:
		printText(ctx, formatCommandHelp(result.Call.Command, true))
		return ExitOK
	case result.Help, result.Guide:
		printText(ctx, formatGuide())
		return ExitOK
	}

	if err := result.Call.Command.Handler(ctx, result.Call); err != nil {
		return report(ctx, err)
	}
	return ExitOK
}

// report prints a failure the way the guide promises and returns its exit code.
func report(ctx *Context, err error) int {
	var usage *UsageError
	if errors.As(err, &usage) {
		fmt.Fprintln(ctx.Stderr, usage.Message)
		if hint := formatHint(usage.Command); hint != "" {
			fmt.Fprintln(ctx.Stderr, hint)
		}
		return ExitUsage
	}

	var pending *PendingError
	if errors.As(err, &pending) {
		fmt.Fprintln(ctx.Stdout, pending.Message)
		return ExitPending
	}

	fmt.Fprintln(ctx.Stderr, err.Error())
	return ExitFailed
}

func handleGuide(ctx *Context, _ Call) error {
	if ctx.JSON {
		return printValue(ctx, guideModel(), func() []string { return nil })
	}
	printText(ctx, formatGuide())
	return nil
}

// guideModel is the machine form of the guide: the same table, all fields.
func guideModel() map[string]any {
	type optionModel struct {
		Flag     string `json:"flag"`
		Value    string `json:"value,omitempty"`
		Summary  string `json:"summary"`
		Required bool   `json:"required,omitempty"`
	}
	type commandModel struct {
		Name     string        `json:"name"`
		Args     string        `json:"args,omitempty"`
		Summary  string        `json:"summary"`
		Options  []optionModel `json:"options,omitempty"`
		Examples []string      `json:"examples,omitempty"`
		Next     []string      `json:"next,omitempty"`
		Notes    []string      `json:"notes,omitempty"`
	}
	type groupModel struct {
		Title    string         `json:"title"`
		Summary  string         `json:"summary,omitempty"`
		Commands []commandModel `json:"commands"`
	}

	renderOptions := func(options []Option) []optionModel {
		out := make([]optionModel, 0, len(options))
		for _, option := range options {
			out = append(out, optionModel{Flag: option.Flag, Value: option.Value, Summary: option.Summary, Required: option.Required})
		}
		return out
	}

	groupModels := make([]groupModel, 0, len(Groups()))
	for _, group := range Groups() {
		commands := make([]commandModel, 0, len(group.Commands))
		for _, command := range group.Commands {
			commands = append(commands, commandModel{
				Name:     command.Name,
				Args:     command.Args,
				Summary:  command.Summary,
				Options:  renderOptions(command.Options),
				Examples: command.Examples,
				Next:     command.Next,
				Notes:    command.Notes,
			})
		}
		groupModels = append(groupModels, groupModel{Title: group.Title, Summary: group.Summary, Commands: commands})
	}

	return map[string]any{
		"version":       Version,
		"groups":        groupModels,
		"flow":          flow(),
		"notes":         notes(),
		"globalOptions": renderOptions(globalOptions),
		"harnesses":     harness.IDs(),
	}
}

// The handlers below are filled in by later milestones; the table, the parser
// and the refusals are already the real ones.

func handleLaunch(h harness.Harness) func(*Context, Call) error {
	return func(_ *Context, _ Call) error {
		return failf("starting %s is not built yet (milestone 3 for claude, 4 for codex).", h.Title())
	}
}

func handleList(_ *Context, _ Call) error {
	return failf("list is not built yet (milestone 2).")
}

func handleSend(_ *Context, _ Call) error {
	return failf("send is not built yet (milestone 3).")
}

func handleWhoami(_ *Context, _ Call) error {
	return failf("whoami is not built yet (milestone 2).")
}
