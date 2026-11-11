package cli

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/workmode"
	"github.com/praline-labs/rewake/internal/host/launch"
	"github.com/praline-labs/rewake/internal/infra/state"
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
	// StartModeSettings are the starting mode's configured model and effort,
	// which override the template's launch settings when this worker starts.
	// Absent when the mode sets neither. They are configuration, not a
	// prediction of the effective launch: typed flags and later environment
	// defaults may still differ.
	StartModeSettings *startModeSettings `json:"startModeSettings,omitempty"`
	// Label names the model where the harness gives it no name of its own.
	Label string `json:"label,omitempty"`
}

// startModeSettings is what the selected starting mode configures.
type startModeSettings struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

type workersListing struct {
	Workers []workerTemplate `json:"workers"`
	// Modes are the runtime work modes space open --work-mode takes, in the
	// cycle's order: the built-ins first, then the custom-only names sorted.
	Modes []workmode.Mode `json:"modes"`
	// NoWriter is inventory alone: no launchable template has the write role.
	// Whether main may implement anyway is MainImplements, apart from it.
	NoWriter bool `json:"noWriter"`
	// MainImplements is the authorship policy in force: a live main's frozen
	// snapshot, or what a fresh launch in this directory would take.
	MainImplements bool `json:"mainImplements"`
	// MainPolicySource says which of the two this listing answered:
	// "session" for a live main's record, "configuration" for the directory.
	MainPolicySource string `json:"mainPolicySource"`
	// policyExplicit says a project file set the policy rather than the
	// default deciding; it shapes the line, not the JSON.
	policyExplicit bool
}

// Where a listing's main policy came from.
const (
	mainPolicySession       = "session"
	mainPolicyConfiguration = "configuration"
)

