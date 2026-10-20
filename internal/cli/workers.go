package cli

import (
	"fmt"
	"strconv"

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
	EnterWorktree bool          `json:"enterWorktree"`
	Model         workerSetting `json:"model"`
	Effort        workerSetting `json:"effort"`
	Context       workerSetting `json:"context"`
}

type workersListing struct {
	Workers []workerTemplate `json:"workers"`
}

func workersCommand() *Command {
	return &Command{
		Name: "workers", MaxPositionals: 0,
		Summary:  "List the workers main can open by alias, without starting anything.",
		Options:  []Option{jsonOption},
		Examples: []string{"rewake workers", "rewake workers --json"},
		Next:     []string{"rewake space open <name> --alias <alias>"},
		Notes: []string{
			"Any role may read this catalog. Aliases come from the user alias file, ~/.config/rewake/aliases.toml, then the current directory's .rewake.toml, which may not set role or command. An unreadable file refuses the listing rather than silently omit workers.",
			"An alias: [alias.<name>] with harness = \"<id>\"; role = \"write\" or \"general\" (general when absent); args = [the harness's arguments]; command = \"<program>\", a wrapper that runs the harness; description; worktree = true, a worker started in a new worktree of its own, which --worktree=<name> on space open names and --no-worktree leaves out; enter_worktree = true, on Claude Code, a worker started without one that may enter one mid-session through its worktree_enter tool, refused beside worktree = true; and [alias.<name>.launch] with model, effort and context. Model and effort become the harness's flags; context does on a harness with a flag for it and is only shown elsewhere. Typed harness flags after -- in space open replace applied ones.",
			"Since October 19, 2026 an alias's rewake list, main = true and launch.apply are gone: a file with them is refused, naming each field and what replaced it.",
		},
		Access: viewForm, Handler: handleWorkers,
	}
}

func handleWorkers(ctx *Context, call Call) error {
	aliases := ctx.launchAliases()
	if err := aliases.Err(); err != nil {
		return &UsageError{Command: call.Command, Message: err.Error()}
	}
	out := workersListing{Workers: []workerTemplate{}}
	for _, name := range aliases.Names() {
		template, err := aliases.Template(name, ctx.table.catalog)
		if err != nil {
			return &UsageError{Command: call.Command, Message: err.Error()}
		}
		out.Workers = append(out.Workers, workerView(template))
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
		Model:   workerSetting{Value: settings.Model, Applied: template.Applied.Model},
		Effort:  workerSetting{Value: settings.Effort, Applied: template.Applied.Effort},
		Context: workerSetting{Value: settings.Context, Applied: template.Applied.Context},
	}
}

func (l workersListing) lines() []string {
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
