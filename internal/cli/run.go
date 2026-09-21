package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/iiiokojiadbi/rewake/internal/alias"
	"github.com/iiiokojiadbi/rewake/internal/harness"
)

// Run executes one invocation and returns the process exit code.
func Run(argv []string, stdout, stderr io.Writer) int {
	ctx := &Context{Stdout: stdout, Stderr: stderr}

	// Aliases first: everything downstream sees the arguments the alias stands
	// for, so nothing else in the CLI has to know that aliases exist.
	aliases := alias.Load()
	for _, note := range aliases.Notes {
		_, _ = fmt.Fprintln(stderr, "rewake: "+note)
	}
	expanded, err := aliases.Expand(argv, harness.IDs(), launchFlagTakesValue, roleFlags(), singleUseFlags)
	if err != nil {
		return report(ctx, &UsageError{Message: err.Error()})
	}

	result, err := parseKnowing(expanded, aliases.Names())
	if err != nil {
		return report(ctx, err)
	}
	ctx.JSON = result.Call.Switch("json")

	switch {
	case result.Version:
		_ = emit(ctx, "rewake "+Version)
		return ExitOK
	case result.Help && result.Call.Command != nil:
		printText(ctx, formatCommandHelp(result.Call.Command, true))
		return ExitOK
	case result.Help, result.Guide:
		// The guide has a machine form too: an agent's first call may well be
		// "rewake --json", and it must not get prose back.
		if ctx.JSON {
			if err := printValue(ctx, guideModel(), func() []string { return nil }); err != nil {
				return report(ctx, err)
			}
			return ExitOK
		}
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
		_, _ = fmt.Fprintln(ctx.Stderr, usage.Message)
		if hint := formatHint(usage.Command); hint != "" {
			_, _ = fmt.Fprintln(ctx.Stderr, hint)
		}
		return ExitUsage
	}

	var exitCode *ExitCodeError
	if errors.As(err, &exitCode) {
		return exitCode.Code
	}

	var pending *PendingError
	if errors.As(err, &pending) {
		_, _ = fmt.Fprintln(ctx.Stdout, pending.Message)
		return ExitPending
	}

	_, _ = fmt.Fprintln(ctx.Stderr, err.Error())
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
			out = append(out, optionModel(option))
		}
		return out
	}

	groupModels := make([]groupModel, 0, len(Groups()))
	for _, group := range Groups() {
		shown := visibleCommands(group)
		if len(shown) == 0 {
			continue
		}
		commands := make([]commandModel, 0, len(shown))
		for _, command := range shown {
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
