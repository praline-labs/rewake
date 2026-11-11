// An alias names a worker main can open: one name, one recorded launch.
//
// Since October 19, 2026 an alias is only main's catalog of workers
// (docs/worker-profile.md): `rewake space open <space> --alias <name>`
// starts one, `rewake workers` lists them, and none is a launch of its own.
// The space owns the worker's name and room; the alias says which harness,
// which role, which program and which arguments.

package launch

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/praline-labs/rewake/internal/core/workmode"
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
	// modesDir holds the modes, beside the alias file in both places.
	modesDir = "modes"
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
	EnterWorktree bool            `toml:"enter_worktree"`
	Launch        AliasSettings   `toml:"launch"`
	Pi            workmode.Config `toml:"pi"`
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
// launch command's help. The fields are on rewake workers --help alone, so the
// two pages cannot drift apart.
const Help = "Main opens workers from templates, rewake ships none: rewake workers --help says where they are defined and with which fields; rewake workers lists them and rewake space open <space> --alias <name> opens one."

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
	// Migrations are the obsolete settings this process skipped rather than
	// refused, each naming its file, its line where there is one and what to
	// write instead: the catalog runs on what is valid and main is told
	// what to repair (alias_obsolete.go).
	Migrations []string
	// obsolete holds the definitions a file left out because its obsolete
	// type is still there, the project's replacing the user's as a whole
	// template does; resolveObsolete decides which one is in force
	// (alias_obsolete.go).
	obsolete map[string]obsoleteDef
	// omitted names the templates an obsolete definition in force left
	// unavailable, with the notice that says what to fix; Template checks
	// their current settings and refuses them by name.
	omitted map[string]obsoleteDef
	// problems are files that could not be read, which refuse the catalog
	// through Err rather than leave a worker silently absent.
	problems []string
	// modes are the mode files read, a project's replacing the user's of the
	// same name; a built-in of that name keeps its place in the cycle.
	modes map[string]workmode.Mode
	// paths is where each alias was read, for a refusal that names the file.
	paths map[string]string
	// piSource is where the global [pi] settings were last read, for a
	// refusal that names the file.
	piSource string
	// aliasOff names the aliases a project file turned off, with where; they
	// are gone from aliases, and Template refuses them by name.
	aliasOff map[string]string
	// briefsDir is the project's [briefs] dir as written, relative to the
	// project; empty without one.
	briefsDir string
	// skillsNever are the skills a brief may not give, each with the file
	// that bars it: the user's and the project's lists joined.
	skillsNever map[string]string
	// pendingOff is what the project file asked, applied once every file is read.
	pendingOff []offRequest
	pi         workmode.Config
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

// available is what a refusal says exists: the templates, or where to define
// one when there are none.
func (s *Set) available() string {
	if len(s.aliases) == 0 {
		return "no templates are defined; rewake workers --help says where"
	}
	return "available: " + strings.Join(s.Names(), ", ")
}

// Lookup returns one alias and where it was read from.
func (s *Set) Lookup(name string) (Alias, string, bool) {
	value, ok := s.aliases[name]
	return value, s.sources[name], ok
}

// BriefsDir is the folder the project's [briefs] dir names, made absolute
// against the project's directory working, or empty when it sets none.
func (s *Set) BriefsDir(working string) string {
	if s.briefsDir == "" {
		return ""
	}
	if filepath.IsAbs(s.briefsDir) {
		return filepath.Clean(s.briefsDir)
	}
	return filepath.Join(working, s.briefsDir)
}

// SkillBarred answers the file whose [skills] never names the skill, empty
// when none does: such a skill is never copied into a brief
// (docs/briefs.md#skills).
func (s *Set) SkillBarred(name string) string {
	return s.skillsNever[name]
}