func workersCommand() *Command {
	return &Command{
		Name: "workers", MaxPositionals: 0,
		Summary:  "List the workers main can open by alias, and the runtime work modes they start in, without starting anything.",
		Options:  []Option{jsonOption},
		Examples: []string{"rewake workers", "rewake workers --json"},
		Next:     []string{"rewake space open <name> --alias <alias>"},
		Notes: []string{
			"Any role may read this catalog. Only worker-capable templates are listed; aliases on a main-only harness are omitted, and space open refuses them by name, through command wrappers too. New workers are pi only; Claude Code still launches main and existing sessions stay running. rewake ships no templates: each is an [alias.<name>] in the user alias file, ~/.config/rewake/aliases.toml, or in .rewake/aliases.toml of the directory the command runs in — not the room's, so run workers and space open from the project's top, where its file, its modes and its [aliases] off are read. A project alias with the user's name replaces the user's whole, role and command included. The project file may not set role or command. It alone may set [main] implements = true or false: true lets main implement product changes itself, subject to its task, work mode and the project's rules, and false, the default, keeps it delegating. That policy is read when a main starts and frozen for its run; editing the file changes the next main there, never the running one. A live main's policy is what this listing shows for the room, and a listing outside one says the configuration is what a fresh launch here would take. It may hold [aliases] off = [\"<alias>\", ...], which hides those templates of the merged catalog, the user's included; a name the merged catalog does not hold is already off and changes nothing but a notice to remove it from the list, while one the same file defines refuses the catalog, and so does [aliases] off in the user file. A .rewake.toml in the directory is refused: the project file is .rewake/aliases.toml. An unreadable file refuses the listing rather than silently omit workers.",
			"A work mode is the session's one runtime behavioral section, and every session has one: " + builtInModes() + " are rewake's, each a complete text that replaces any other. The person's ~/.config/rewake/modes/<mode>.md and the project's .rewake/modes/<mode>.md hold their own, the nearest of one name winning; a file of a built-in's name replaces its text and keeps its place in the cycle. A mode file is Markdown opening with a TOML front matter between two +++ lines that holds description = \"<one line>\", at most 32 KiB. The cycle is review, advisor, build, process, then the custom-only names sorted. An old types/ directory or [types] section is skipped with a notice saying where its files go, and an alias that still carries type = ... is left out until it is fixed; only a type holding a string and [types] off = [\"<name>\", ...] are skipped, and a definition is checked against the current fields first, so a broken start_mode, mode or launch setting beside it still refuses the file. The user's and the project's definitions replace one another whole, so only the definition in force is left out and reported; the rest of the catalog runs and a launch's briefing carries the notice, so main can repair it. A type's text moves to modes/<name>.md unchanged, and a custom code or analysis text takes the name it is meant to answer to. Malformed TOML, an unknown field and a malformed value still refuse the file.",
			"An alias: [alias.<name>] with harness = \"<id>\"; role = \"write\" or \"general\" (general when absent); args = [the harness's arguments]; command = \"<program>\", a wrapper that runs the harness; description; worktree = true, a worker started in a new worktree of its own, which --worktree=<name> on space open names and --no-worktree leaves out; enter_worktree = true, retained for existing worker-profile internals on a harness that supports entry, refused beside worktree = true. And [alias.<name>.launch] with model and effort, which become the harness's flags; context, the window auto-compaction keeps to — Claude Code's --autocompact, auto or 100k to 1M such as 300k —, applied where the harness has a flag for it and only shown elsewhere; and label, how the strip and rewake list name the model where the harness gives it no name. Modes are configured by [pi] and [alias.<name>.pi] with start_mode = \"<mode>\"; [pi.modes.<mode>] and [alias.<name>.pi.modes.<mode>] take model = provider/model-id and effort. Global fields merge user then project; the selected template overlays them. Start precedence is space open --work-mode, template start_mode, global start_mode, then advisor (main included); a configured or typed mode the catalog does not hold is refused. Initial explicit harness flags win, then the starting mode, then ordinary launch defaults. The starting mode's own model and effort are shown as start_model and start_effort and override the template's launch settings; they are configuration, not a prediction of the effective launch. Review and advisor are prompt-only read-only guidance, not permissions. space open --help says how harness flags after -- meet the template's.",
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
		modes := aliases.Modes(template.Name)
		view.StartMode = modes.Start()
		if fields := modes.Fields(view.StartMode); fields.Model != "" || fields.Effort != "" {
			view.StartModeSettings = &startModeSettings{Model: fields.Model, Effort: fields.Effort}
		}
		out.Workers = append(out.Workers, view)
	}
	out.NoWriter = true
	for _, w := range out.Workers {
		if w.Role == role.Write.ID {
			out.NoWriter = false
		}
	}
	out.setMainPolicy(aliases)
	// A template an obsolete setting left out is not listed, but its current
	// settings are still checked: the type may omit a valid template, never
	// screen a malformed one.
	for _, name := range aliases.OmittedNames() {
		if _, err := aliases.Template(name, ctx.table.catalog); err != nil {
			var omitted *launch.OmittedTemplateError
			var mainOnly *launch.MainOnlyTemplateError
			if errors.As(err, &omitted) || errors.As(err, &mainOnly) {
				continue
			}
			return &UsageError{Command: call.Command, Message: err.Error()}
		}
	}
	return printValue(ctx, out, out.lines)
}

// setMainPolicy fills the listing's authorship policy. A live main's record is
// the room's snapshot, for every caller and whatever directory it stands in;
// with no live main the listing says the current configuration is what a fresh
// launch here would take, rather than presenting it as a running decision.
// A missing or unreadable state directory is no live main: the catalog is a
// view and must not create or change anything to answer.
func (l *workersListing) setMainPolicy(aliases *launch.Set) {
	if policy, live := liveMainPolicy(); live {
		l.MainImplements, l.MainPolicySource, l.policyExplicit = policy.Implements, mainPolicySession, policy.Explicit
		return
	}
	policy := aliases.Main()
	l.MainImplements, l.MainPolicySource, l.policyExplicit = policy.Implements, mainPolicyConfiguration, policy.Explicit
}

// liveMainPolicy is the frozen policy of the room's live main, if it has one.
func liveMainPolicy() (launch.MainPolicy, bool) {
	dir, err := state.Dir()
	if err != nil {
		return launch.MainPolicy{}, false
	}
	sessions, err := registry.ListReadOnly(dir)
	if err != nil {
		return launch.MainPolicy{}, false
	}
	for _, session := range sessions {
		if session.Role == role.Main.ID {
			return launch.MainPolicy{Implements: session.MainImplements}, true
		}
	}
	return launch.MainPolicy{}, false
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
		// Obsolete settings are skipped, not refused: the catalog runs on
		// what is valid, and the command says what to repair. A launch also
		// carries them into the session's briefing.
		for _, migration := range ctx.aliases.Migrations {
			_, _ = fmt.Fprintln(ctx.Stderr, "rewake: "+migration)
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
	lines = append(lines, "", l.mainPolicyLine())
	if l.NoWriter {
		if l.MainImplements {
			lines = append(lines, "  Main may implement product changes itself, within its work mode and the project's rules; no writer template is required.")
		} else {
			lines = append(lines, "  Implementation must be delegated: when implementation is requested, a writer template or the owner's decision is needed.")
		}
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

// mainPolicyLine says whether main may implement here and where that answer
// came from: a live main's frozen launch policy, or the configuration a fresh
// launch in this directory would take.
func (l workersListing) mainPolicyLine() string {
	state := "delegated"
	if l.MainImplements {
		state = "enabled"
	}
	if l.MainPolicySource == mainPolicySession {
		return fmt.Sprintf("Main implementation: %s by this room's live main; its launch froze that policy, so editing the alias file does not change this run.", state)
	}
	switch {
	case l.MainImplements:
		return "Main implementation: enabled by this directory's project configuration ([main] implements = true); a fresh main started here takes that policy."
	case l.policyExplicit:
		return "Main implementation: delegated by this directory's project configuration ([main] implements = false); a fresh main started here takes that policy."
	default:
		return "Main implementation: delegated, the default because this directory's project sets no [main] implements; a fresh main started here takes that policy."
	}
}

func (l workersListing) workerLines() []string {
	if len(l.Workers) == 0 {
		return []string{"No launchable worker templates. To delegate, define [alias.<name>] for a worker-capable harness in ~/.config/rewake/aliases.toml or .rewake/aliases.toml; rewake workers --help lists the fields."}
	}
	var lines []string
	if l.NoWriter {
		lines = append(lines, "No launchable write-role templates.")
	}
	for _, w := range l.Workers {
		line := fmt.Sprintf("%s  harness=%s role=%s command=%s", w.Name, w.Harness, w.Role, strconv.Quote(w.Command))
		if w.Worktree {
			line += " worktree"
		}
		if w.EnterWorktree {
			line += " enter_worktree"
		}
		line += " start_mode=" + w.StartMode
		if w.StartModeSettings != nil {
			if w.StartModeSettings.Model != "" {
				line += fmt.Sprintf(" start_model=%s (starting mode; overrides template model)", strconv.Quote(w.StartModeSettings.Model))
			}
			if w.StartModeSettings.Effort != "" {
				line += fmt.Sprintf(" start_effort=%s (starting mode; overrides template effort)", strconv.Quote(w.StartModeSettings.Effort))
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
