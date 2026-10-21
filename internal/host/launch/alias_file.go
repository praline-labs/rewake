package launch

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"

	"github.com/praline-labs/rewake/internal/core/worktype"
)

// Load reads the user's files and then the project's: the alias files, then
// the types beside each.
func Load() *Set {
	set := newSet()
	if home, err := os.UserHomeDir(); err == nil {
		dir := filepath.Join(home, ".config", userDir)
		set.read(filepath.Join(dir, userFile), "the user alias file", true)
		set.readTypes(filepath.Join(dir, typesDir), worktype.User)
	}
	if working, err := os.Getwd(); err == nil {
		// This directory only, like the settings file: a parent directory does
		// not get to decide how a session launches.
		set.refuseMoved(filepath.Join(working, movedProjectFile))
		dir := filepath.Join(working, projectDir)
		set.read(filepath.Join(dir, userFile), "the project alias file", false)
		set.readTypes(filepath.Join(dir, typesDir), worktype.Project)
	}
	return set
}

func newSet() *Set {
	return &Set{
		aliases: map[string]Alias{}, sources: map[string]string{}, fromProject: map[string]bool{},
		types: map[string]worktype.Type{}, off: map[string]string{},
	}
}

// read parses one file into the set. user says whether this is the file in the
// home directory, which may set more and is held to stricter permissions.
func (s *Set) read(path, description string, user bool) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.unreadable("not reading " + description + ": " + err.Error())
		return
	}
	if !info.Mode().IsRegular() {
		// A named pipe here would block the launch for ever at open, with no
		// sign of why — the same hazard the settings file refuses.
		s.unreadable(fmt.Sprintf("not reading %s at %s: it is not an ordinary file (%s)", description, path, info.Mode().Type()))
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
		s.unreadable("not reading " + description + ": " + err.Error())
		return
	}
	var file struct {
		Alias map[string]Alias `toml:"alias"`
		// Types turns built-in types off: the person's or the project's
		// choice of which of rewake's texts main is offered.
		Types struct {
			Off []string `toml:"off"`
		} `toml:"types"`
	}
	decoder := toml.NewDecoder(bytes.NewReader(raw))
	// A field rewake does not know is refused, not skipped: a silently
	// ignored field is a worker that starts as something nobody wrote.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			s.unreadable(unknownFields(description, path, strict))
			return
		}
		// Named, not skipped: a file that is there and unreadable is a
		// mistake somebody wants to hear about.
		var decode *toml.DecodeError
		where := path
		if errors.As(err, &decode) {
			line, column := decode.Position()
			where = fmt.Sprintf("%s:%d:%d", path, line, column)
		}
		s.unreadable(fmt.Sprintf("%s at %s could not be read: %v", description, where, err))
		return
	}
	if err := s.turnOff(file.Types.Off, description, path); err != nil {
		s.unreadable(err.Error())
		return
	}
	for name, value := range file.Alias {
		s.aliases[name] = value
		s.sources[name] = description
		s.fromProject[name] = !user
	}
}

// removedFields are the fields aliases lost on October 19, 2026, when they
// became main's catalog of workers, each with what replaced it. A file
// written before says which, so it is rewritten once rather than guessed at.
var removedFields = map[string]string{
	"rewake":       `the worker's role is role = "write" or "general"; the space names the worker and its room`,
	"main":         "main is no alias: it is started with rewake <harness>",
	"launch.apply": "model, effort and context always apply; flags that should not go in args",
}

// unknownFields names every field the file has that an alias does not, the
// removed ones with what took their place.
func unknownFields(description, path string, strict *toml.StrictMissingError) string {
	var removed, unknown []string
	for _, field := range strict.Errors {
		key := field.Key()
		dotted := strings.Join(key, ".")
		line, _ := field.Position()
		// A key here is alias.<name>.<field>; the field is what was removed.
		why, gone := "", false
		if len(key) > 2 && key[0] == "alias" {
			why, gone = removedFields[strings.Join(key[2:], ".")]
		}
		if gone {
			removed = append(removed, fmt.Sprintf("%s (line %d): %s", dotted, line, why))
		} else {
			unknown = append(unknown, fmt.Sprintf("%s (line %d)", dotted, line))
		}
	}
	var parts []string
	if len(removed) > 0 {
		parts = append(parts, "fields removed when aliases became main's catalog of workers (October 19, 2026): "+strings.Join(removed, "; "))
	}
	if len(unknown) > 0 {
		parts = append(parts, "fields an alias does not have: "+strings.Join(unknown, ", "))
	}
	return fmt.Sprintf("%s at %s has %s; rewrite it as rewake workers --help shows", description, path, strings.Join(parts, "; and "))
}

// Err refuses a catalog that could silently omit workers from an unreadable
// file.
func (s *Set) Err() error {
	if len(s.problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(s.problems, "; "))
}

func (s *Set) unreadable(message string) {
	s.problems = append(s.problems, message)
}
