package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/praline-labs/rewake/internal/alias"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
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
	expanded, err := aliases.Expand(argv, harness.IDs(), launchFlagTakesValue, projectForbidden(), singleUseFlags)
	if err != nil {
		return report(ctx, &UsageError{Message: err.Error()})
	}

	if err := singleProgram(expanded); err != nil {
		return report(ctx, err)
	}
	result, err := parseKnowing(expanded, aliases.Names())
	if err != nil {
		return report(ctx, err)
	}
	ctx.JSON = result.Call.Switch("json")

	switch {
	case result.Version:
		build := thisBuild()
		if err := printValue(ctx, build, func() []string { return []string{build.Line()} }); err != nil {
			return report(ctx, err)
		}
		return ExitOK
	case result.Help && result.Call.Command != nil:
		printText(ctx, formatCommandHelp(result.Call.Command, true))
		return ExitOK
	case result.Help, result.Guide:
		// The guide has a machine form too: an agent's first call may well be
		// "rewake --json", and it must not get prose back.
		if ctx.JSON {
			if err := printValue(ctx, guideModel(callerPlaybook()), func() []string { return nil }); err != nil {
				return report(ctx, err)
			}
			return ExitOK
		}
		printText(ctx, formatGuide(callerPlaybook()))
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
		return printValue(ctx, guideModel(callerPlaybook()), func() []string { return nil })
	}
	printText(ctx, formatGuide(callerPlaybook()))
	return nil
}

// guideModel is the machine form of the guide: the same table, all fields.
func guideModel(play *role.Playbook) map[string]any {
	type optionModel struct {
		Flag     string `json:"flag"`
		Value    string `json:"value,omitempty"`
		Summary  string `json:"summary"`
		Required bool   `json:"required,omitempty"`
		// Repeatable is a flag that may be given more than once.
		Repeatable bool `json:"repeatable,omitempty"`
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

	model := map[string]any{
		"version":       Version,
		"build":         thisBuild(),
		"groups":        groupModels,
		"flow":          flow(),
		"notes":         notes(),
		"globalOptions": renderOptions(globalOptions),
		"harnesses":     harness.IDs(),
	}
	if play != nil {
		// The same answer the printed form gives, in the same words: an agent
		// reading --json must not get a different set of moves from the one a
		// person reads on the screen.
		steps := make([]map[string]string, 0, len(play.Steps))
		for _, step := range play.Steps {
			steps = append(steps, map[string]string{"do": step.Do, "why": step.Why})
		}
		sections := make([]map[string]any, 0, len(play.Sections))
		for _, section := range play.Sections {
			sections = append(sections, map[string]any{"title": section.Title, "lines": section.Lines})
		}
		model["role"] = map[string]any{"heading": play.Heading, "steps": steps, "sections": sections}
	}
	return model
}

// singleProgram refuses a launch line that names --command twice before the
// harness word. A launch starts one program, and letting the last one win
// would start something other than one of the two the person wrote. An
// alias's copy is not counted: a typed --command replaces it during expansion.
func singleProgram(argv []string) error {
	count := 0
	var command *Command
	for index := 0; index < len(argv); index++ {
		token := argv[index]
		if token == "--" {
			break
		}
		if !strings.HasPrefix(token, "-") {
			// The launch word, so the refusal can print its syntax and help.
			command = findCommand(token)
			break
		}
		name, _, inline := strings.Cut(strings.TrimPrefix(token, "--"), "=")
		if name == alias.CommandFlag {
			count++
		}
		if !inline && launchFlagTakesValue(name) {
			index++
		}
	}
	if count > 1 {
		return &UsageError{Command: command, Message: "--command given twice; a launch starts one program. Keep one --command before the harness name."}
	}
	return nil
}
