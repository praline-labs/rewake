package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/launch"
)

type workerSetting struct {
	Value   string `json:"value"`
	Applied bool   `json:"applied"`
}

type workerTemplate struct {
	Name        string        `json:"name"`
	Harness     string        `json:"harness"`
	Role        string        `json:"role"`
	Command     string        `json:"command"`
	Description string        `json:"description"`
	Main        bool          `json:"main"`
	Model       workerSetting `json:"model"`
	Effort      workerSetting `json:"effort"`
	Context     workerSetting `json:"context"`
}

type workersListing struct {
	Workers []workerTemplate `json:"workers"`
}

func workersCommand() *Command {
	return &Command{
		Name: "workers", MaxPositionals: 0,
		Summary:  "Read launch aliases as worker templates, without starting anything.",
		Options:  []Option{{Flag: "--all", Summary: "Include aliases marked main or launching with --main."}, jsonOption},
		Examples: []string{"rewake workers", "rewake workers --all --json"},
		Next:     []string{"rewake space open <name> --alias <alias>"},
		Notes: []string{
			"Any role may read this catalog. Aliases come from the user alias file, then the current directory's .rewake.toml, as for a direct launch. An unreadable file refuses the listing rather than silently omit workers.",
			"Model, effort and context describe the alias's launch table, not a running session. Model and effort apply unless launch.apply is false; context is always only shown. Typed harness flags override applied settings at launch.",
			"main = true marks a main template; --main in an older alias does too. Neither may be opened as a worker. --all shows them marked.",
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
		view, _, err := ctx.workerAlias(name)
		if err != nil {
			return &UsageError{Command: call.Command, Message: err.Error()}
		}
		if !view.Main || call.Switch("all") {
			out.Workers = append(out.Workers, view)
		}
	}
	return printValue(ctx, out, out.lines)
}

func (ctx *Context) launchAliases() *launch.Set {
	if ctx.aliases == nil {
		ctx.aliases = launch.Load()
	}
	return ctx.aliases
}

// workerAlias uses the launch parser so role and command mean exactly what
// they do on a direct launch; this view neither resolves nor runs the program.
func (ctx *Context) workerAlias(name string) (workerTemplate, Call, error) {
	aliases := ctx.launchAliases()
	value, _, ok := aliases.Lookup(name)
	if !ok {
		return workerTemplate{}, Call{}, fmt.Errorf("no alias %q; available: %s", name, strings.Join(aliases.Names(), ", "))
	}
	words, err := aliases.ExpandConfigured([]string{name}, ctx.table.catalog, ctx.table.launchFlagTakesValue, projectForbidden())
	if err != nil {
		return workerTemplate{}, Call{}, err
	}
	parsed, err := ctx.table.parse(words)
	if err != nil {
		return workerTemplate{}, Call{}, err
	}
	if parsed.Help || parsed.Guide || parsed.Version || parsed.Call.Command == nil || parsed.Call.Command.Harness == nil {
		return workerTemplate{}, Call{}, fmt.Errorf("alias %q does not describe a worker launch", name)
	}
	part, err := chosenRole(parsed.Call)
	if err != nil {
		return workerTemplate{}, Call{}, err
	}
	if part.ID == "" {
		part = role.General
	}
	applied := value.Launch.Applied()
	view := workerTemplate{
		Name: name, Harness: value.Harness, Role: part.ID,
		Command:     parsed.Call.Flag(launch.CommandFlag, value.Harness),
		Description: value.Description, Main: value.Main || part.ID == role.Main.ID,
		Model:   workerSetting{Value: value.Launch.Model, Applied: applied && value.Launch.Model != ""},
		Effort:  workerSetting{Value: value.Launch.Effort, Applied: applied && value.Launch.Effort != ""},
		Context: workerSetting{Value: value.Launch.Context},
	}
	return view, parsed.Call, nil
}

func (l workersListing) lines() []string {
	if len(l.Workers) == 0 {
		return []string{"No worker aliases. Use rewake workers --all to include main aliases."}
	}
	var lines []string
	for _, w := range l.Workers {
		line := fmt.Sprintf("%s  harness=%s role=%s command=%s", w.Name, w.Harness, w.Role, strconv.Quote(w.Command))
		if w.Main {
			line += " [main]"
		}
		for _, setting := range []struct {
			name  string
			value workerSetting
		}{{"model", w.Model}, {"effort", w.Effort}, {"context", w.Context}} {
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
