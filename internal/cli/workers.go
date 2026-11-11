package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
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
	// StartMode is the runtime work mode space open starts this worker in:
	// the template's start_mode, else the merged global one, else advisor. A
	// configured mode the catalog does not hold refused the catalog already,
	// so this one is always there.
	StartMode string        `json:"startMode"`
	Model     workerSetting `json:"model"`
	Effort    workerSetting `json:"effort"`
	Context   workerSetting `json:"context"`
	// Label names the model where the harness gives it no name of its own.
	Label string `json:"label,omitempty"`
}

type workersListing struct {
	Workers []workerTemplate `json:"workers"`
	// Modes are the runtime work modes space open --work-mode takes, in the
	// cycle's order: the built-ins first, then the custom-only names sorted.
	Modes []workmode.Mode `json:"modes"`
	// NoWriter says no launchable template has the write role;
	// main still delegates implementation.
	NoWriter bool `json:"noWriter"`
}

func workersCommand() *Command {
	return &Command{
		Name: "workers", MaxPositionals: 0,
		Summary:  "List the workers main can open by alias, and the runtime work modes they start in, without starting anything.",
		Options:  []Option{jsonOption},
		Examples: []string{"rewake workers", "rewake workers --json"},
		Next:     []string{"rewake space open <name> --alias <alias>"},
		Notes: []string{
			"Any role may read this catalog. Only worker-capable templates are listed; aliases on a main-only harness are omitted, and space open refuses them by name, through command wrappers too. New workers are pi only; Claude Code still launches main and existing sessions stay running. rewake ships no templates: each is an [alias.<name>] in the user alias file, ~/.config/rewake/aliases.toml, or in .rewake/aliases.toml of the directory the command runs in — not the room's, so run workers and space open from the project's top, where its file, its modes and its [aliases] off are read. A project alias with the user's name replaces the user's whole, role and command included. The project file may not set role or command. It may hold [aliases] off = [\"<alias>\", ...], which hides those templates of the merged catalog, the user's included; a name that is no template, or one the same file defines, refuses the catalog, and so does [aliases] off in the user file. A .rewake.toml in the directory is refused: the project file is .rewake/aliases.toml. An unreadable file refuses the listing rather than silently omit workers.",
			"A work mode is the session's one runtime behavioral section, and every session has one: " + builtInModes() + " are rewake's, each a complete text that replaces any other. The person's ~/.config/rewake/modes/<mode>.md and the project's .rewake/modes/<mode>.md hold their own, the nearest of one name winning; a file of a built-in's name replaces its text and keeps its place in the cycle. A mode file is Markdown opening with a TOML front matter between two +++ lines that holds description = \"<one line>\", at most 32 KiB. The cycle is review, advisor, build, process, then the custom-only names sorted. An old type directory or [types] section is refused with where its files go; a type's text moves to modes/<name>.md unchanged, and a custom code or analysis text takes the name it is meant to answer to.",
			"An alias: [alias.<name>] with harness = \"<id>\"; role = \"write\" or \"general\" (general when absent); args = [the harness's arguments]; command = \"<program>\", a wrapper that runs the harness; description; worktree = true, a worker started in a new worktree of its own, which --worktree=<name> on space open names and --no-worktree leaves out; enter_worktree = true, retained for existing worker-profile internals on a harness that supports entry, refused beside worktree = true. And [alias.<name>.launch] with model and effort, which become the harness's flags; context, the window auto-compaction keeps to — Claude Code's --autocompact, auto or 100k to 1M such as 300k —, applied where the harness has a flag for it and only shown elsewhere; and label, how the strip and rewake list name the model where the harness gives it no name. Modes are configured by [pi] and [alias.<name>.pi] with start_mode = \"<mode>\"; [pi.modes.<mode>] and [alias.<name>.pi.modes.<mode>] take model = provider/model-id and effort. Global fields merge user then project; the selected template overlays them. Start precedence is space open --work-mode, template start_mode, global start_mode, then advisor (main included); a configured or typed mode the catalog does not hold is refused. Initial explicit harness flags win, then the starting mode, then ordinary launch defaults. Review and advisor are prompt-only read-only guidance, not permissions. space open --help says how harness flags after -- meet the template's.",
		},
		Access: viewForm, Handler: handleWorkers,
	}
}

func handleWorkers(ctx *Context, call Call) error {
	aliases := ctx.launchAliases()
	if err := aliases.Err(); err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	out := workersListing{Workers: []workerTemplate{}, Modes: modesWithoutText(aliases.Catalog())}
	for _, name := range aliases.Names() {
		template, err := aliases.Template(name, ctx.table.catalog)
		if err != nil {
			var mainOnly *launch.MainOnlyTemplateError
			if errors.As(err, &mainOnly) {
				continue
			}
			return &UsageError{Command: call.Command, Message: err.Error()}
		}
		view := workerView(template)
		view.StartMode = aliases.Modes(template.Name).Start()
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

// builtInModes names the modes rewake ships, for the help pages.
func builtInModes() string {
	return strings.Join(workmode.Names(workmode.BuiltIns()), ", ")
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
		Model:   workerSetting{Value: settings.Model, Applied: template.Applied.Model},
		Effort:  workerSetting{Value: settings.Effort, Applied: template.Applied.Effort},
		Context: workerSetting{Value: settings.Context, Applied: template.Applied.Context},
		Label:   settings.Label,
	}
}

// modesWithoutText is the catalog as workers shows it: a mode's name, its
// source, its path and its description, not the text a session reads.
func modesWithoutText(catalog []workmode.Mode) []workmode.Mode {
	out := make([]workmode.Mode, len(catalog))
	for index, value := range catalog {
		value.Text = ""
		out[index] = value
	}
	return out
}

func (l workersListing) lines() []string {
	lines := l.workerLines()
	if l.NoWriter {
		lines = append(lines, "", "No template writes code here: main remains the orchestrator; ask the owner for a writer template.")
	}
	lines = append(lines, "", "Modes in cycle order (space open --work-mode <mode>):")
	if len(l.Modes) == 0 {
		lines = append(lines, "  none")
	}
	for _, value := range l.Modes {
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
		return []string{"No launchable worker templates: define [alias.<name>] for a worker-capable harness in ~/.config/rewake/aliases.toml or .rewake/aliases.toml; rewake workers --help lists the fields."}
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
		line += " start_mode=" + w.StartMode
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
