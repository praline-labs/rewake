package alias

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

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
