package cli

import (
	"fmt"
	"strconv"

	"github.com/praline-labs/rewake/internal/core/worktype"
	"github.com/praline-labs/rewake/internal/host/launch"
)

type workerSetting struct {
	Value   string `json:"value"`
	Applied bool   `json:"applied"`
}

type workerTemplate struct {
	Name        string `json:"name"`
	Harness     string `json:"harness"`
	Role        string `json:"role"`
	Command     string `json:"command"`
	Description string `json:"description"`
	// Worktree says the worker starts in a new worktree of its own.
	Worktree bool `json:"worktree"`
	// EnterWorktree says the worker may enter one mid-session instead.
	EnterWorktree bool `json:"enterWorktree"`
	// Type is the template's default kind of work; TypeProblem why space
	// open would refuse it — not there, or turned off.
	Type        string        `json:"type,omitempty"`
	TypeProblem string        `json:"typeProblem,omitempty"`
	Model       workerSetting `json:"model"`
	Effort      workerSetting `json:"effort"`
	Context     workerSetting `json:"context"`
}

type workersListing struct {
	Workers []workerTemplate `json:"workers"`
	// Types are the kinds of work space open --type takes.
	Types []worktype.Type `json:"types"`
}

func workersCommand() *Command {
	return &Command{
		Name: "workers", MaxPositionals: 0,
		Summary:  "List the workers main can open by alias, without starting anything.",
		Options:  []Option{jsonOption},
		Examples: []string{"rewake workers", "rewake workers --json"},
		Next:     []string{"rewake space open <name> --alias <alias>"},
		Notes: []string{
			"Any role may read this catalog. Aliases come from the user alias file, ~/.config/rewake/aliases.toml, then the current directory's .rewake/aliases.toml, which may not set role or command. A .rewake.toml, where a project's aliases once lived, is refused, naming the new place. A project's .rewake/aliases.toml may hold [aliases] off = [\"<alias>\", ...] to hide templates for that project; the user file may not. An unreadable file refuses the listing rather than silently omit workers.",
			"Types are the kinds of work a worker is raised for, each a text that follows its role's briefing: rewake's built-in code, analysis, process and review; the person's ~/.config/rewake/types/<type>.md; the project's .rewake/types/<type>.md. The nearest of one name wins. A type file is Markdown opening with a TOML front matter between two +++ lines that holds description = \"<one line>\", at most 32 KiB. [types] off = [\"<built-in type>\", ...] in either alias file hides those built-in ones; a file of the same name still stands. An alias's type = \"<type>\" is its default, shown with why space open would refuse it when it is not there or turned off.",
			"An alias: [alias.<name>] with harness = \"<id>\"; role = \"write\" or \"general\" (general when absent); args = [the harness's arguments]; command = \"<program>\", a wrapper that runs the harness; description; worktree = true, a worker started in a new worktree of its own, which --worktree=<name> on space open names and --no-worktree leaves out; enter_worktree = true, on Claude Code, a worker started without one that may enter one mid-session through its worktree_enter tool, refused beside worktree = true; and [alias.<name>.launch] with model, effort and context. Model and effort become the harness's flags; context does on a harness with a flag for it and is only shown elsewhere. Typed harness flags after -- in space open replace applied ones.",
			"An alias's old rewake list, main = true and launch.apply are gone: a file with them is refused, naming each field and what replaced it.",
		},
		Access: viewForm, Handler: handleWorkers,
	}
}

func handleWorkers(ctx *Context, call Call) error {
	aliases := ctx.launchAliases()
	if err := aliases.Err(); err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	out := workersListing{Workers: []workerTemplate{}, Types: aliases.Types()}
	for _, name := range aliases.Names() {
		template, err := aliases.Template(name, ctx.table.catalog)
		if err != nil {
			return &UsageError{Command: call.Command, Message: err.Error()}
		}
		view := workerView(template)
		if template.Type != "" {
			// Shown, not refused: the template still opens with --type or
			// --no-type.
			if _, err := aliases.Type(template.Type); err != nil {
				view.TypeProblem = err.Error()
			}
		}
		out.Workers = append(out.Workers, view)
	}
	return printValue(ctx, out, out.lines)
}

// launchAliases reads the catalog once per process, saying what is wrong
// with it where it is read.
func (ctx *Context) launchAliases() *launch.Set {
	if ctx.aliases == nil {
		ctx.aliases = launch.Load()
		for _, note := range ctx.aliases.Notes {
			_, _ = fmt.Fprintln(ctx.Stderr, "rewake: "+note)
		}
	}
	return ctx.aliases
}

// workerView is a template as the catalog shows it; the view neither
// resolves nor runs the program.
func workerView(template launch.Template) workerTemplate {
	command := template.Command
	if command == "" {
		command = template.Harness.ID()
	}
	settings := template.Settings
	return workerTemplate{
		Name: template.Name, Harness: template.Harness.ID(), Role: template.Role.ID,
		Command: command, Description: template.Description, Worktree: template.Worktree, EnterWorktree: template.EnterWorktree,
		Type:    template.Type,
		Model:   workerSetting{Value: settings.Model, Applied: template.Applied.Model},
		Effort:  workerSetting{Value: settings.Effort, Applied: template.Applied.Effort},
		Context: workerSetting{Value: settings.Context, Applied: template.Applied.Context},
	}
}

func (l workersListing) lines() []string {
	lines := l.workerLines()
	lines = append(lines, "", "Types (space open --type <type>):")
	if len(l.Types) == 0 {
		lines = append(lines, "  none")
	}
	for _, value := range l.Types {
		where := value.Source
		if value.Path != "" {
			where += " " + value.Path
		}
		lines = append(lines, fmt.Sprintf("  %s (%s) — %s", value.Name, where, value.Description))
	}
	return lines
}

func (l workersListing) workerLines() []string {
	if len(l.Workers) == 0 {
		return []string{"No worker aliases."}
	}
	var lines []string
	for _, w := range l.Workers {
		line := fmt.Sprintf("%s  harness=%s role=%s command=%s", w.Name, w.Harness, w.Role, strconv.Quote(w.Command))
		if w.Worktree {
			line += " worktree"
		}
		if w.EnterWorktree {
			line += " enter_worktree"
		}
		if w.Type != "" {
			line += " type=" + w.Type
			if w.TypeProblem != "" {
				line += " (not available: " + w.TypeProblem + ")"
			}
		}
		for _, setting := range []struct {
			name  string
			value workerSetting
		}{{"model", w.Model}, {"effort", w.Effort}, {"context", w.Context}} {
			if setting.value.Value == "" {
				continue
			}
			mode := "shown only"
			if setting.value.Applied {
				mode = "applied"
			}
			line += fmt.Sprintf(" %s=%s (%s)", setting.name, strconv.Quote(setting.value.Value), mode)
		}
		if w.Description != "" {
			line += " — " + w.Description
		}
		lines = append(lines, line)
	}
	return lines
}
