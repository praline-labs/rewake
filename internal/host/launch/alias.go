// An alias names a worker main can open: one name, one recorded launch.
//
// Since October 19, 2026 an alias is only main's catalog of workers
// (docs/v2/worker-profile.md): `rewake space open <space> --alias <name>`
// starts one, `rewake workers` lists them, and none is a launch of its own.
// The space owns the worker's name and room; the alias says which harness,
// which role, which program and which arguments.

package launch

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// Where aliases are read from, weakest first. The project file wins because it
// is read second, the same order the settings file follows.
const (
	// userFile sits beside the settings file, in ~/.config/rewake/.
	userDir  = "rewake"
	userFile = "aliases.toml"
	// projectFile is for aliases that belong to one repository.
	projectFile = ".rewake.toml"
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
	Command     string        `toml:"command"`
	Description string        `toml:"description"`
	Launch      AliasSettings `toml:"launch"`
}

// AliasSettings are the portable settings of a worker. The harness turns
// each into its own flags; one it has no flag for is only shown.
type AliasSettings struct {
	Model   string `toml:"model"`
	Effort  string `toml:"effort"`
	Context string `toml:"context"`
}

// CommandFlag is rewake's flag for the program a launch starts, which the
// alias field of the same name becomes in the worker's launch.
const CommandFlag = "command"

// Help is the one line that tells a person aliases exist, printed with every
// launch command's help. Somebody who has to read the documentation to learn
// that a feature exists will not find it.
const Help = "Main opens workers by alias: [alias.<name>] in ~/.config/" + userDir + "/" + userFile +
	" or in " + projectFile + " in the working directory, with harness = \"<id>\", role = \"write\" or \"general\", " +
	"args = [arguments for the harness], description, [alias.<name>.launch] model, effort and context, and, " +
	"in the user file only, role and command = \"<program>\" for a wrapper script. rewake workers lists them; " +
	"rewake space open <space> --alias <name> opens one. An alias is not a launch of its own."

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
	// Applied says which of the settings became flags.
	Applied struct{ Model, Effort, Context bool }
}

// Template resolves one alias, refusing one that would start something other
// than what it says.
func (s *Set) Template(name string, catalog adapter.Catalog) (Template, error) {
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
	for _, value := range []string{a.Description, a.Launch.Model, a.Launch.Effort, a.Launch.Context} {
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
