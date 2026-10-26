package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/core/role"
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
	// Label names the model where the harness gives it no name of its own.
	Label string `json:"label,omitempty"`
}

type workersListing struct {
	Workers []workerTemplate `json:"workers"`
	// Types are the kinds of work space open --type takes.
	Types []worktype.Type `json:"types"`
	// NoWriter says no template of the catalog as read now has the write
	// role, so main writes the code itself.
	NoWriter bool `json:"noWriter"`
}

func workersCommand() *Command {
	return &Command{
		Name: "workers", MaxPositionals: 0,
		Summary:  "List the workers main can open by alias, without starting anything.",
		Options:  []Option{jsonOption},
		Examples: []string{"rewake workers", "rewake workers --json"},
		Next:     []string{"rewake space open <name> --alias <alias>"},
		Notes: []string{
			"Any role may read this catalog. rewake ships no templates: each is an [alias.<name>] in the user alias file, ~/.config/rewake/aliases.toml, or in .rewake/aliases.toml of the directory the command runs in — not the room's, so run workers and space open from the project's top, where its file, its types and its [aliases] off are read. A project alias with the user's name replaces the user's whole, role and command included. The project file may not set role or command. It may hold [aliases] off = [\"<alias>\", ...], which hides those templates of the merged catalog, the user's included; a name that is no template, or one the same file defines, refuses the catalog, and so does [aliases] off in the user file. A .rewake.toml in the directory is refused: the project file is .rewake/aliases.toml. An unreadable file refuses the listing rather than silently omit workers.",
			"Types are the kinds of work a worker is raised for, each a text that follows its role's briefing: rewake's built-in " + builtInTypes() + "; the person's ~/.config/rewake/types/<type>.md; the project's .rewake/types/<type>.md. The nearest of one name wins. A type file is Markdown opening with a TOML front matter between two +++ lines that holds description = \"<one line>\", at most 32 KiB. [types] off = [\"<built-in type>\", ...] in either alias file hides those built-in ones; a file of the same name still stands. An alias's type = \"<type>\" is its default, shown with why space open would refuse it when it is not there or turned off.",
			"An alias: [alias.<name>] with harness = \"<id>\"; role = \"write\" or \"general\" (general when absent); args = [the harness's arguments]; command = \"<program>\", a wrapper that runs the harness; description; type = \"<type>\"; worktree = true, a worker started in a new worktree of its own, which --worktree=<name> on space open names and --no-worktree leaves out; enter_worktree = true, on Claude Code, a worker started without one that may enter one mid-session through its worktree_enter tool, refused beside worktree = true. And [alias.<name>.launch] with model and effort, which become the harness's flags; context, the window auto-compaction keeps to — Claude Code's --autocompact, auto or 100k to 1M such as 300k —, applied where the harness has a flag for it and only shown elsewhere; and label, how the strip and rewake list name the model where the harness gives it no name. A template with no model or effort takes the launch defaults rewake <harness> --help names. space open --help says how harness flags after -- meet the template's.",
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
	out.NoWriter = true
	for _, w := range out.Workers {
		if w.Role == role.Write.ID {
			out.NoWriter = false
		}
	}
	return printValue(ctx, out, out.lines)
}

// builtInTypes names the types rewake ships, for the help pages.
func builtInTypes() string {
	return strings.Join(worktype.Names(worktype.BuiltIns()), ", ")
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
		Label:   settings.Label,
	}
}

func (l workersListing) lines() []string {
	lines := l.workerLines()
	if l.NoWriter && len(l.Workers) > 0 {
		lines = append(lines, "", "No template writes code here: main writes it itself.")
	}
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
		return []string{"No worker aliases: define [alias.<name>] in ~/.config/rewake/aliases.toml or .rewake/aliases.toml; rewake workers --help lists the fields."}
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
		if w.Label != "" {
			line += " label=" + strconv.Quote(w.Label)
		}
		if w.Description != "" {
			line += " — " + w.Description
		}
		lines = append(lines, line)
	}
	return lines
}
