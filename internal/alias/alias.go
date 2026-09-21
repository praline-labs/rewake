// Package alias turns a short name into the arguments of a launch.
//
// A launch that states a role, a name, a model and a configuration override is
// long enough that people stop typing it and start approximating it, which is
// how a session comes up as something other than what was meant. An alias is
// the opposite of a guess: one name, one recorded set of arguments.
package alias

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/iiiokojiadbi/rewake/internal/harness"
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

// Alias is one name's worth of launch.
//
// Three fields rather than one list, because the three are not interchangeable:
// what rewake reads, which harness to start, and what that harness gets. A
// single flat list would have to be cut at the harness name to know which flag
// belongs to whom, and a person editing the file would have to know that rule.
//
// Lists rather than one string, throughout. A string would have to be split
// into words, and splitting words means quoting rules — the part of this that
// has broken twice before.
type Alias struct {
	// Harness is the launch command, by its id: codex, claude.
	Harness string `toml:"harness"`
	// Rewake are flags read by rewake itself, such as --write or --name.
	Rewake []string `toml:"rewake"`
	// Args are handed to the harness untouched.
	Args []string `toml:"args"`
}

// Help is the one line that tells a person aliases exist, printed with every
// launch command's help. Somebody who has to read the documentation to learn
// that a feature exists will not find it.
const Help = "A launch can be given a short name: put it in ~/.config/" + userDir + "/" + userFile +
	" or in " + projectFile + " in the working directory, as [alias.<name>] with harness = \"<id>\", " +
	"rewake = [flags rewake reads] and args = [arguments for the harness]. " +
	"Then `rewake <name>` launches it: rewake's own flags go before that name, the harness's after it, " +
	"and a flag you type replaces the one the alias carries."

// Set is what this process can expand.
type Set struct {
	aliases map[string]Alias
	sources map[string]string
	// fromProject marks the aliases read from a file in the working directory,
	// which may decide less than the user's own file.
	fromProject map[string]bool
	// Notes are what to tell the person: a file that could not be read, an
	// alias that makes no sense. They are printed, not swallowed — an alias
	// silently absent is a launch that silently runs something else.
	Notes []string
}

