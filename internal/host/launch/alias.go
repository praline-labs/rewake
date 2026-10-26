// An alias names a worker main can open: one name, one recorded launch.
//
// Since October 19, 2026 an alias is only main's catalog of workers
// (docs/worker-profile.md): `rewake space open <space> --alias <name>`
// starts one, `rewake workers` lists them, and none is a launch of its own.
// The space owns the worker's name and room; the alias says which harness,
// which role, which program and which arguments.

package launch

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/core/worktype"
)

// Where aliases are read from, weakest first. The project file wins because it
// is read second, the same order the settings file follows.
const (
	// userFile sits beside the settings file, in ~/.config/rewake/.
	userDir  = "rewake"
	userFile = "aliases.toml"
	// projectDir holds what belongs to one repository: its aliases, as
	// userFile, and its types.
	projectDir = ".rewake"
	// movedProjectFile held a project's aliases until October 21, 2026.
	movedProjectFile = ".rewake.toml"
	// typesDir holds types, beside the alias file in both places.
	typesDir = "types"
)

// Alias is one worker of the catalog.
//
// Lists rather than one string, throughout. A string would have to be split
// into words, and splitting words means quoting rules — the part of this that
// has broken twice before.
type Alias struct {
	// Harness is the worker's harness, by its id in the catalog.
	Harness string `toml:"harness"`
	// Role is the worker's role, write or general; general when empty.
	Role string `toml:"role"`
	// Args are handed to the harness untouched.
	Args []string `toml:"args"`
	// Command is the program started instead of the harness's own — a wrapper
	// script that runs it.
	Command     string `toml:"command"`
	Description string `toml:"description"`
	// Worktree starts the worker in a new worktree of its own, as a typed
	// --worktree on space open does.
	Worktree bool `toml:"worktree"`
	// EnterWorktree lets the worker, started without a checkout, enter one
	// rewake makes for it mid-session, once (docs/worktree-tools.md).
	EnterWorktree bool `toml:"enter_worktree"`
	// Type is the kind of work its worker does unless space open names
	// another (docs/worker-types.md).
	Type   string        `toml:"type"`
	Launch AliasSettings `toml:"launch"`
}

// AliasSettings are the portable settings of a worker. The harness turns
// each into its own flags; one it has no flag for is only shown.
type AliasSettings struct {
	Model   string `toml:"model"`
	Effort  string `toml:"effort"`
	Context string `toml:"context"`
	// Label is how the strip and rewake list name the model, for a harness
	// that gives its model no name of its own.
	Label string `toml:"label"`
}

// CommandFlag is rewake's flag for the program a launch starts, which the
// alias field of the same name becomes in the worker's launch.
const CommandFlag = "command"

// Help is the one line that tells a person aliases exist, printed with every
// launch command's help. Somebody who has to read the documentation to learn
// that a feature exists will not find it.
const Help = "Main opens workers by alias: [alias.<name>] in ~/.config/" + userDir + "/" + userFile +
	" or in " + projectDir + "/" + userFile + " in the working directory, with harness = \"<id>\", role = \"write\" or \"general\", " +
	"args = [arguments for the harness], description, worktree = true for a worker in a new worktree of its own or, on Claude Code, enter_worktree = true for one that may enter one mid-session, [alias.<name>.launch] model, effort and context, and, " +
	"in the user file only, role and command = \"<program>\" for a wrapper script. " +
	"An alias's type = \"<type>\" is its worker's kind of work by default, one of the types: texts in ~/.config/" + userDir + "/" + typesDir + "/<type>.md and " + projectDir + "/" + typesDir + "/<type>.md beside rewake's own; [types] off = [...] in either alias file hides built-in ones, and [aliases] off = [...] in the project file hides templates of the catalog for that project. " +
	"rewake workers lists them; rewake space open <space> --alias <name> [--type <type>] opens one. An alias is not a launch of its own."

