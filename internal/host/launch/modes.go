package launch

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/praline-labs/rewake/internal/core/workmode"
)

// The modes main may raise a worker for (docs/work-modes.md): rewake's own
// advisor, build, review and process, the person's in ~/.config/rewake/modes/
// and the project's in .rewake/modes/, the nearest of one name winning. A
// file of a built-in's name replaces its text and keeps its place in the
// cycle; a mode is never turned off.

// readModes reads every <name>.md of a modes directory. A missing directory is
// no modes; a file that does not read refuses the catalog, as an alias file
// does, rather than leave a mode silently absent.
func (s *Set) readModes(dir, source string) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.unreadable(fmt.Sprintf("not reading the %s modes in %s: %v", source, dir, err))
		return
	}
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".md")
		if !ok || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			// A named pipe would block the read for ever, as for the alias file.
			s.unreadable(fmt.Sprintf("not reading the %s mode %s: it is not an ordinary file", source, path))
			continue
		}
		if info.Size() > workmode.MaxSize {
			s.unreadable(fmt.Sprintf("the %s mode %s is %d bytes, over the %d KiB a mode may hold", source, path, info.Size(), workmode.MaxSize>>10))
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			s.unreadable(fmt.Sprintf("not reading the %s mode %s: %v", source, path, err))
			continue
		}
		value, err := workmode.Parse(name, raw)
		if err != nil {
			s.unreadable(fmt.Sprintf("%v (%s)", err, path))
			continue
		}
		value.Source, value.Path = source, path
		s.modes[name] = value
	}
}

// noteModesDir skips the types directory of before this change with a
// migration notice, naming where its files go: refusing it blocked every
// launch in a project that still carried one, and reading it silently would
// start workers without the discipline their person wrote for them.
func (s *Set) noteModesDir(dir string) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	var names []string
	if err == nil {
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
	}
	files := "it holds " + strings.Join(names, ", ")
	if len(names) == 0 {
		files = "it is empty"
	}
	s.Migrations = append(s.Migrations, fmt.Sprintf("%s is no longer read and is skipped: a worker's kind of work is its runtime work mode, a file under %s; move each file there, keeping its front matter and text. code is now build and analysis is advisor, so a custom code.md or analysis.md needs a name of its own; review and process keep theirs. %s",
		dir, filepath.Join(filepath.Dir(dir), modesDir), files))
}

// refuseMoved refuses the project alias file of before October 21, 2026,
// naming where its aliases go now: read silently, or left unread, either
// would start workers other than the project says.
func (s *Set) refuseMoved(path string) {
	if _, err := os.Lstat(path); err == nil {
		s.unreadable(fmt.Sprintf("%s is no longer read: a project's aliases moved to %s, beside its modes in %s; move the file there",
			path, filepath.Join(filepath.Dir(path), projectDir, userFile), filepath.Join(filepath.Dir(path), projectDir, modesDir)))
	}
}

// ModeList is the mode catalog: the files read and the built-in modes they
// do not replace, by name.
func (s *Set) ModeList() []workmode.Mode {
	all := map[string]workmode.Mode{}
	for name, value := range workmode.BuiltIns() {
		all[name] = value
	}
	for name, value := range s.modes {
		all[name] = value
	}
	out := make([]workmode.Mode, 0, len(all))
	for _, name := range workmode.Names(all) {
		out = append(out, all[name])
	}
	return out
}

// Catalog is the same modes in the cycle's order: the built-ins as the
// design puts them, then the custom-only names sorted.
func (s *Set) Catalog() []workmode.Mode {
	all := map[string]workmode.Mode{}
	for name, value := range workmode.BuiltIns() {
		all[name] = value
	}
	for name, value := range s.modes {
		all[name] = value
	}
	return workmode.Inventory(all)
}

// Mode resolves one mode by name, the nearest first, refusing one that is not
// there.
func (s *Set) Mode(name string) (workmode.Mode, error) {
	if value, ok := s.modes[name]; ok {
		return value, nil
	}
	if value, ok := workmode.BuiltIns()[name]; ok {
		return value, nil
	}
	return workmode.Mode{}, fmt.Errorf("no mode %q; available: %s", name, s.modeNames())
}

func (s *Set) modeNames() string {
	var names []string
	for _, value := range s.ModeList() {
		names = append(names, value.Name)
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// modeKnown is the catalog as a map, for a configuration's names to be
// checked against.
func (s *Set) modeKnown() map[string]workmode.Mode {
	known := map[string]workmode.Mode{}
	for _, value := range s.ModeList() {
		known[value.Name] = value
	}
	return known
}

// checkModeNames refuses a start_mode or a [pi.modes.<name>] the catalog
// does not hold: a typo would otherwise apply nothing and start a mode the
// person meant to configure.
func (s *Set) checkModeNames() {
	known := s.modeKnown()
	if s.piSource != "" {
		if err := s.pi.Unknown(known); err != nil {
			s.unreadable(fmt.Sprintf("%s: [pi] %v", s.piSource, err))
		}
	}
	names := make([]string, 0, len(s.aliases))
	for name := range s.aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := s.aliases[name].Pi.Unknown(known); err != nil {
			s.unreadable(fmt.Sprintf("alias %q in %s: [alias.%s.pi] %v", name, s.paths[name], name, err))
		}
	}
	// A template an obsolete setting left out is held to the same catalog:
	// the type may leave it out, never make a typo in its mode harmless.
	for _, name := range s.OmittedNames() {
		def := s.omitted[name]
		if err := def.value.Pi.Unknown(known); err != nil {
			s.unreadable(fmt.Sprintf("alias %q in %s: [alias.%s.pi] %v", name, def.path, name, err))
		}
	}
}

// Modes gives a launch the merged global configuration and its selected
// template's overlay, in the catalog's cycle order. Templates themselves
// still replace one another whole.
func (s *Set) Modes(template string) workmode.Config {
	c := s.pi
	if value, _, ok := s.Lookup(template); ok {
		c = c.Overlay(value.Pi)
	}
	c.Catalog = s.Catalog()
	return c
}