// Load reads the user file and then the project file.
func Load() *Set {
	set := newSet()
	if home, err := os.UserHomeDir(); err == nil {
		set.read(filepath.Join(home, ".config", userDir, userFile), "the user alias file", true)
	}
	if working, err := os.Getwd(); err == nil {
		// This directory only, like the settings file: a parent directory does
		// not get to decide how a session launches.
		set.read(filepath.Join(working, projectFile), "the project alias file", false)
	}
	return set
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

// Expand replaces an alias in argv with what it stands for. argv without an
// alias comes back unchanged, and so does an empty one.
//
// A flag typed on the line replaces the alias's copy of it — but only for the
// flags the harness takes at most once, which the harness itself names through
// singleUse. Ordering alone would not do: Codex refuses to parse a repeated
// --model rather than taking the last one, so an alias naming a model plus a
// model typed on the line would end the launch complaining about a flag the
// person wrote once.
//
// Everything else is appended, and that is the safe default rather than a gap.
// A repeated --add-dir is how a second directory is added; a repeated -c key
// takes its last value when it is a plain one but merges when the setting is
// structured, so dropping the earlier one can remove part of a setting the
// later one does not restate. Replacing either would take away something
// nobody asked to remove.
func (s *Set) Expand(argv []string, knownHarnesses []string, takesValue func(string) bool, projectMayNotSet []string, singleUse func(harness string) []harness.Flag) ([]string, error) {
	position := commandWord(argv, takesValue)
	if position < 0 {
		return argv, nil
	}
	name := argv[position]
	value, source, ok := s.Lookup(name)
	if !ok {
		return argv, nil
	}
	if err := value.check(name, source, knownHarnesses, takesValue); err != nil {
		return nil, err
	}
	if s.fromProject[name] {
		for _, flag := range value.Rewake {
			if contains(projectMayNotSet, strings.TrimPrefix(flag, "--")) {
				// A file in whatever directory somebody happens to be in must
				// not choose the role of the session launching there. The role
				// outranks the room: a main sees every session's telemetry and
				// may ask for repository access. Same boundary as the settings
				// file's closed list of names, and for the same reason.
				return nil, fmt.Errorf("alias %q in %s sets %s; a role belongs in the user alias file, not in one carried by a directory",
					name, source, flag)
			}
		}
	}

	var flags []harness.Flag
	if singleUse != nil {
		flags = singleUse(value.Harness)
	}
	typedBefore, typedAfter := argv[:position], argv[position+1:]
	expanded := make([]string, 0, len(argv)+len(value.Rewake)+len(value.Args)+1)
	// rewake's own flags are all single-use, and rewake knows which of them
	// take a value; the harness half knows neither, so it is given the list the
	// harness published and treats a following non-flag token as the value.
	expanded = append(expanded, replaced(value.Rewake, typedBefore, rewakeFlags(takesValue, value.Rewake, typedBefore))...)
	expanded = append(expanded, typedBefore...)
	expanded = append(expanded, value.Harness)
	expanded = append(expanded, replaced(value.Args, typedAfter, flags)...)
	expanded = append(expanded, typedAfter...)
	return expanded, nil
}

// rewakeFlags describes rewake's own flags the way a harness describes its
// own: every flag it takes is single-use, and whether one carries a value is
// something rewake knows from its command table.
func rewakeFlags(takesValue func(string) bool, lists ...[]string) []harness.Flag {
	var flags []harness.Flag
	named := map[string]bool{}
	for _, argument := range concat(lists...) {
		if !strings.HasPrefix(argument, "--") || argument == "--" {
			continue
		}
		name, _, _ := strings.Cut(argument, "=")
		if named[name] {
			continue
		}
		named[name] = true
		flags = append(flags, harness.Flag{
			Spellings:  []string{name},
			TakesValue: takesValue != nil && takesValue(strings.TrimPrefix(name, "--")),
		})
	}
	return flags
}

// commandWord is where the command or alias stands in argv, or -1.
//
// It has to step over the value of a flag that takes one: in
// `rewake --name mine wcodex`, the first word that is not a flag is "mine",
// and treating that as the command would leave the alias unexpanded and the
// launch subtly different from what was asked for. Which flags take a value is
// the caller's knowledge, not this package's.
func commandWord(argv []string, takesValue func(string) bool) int {
	for i := 0; i < len(argv); i++ {
		token := argv[i]
		if token == "--" {
			// Nothing after the terminator is a command word either; rewake
			// refuses such a line on its own, and expanding an alias out of it
			// first would only change which refusal the person sees.
			return -1
		}
		if !strings.HasPrefix(token, "-") {
			return i
		}
		name := strings.TrimPrefix(token, "--")
		if strings.Contains(name, "=") {
			// --flag=value carries its value with it.
			continue
		}
		if takesValue != nil && takesValue(name) {
			i++
		}
	}
	return -1
}

// replaced drops each argument of the alias whose parameter was named on the
// line, along with the value belonging to it.
//
// The terminator is read on the command being built, not on each list on its
// own. "--" ends the flags of everything that follows it there: a terminator
// the person typed ends their own arguments, and one the alias carries ends
// both its own tail and everything the line appends behind it. Either way a
// word of a prompt must never replace a setting — which is what happened when
// each half was checked separately.
func replaced(arguments, typed []string, flags []harness.Flag) []string {
	visible := harness.BeforeTerminator(arguments)
	if len(visible) != len(arguments) {
		// The alias itself ends the flags. Everything the line adds lands
		// behind that terminator in the command being built, so all of it is
		// input for the harness — and a word of a prompt must not replace a
		// setting. Nothing is dropped here at all.
		return arguments
	}
	named := map[string]bool{}
	for _, name := range parameters(harness.BeforeTerminator(typed), flags) {
		named[name] = true
	}
	kept := make([]string, 0, len(arguments))
	for i := 0; i < len(visible); i++ {
		name, width := parameterAt(visible, i, flags)
		if name != "" && named[name] {
			i += width - 1
			continue
		}
		kept = append(kept, visible[i:i+width]...)
		i += width - 1
	}
	return kept
}

// parameters names every parameter these arguments set.
func parameters(arguments []string, flags []harness.Flag) []string {
	var names []string
	for i := 0; i < len(arguments); i++ {
		name, width := parameterAt(arguments, i, flags)
		if name != "" {
			names = append(names, name)
		}
		i += width - 1
	}
	return names
}

// parameterAt answers which parameter the argument at index sets, and how many
// arguments it occupies. A flag that is not in the list answers "" — it may be
// repeated for all we know, so it is left alone.
//
// The shapes of a flag are read by harness.MatchFlag, the one place that knows
// them: --flag value, --flag=value, -f value, -f=value and -fvalue. Each of
// those has cost us a defect somewhere else already.
func parameterAt(arguments []string, index int, flags []harness.Flag) (string, int) {
	arg := arguments[index]
	if !strings.HasPrefix(arg, "-") || arg == "-" || arg == "--" {
		return "", 1
	}
	for _, flag := range flags {
		for _, spelling := range flag.Spellings {
			_, separate, ok := harness.MatchFlag(arg, spelling)
			if !ok {
				continue
			}
			width := 1
			if separate && flag.TakesValue && index+1 < len(arguments) {
				// The value is the next argument — unless nothing follows,
				// which is a malformed line the harness will refuse anyway.
				width = 2
			}
			return flag.Spellings[0], width
		}
	}
	return "", 1
}

// check refuses an alias that would expand into something unusable, and says
// what it expanded to. A launch that fails three layers down, complaining
// about a flag the person never typed, is worse than no alias at all.
func (a Alias) check(name, source string, knownHarnesses []string, takesValue func(string) bool) error {
	if a.Harness == "" {
		return fmt.Errorf("alias %q in %s names no harness; add harness = \"<name>\"", name, source)
	}
	if len(knownHarnesses) > 0 && !contains(knownHarnesses, a.Harness) {
		return fmt.Errorf("alias %q in %s launches %q, which is not a harness; known: %s",
			name, source, a.Harness, strings.Join(knownHarnesses, ", "))
	}
	// An alias names arguments to rewake, and nothing else. Without this a list
	// could begin with a command word and turn `rewake <name>` into some other
	// command entirely — the same reason the settings file has no
	// substitutions. A flag's own value is not a command word, so the walk
	// steps over it the way the argument parser does.
	for i := 0; i < len(a.Rewake); i++ {
		flag := a.Rewake[i]
		if !strings.HasPrefix(flag, "--") || flag == "--" {
			return fmt.Errorf("alias %q in %s has %q in rewake, which only takes flags like --write or --name", name, source, flag)
		}
		body := strings.TrimPrefix(flag, "--")
		if strings.Contains(body, "=") {
			continue
		}
		if takesValue != nil && takesValue(body) {
			if i+1 >= len(a.Rewake) {
				return fmt.Errorf("alias %q in %s ends with %s, which needs a value", name, source, flag)
			}
			i++
		}
	}
	for _, argument := range a.Args {
		if argument == "" {
			return fmt.Errorf("alias %q in %s has an empty argument", name, source)
		}
	}
	return nil
}

func newSet() *Set {
	return &Set{aliases: map[string]Alias{}, sources: map[string]string{}, fromProject: map[string]bool{}}
}

// read parses one file into the set. user says whether this is the file in the
// home directory, which may set more and is held to stricter permissions.
func (s *Set) read(path, description string, user bool) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.Notes = append(s.Notes, "not reading "+description+": "+err.Error())
		return
	}
	if !info.Mode().IsRegular() {
		// A named pipe here would block the launch for ever at open, with no
		// sign of why — the same hazard the settings file refuses.
		s.Notes = append(s.Notes, fmt.Sprintf("not reading %s at %s: it is not an ordinary file (%s)", description, path, info.Mode().Type()))
		return
	}
	if user && info.Mode().Perm()&0o077 != 0 {
		// Only the file in the home directory, where the convention is 0600.
		// The stake here is higher than for settings: that file names a model,
		// this one can name a role, and a role decides what a session may see
		// and ask for.
		s.Notes = append(s.Notes, fmt.Sprintf("%s at %s is writable or readable by others (%#o); the convention for it is 0600", description, path, info.Mode().Perm()))
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		s.Notes = append(s.Notes, "not reading "+description+": "+err.Error())
		return
	}
	var file struct {
		Alias map[string]Alias `toml:"alias"`
	}
	if err := toml.Unmarshal(raw, &file); err != nil {
		// Named, not skipped: a file that is there and unreadable is a
		// mistake somebody wants to hear about.
		s.Notes = append(s.Notes, fmt.Sprintf("%s at %s could not be read: %v", description, path, err))
		return
	}
	for name, value := range file.Alias {
		s.aliases[name] = value
		s.sources[name] = description
		s.fromProject[name] = !user
	}
}

// concat joins argument lists without touching either.
func concat(lists ...[]string) []string {
	var all []string
	for _, list := range lists {
		all = append(all, list...)
	}
	return all
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