// Set is what this process read.
type Set struct {
	aliases map[string]Alias
	sources map[string]string
	// fromProject marks the aliases read from a file in the working directory,
	// which may decide less than the user's own file.
	fromProject map[string]bool
	// Notes are advisories for the person, such as a user file open to
	// others; they are printed and refuse nothing.
	Notes []string
	// problems are files that could not be read, which refuse the catalog
	// through Err rather than leave a worker silently absent.
	problems []string
	// types are the type files read, a project's replacing the user's of
	// the same name; off names the built-in types turned off, with where.
	types map[string]worktype.Type
	off   map[string]string
	// aliasOff names the aliases a project file turned off, with where; they
	// are gone from aliases, and Template refuses them by name.
	aliasOff map[string]string
	// pendingOff is what the project file asked, applied once every file is read.
	pendingOff []offRequest
}

// Names are the aliases this process knows, in order, for a refusal that says
// what does exist.
func (s *Set) Names() []string {
	names := make([]string, 0, len(s.aliases))
	for name := range s.aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Lookup returns one alias and where it was read from.
func (s *Set) Lookup(name string) (Alias, string, bool) {
	value, ok := s.aliases[name]
	return value, s.sources[name], ok
}

// Template is an alias resolved against its harness: what space open starts.
type Template struct {
	Name        string
	Harness     adapter.Harness
	Role        role.Role
	Command     string
	Description string
	// Args are the alias's arguments with its applied settings in them.
	Args     []string
	Settings AliasSettings
	// Worktree says the worker starts in a new worktree of its own.
	Worktree bool
	// EnterWorktree says it may enter one mid-session instead.
	EnterWorktree bool
	// Type is its default kind of work, as the alias names it; space open
	// resolves it (Set.Type).
	Type string
	// Applied says which of the settings became flags.
	Applied struct{ Model, Effort, Context bool }
}

// Template resolves one alias, refusing one that would start something other
// than what it says.
func (s *Set) Template(name string, catalog adapter.Catalog) (Template, error) {
	if where, off := s.aliasOff[name]; off {
		return Template{}, fmt.Errorf("the alias %q is turned off in %s ([aliases] off); available: %s", name, where, strings.Join(s.Names(), ", "))
	}
	value, source, ok := s.Lookup(name)
	if !ok {
		return Template{}, fmt.Errorf("no alias %q; available: %s", name, strings.Join(s.Names(), ", "))
	}
	if err := value.check(name, source, catalog.IDs(), s.fromProject[name]); err != nil {
		return Template{}, err
	}
	h, _ := catalog.Find(value.Harness)
	part := role.General
	if value.Role != "" {
		part, _ = role.Find(value.Role)
	}
	template := Template{
		Name: name, Harness: h, Role: part, Command: value.Command,
		Description: value.Description, Args: slices.Clone(value.Args), Settings: value.Launch,
		Worktree: value.Worktree, EnterWorktree: value.EnterWorktree, Type: value.Type,
	}
	if err := worktreeField(h, value); err != nil {
		return Template{}, fmt.Errorf("alias %q in %s: %w", name, source, err)
	}
	if err := enterWorktreeField(h, value); err != nil {
		return Template{}, fmt.Errorf("alias %q in %s: %w", name, source, err)
	}
	flags, err := settingFlags(h, value.Launch, &template)
	if err != nil {
		return Template{}, fmt.Errorf("alias %q: %w", name, err)
	}
	if len(flags) > 0 {
		// Structured fields replace an older copy in args, even when args
		// ends with a prompt terminator. New flags still precede that tail.
		visible := adapter.BeforeTerminator(template.Args)
		kept := replaced(visible, flags, h.SingleUseFlags())
		template.Args = adapter.AddFlags(append(slices.Clone(kept), template.Args[len(visible):]...), flags...)
	}
	return template, nil
}

// worktreeField refuses worktree = true where it cannot be what it says: on a
// harness rewake makes no worktree for, or beside the harness's worktree flag
// in args, which would leave two answers to one question.
func worktreeField(h adapter.Harness, value Alias) error {
	if !value.Worktree {
		return nil
	}
	taker, ok := h.(adapter.WorktreeHarness)
	if !ok {
		return fmt.Errorf("worktree = true, yet %s makes no worktree through rewake", h.Title())
	}
	if adapter.HasFlag(adapter.BeforeTerminator(value.Args), taker.WorktreeFlag()) {
		return fmt.Errorf("worktree = true and %s in args; keep one, and name a worktree with --worktree=<name> on space open", taker.WorktreeFlag())
	}
	return nil
}

// enterWorktreeField refuses enter_worktree = true on a harness whose session
// cannot move into a checkout, and beside a checkout made at the start, by
// worktree = true or the harness's flag in args: a worker has one checkout,
// made at its start or entered later, not both.
func enterWorktreeField(h adapter.Harness, value Alias) error {
	if !value.EnterWorktree {
		return nil
	}
	if _, ok := h.(adapter.WorktreeEntryHarness); !ok {
		return fmt.Errorf("enter_worktree = true, yet a %s session cannot enter a worktree mid-session; worktree = true starts it in one", h.Title())
	}
	if value.Worktree {
		return errors.New("worktree = true and enter_worktree = true; a worker has one checkout, made at its start or entered later, so keep one")
	}
	if taker, ok := h.(adapter.WorktreeHarness); ok && adapter.HasFlag(adapter.BeforeTerminator(value.Args), taker.WorktreeFlag()) {
		return fmt.Errorf("enter_worktree = true and %s in args; a worker has one checkout, made at its start or entered later, so keep one", taker.WorktreeFlag())
	}
	return nil
}

// settingFlags turns the settings into the harness's flags and records which
// of them it applied. Context is only shown on a harness with no flag for it.
func settingFlags(h adapter.Harness, settings AliasSettings, template *Template) ([]string, error) {
	translator, ok := h.(adapter.LaunchSettingsHarness)
	if settings.Model == "" && settings.Effort == "" && (!ok || settings.Context == "" || !translator.AppliesContext()) {
		return nil, nil
	}
	if !ok {
		return nil, fmt.Errorf("%s has no launch settings; put the model and effort in args", h.Title())
	}
	context := settings.Context
	if !translator.AppliesContext() {
		context = ""
	}
	flags, err := translator.LaunchSettings(settings.Model, settings.Effort, context)
	if err != nil {
		return nil, err
	}
	template.Applied.Model, template.Applied.Effort, template.Applied.Context = settings.Model != "", settings.Effort != "", context != ""
	return flags, nil
}

// check refuses an alias that would start something unusable, and says what.
// A worker that fails three layers down, complaining about a flag the person
// never typed, is worse than no alias at all.
func (a Alias) check(name, source string, knownHarnesses []string, fromProject bool) error {
	if a.Type != "" && !worktype.ValidName(a.Type) {
		return fmt.Errorf("alias %q in %s has type = %q, which cannot name a type; rewake workers lists them", name, source, a.Type)
	}
	for _, value := range []string{a.Description, a.Launch.Model, a.Launch.Effort, a.Launch.Context, a.Launch.Label} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("alias %q in %s has a multiline description or launch setting; each must be one line", name, source)
		}
	}
	if a.Harness == "" {
		return fmt.Errorf("alias %q in %s names no harness; add harness = \"<name>\"", name, source)
	}
	if !slices.Contains(knownHarnesses, a.Harness) {
		return fmt.Errorf("alias %q in %s starts %q, which is not a harness; known: %s",
			name, source, a.Harness, strings.Join(knownHarnesses, ", "))
	}
	if a.Role != "" {
		if part, found := role.Find(a.Role); !found || part.Spaces {
			return fmt.Errorf("alias %q in %s has role = %q; a worker's role is %q or %q", name, source, a.Role, role.Write.ID, role.General.ID)
		}
	}
	if fromProject && (a.Role != "" || a.Command != "") {
		// A file in whatever directory somebody happens to be in must not
		// choose the role of a worker opened there, nor the program it starts:
		// the program is whatever runs with the person's credentials.
		what := "role"
		if a.Command != "" {
			what = "command"
		}
		return fmt.Errorf("alias %q in %s sets %s; it belongs in the user alias file, not in one carried by a directory", name, source, what)
	}
	for _, argument := range a.Args {
		if argument == "" {
			return fmt.Errorf("alias %q in %s has an empty argument", name, source)
		}
	}
	return nil
}
