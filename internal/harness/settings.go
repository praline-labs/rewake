package harness

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Where launch defaults come from, in the order that decides them.
//
// A setting you have to load by hand before every launch is not a setting, it
// is a ritual — and the one time it is forgotten, a session comes up on
// something else without saying so. So rewake reads the files itself.
//
// Strongest first:
//
//  1. a flag on the launch — handled by the caller, not here;
//  2. a variable already in the environment;
//  3. the project file, for one repository;
//  4. the user file, for everything this person launches.
//
// The rule behind the order is that the closer a choice is made to the moment
// of launching, the more it means. A variable already set is never replaced by
// a file.

const (
	// userSettingsDir follows the convention for this kind of file:
	// ~/.config/<service>/, one KEY=VALUE per line, 0600 on the file and 0700
	// on the directory.
	userSettingsDir = "rewake"
	// userSettingsFile is "settings" rather than "credentials" because that is
	// what it holds: choices about launching, not secrets.
	userSettingsFile = "settings"
	// projectSettingsFile sits in the working directory for settings that
	// belong to one repository. The extension says what the format is, and
	// keeps the name free if a directory is ever wanted.
	projectSettingsFile = ".rewake.env"
)

// readable is the closed set of names a file may set. A prefix alone would
// leave the boundary in the hands of whoever calls Lookup — and the state
// directory and the room are spelled with the same prefix, so a file found in
// whatever directory somebody is in could end up steering another session's
// state. What a file may decide is listed here, not inferred from a name.
var readable = map[string]bool{
	"REWAKE_CLAUDE_MODEL":  true,
	"REWAKE_CLAUDE_EFFORT": true,
}

// SettingsHelp is the one line that tells a person these files exist, printed
// with every launch command's help. Somebody who has to read the documentation
// to learn where a setting lives will not find it.
const SettingsHelp = "Launch defaults come as KEY=VALUE lines from ~/.config/" + userSettingsDir + "/" + userSettingsFile +
	" and " + projectSettingsFile + " in the working directory: REWAKE_CLAUDE_MODEL, " +
	"REWAKE_CLAUDE_EFFORT. The environment beats both files, an empty variable turns a " +
	"default off for the launch, and a typed flag beats everything."

// Settings are the launch defaults available to this process.
type Settings struct {
	values  map[string]string
	sources map[string]string
	// Notes are what to tell the person at launch: a file that could not be
	// read, a line that made no sense, permissions wider than the convention.
	Notes []string
}

// LoadSettings reads the user file and then the project file. A file that is
// absent is not a problem; a file that is present and unreadable is, and says
// so rather than leaving a person to wonder.
func LoadSettings() *Settings {
	settings := &Settings{values: map[string]string{}, sources: map[string]string{}}
	if home, err := os.UserHomeDir(); err == nil {
		settings.read(filepath.Join(home, ".config", userSettingsDir, userSettingsFile), "the user settings file", true)
	}
	if working, err := os.Getwd(); err == nil {
		// This directory only. Walking up would let a parent directory decide
		// how a session launches, which is a surprise nobody asked for.
		settings.read(filepath.Join(working, projectSettingsFile), "the project settings file", false)
	}
	return settings
}

// Lookup returns a value and where it came from. A variable already in the
// environment wins over any file; among files the project one wins, because it
// is read second.
func (s *Settings) Lookup(key string) (string, string, bool) {
	if value, set := os.LookupEnv(key); set {
		// An empty variable is an answer, not an absence: it says "no default
		// for this launch", and the files are not consulted behind it.
		// Somebody who exports an empty value for one command has made a
		// choice as explicit as any other, and falling through to a file
		// would overrule it.
		value = strings.TrimSpace(value)
		return value, "the environment", value != ""
	}
	value, ok := s.values[key]
	return value, s.sources[key], ok
}

// read parses one file into the settings.
func (s *Settings) read(path, description string, strict bool) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.Notes = append(s.Notes, "not reading "+description+": "+err.Error())
		return
	}
	if !info.Mode().IsRegular() {
		// A named pipe here would block the launch for ever at open: nobody
		// is writing to it, and a session that hangs before it starts gives no
		// sign of why. Anything that is not an ordinary file is refused before
		// it can be opened at all.
		s.Notes = append(s.Notes, fmt.Sprintf("not reading %s at %s: it is not an ordinary file (%s)", description, path, info.Mode().Type()))
		return
	}
	if strict && info.Mode().Perm()&0o077 != 0 {
		// Only for the file in the home directory, where the convention is
		// 0600 and where more than a model name may end up. A project file
		// created under the usual umask is 0644, and warning about that on
		// every launch in every repository would teach people to skip these
		// notes — including the ones about a file that could not be read.
		s.Notes = append(s.Notes, fmt.Sprintf("%s at %s is readable by others (%#o); the convention for it is 0600", description, path, info.Mode().Perm()))
	}
	file, err := os.Open(path)
	if err != nil {
		s.Notes = append(s.Notes, "not reading "+description+": "+err.Error())
		return
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for line := 1; scanner.Scan(); line++ {
		key, value, note := parseSetting(scanner.Text())
		if note != "" {
			s.Notes = append(s.Notes, fmt.Sprintf("%s, line %d: %s", description, line, note))
			continue
		}
		if key == "" {
			continue
		}
		s.values[key] = value
		s.sources[key] = description
	}
	if err := scanner.Err(); err != nil {
		s.Notes = append(s.Notes, "stopped reading "+description+": "+err.Error())
	}
}

// parseSetting reads one line. The format is deliberately the smallest thing
// that works: KEY=VALUE, blank lines and # comments skipped, surrounding
// whitespace trimmed, one matching pair of quotes removed.
//
// Nothing resembling a shell: no $VAR expansion, no command substitution, no
// line continuation. This is a settings file, and a file that can run things
// is a different and much larger promise.
func parseSetting(raw string) (string, string, string) {
	line := strings.TrimSpace(raw)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", ""
	}
	// "export KEY=VALUE" is what somebody used to such files will write, and
	// it is the likeliest way to lose a setting in silence: the line would
	// otherwise fall into the same branch as a name that is not ours.
	// Recognizing a common form is not guessing at intent.
	line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", "expected KEY=VALUE"
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", "a setting with no name"
	}
	if !readable[key] {
		// Silently, and on purpose. Reading arbitrary names out of a file
		// found in whatever directory somebody happens to be in would make
		// that file a way to change the environment of the harness being
		// launched. Only the settings listed above are taken from it, and the
		// file may hold anything else for its own reasons.
		return "", "", ""
	}
	return key, unquote(trailingComment(strings.TrimSpace(value))), ""
}

// trailingComment removes a comment at the end of a value: a '#' preceded by
// whitespace, outside quotes.
//
// Both conditions matter. A '#' with no space before it belongs to the value,
// because a value may contain one. A '#' inside quotes belongs to the value
// too — cutting there would silently shorten it. And a value that is empty
// before the comment stays empty rather than becoming the comment.
func trailingComment(value string) string {
	var quote byte
	for i := 0; i < len(value); i++ {
		symbol := value[i]
		switch {
		case quote != 0:
			if symbol == quote {
				quote = 0
			}
		case symbol == '"' || symbol == '\'':
			quote = symbol
		case symbol == '#' && (i == 0 || value[i-1] == ' ' || value[i-1] == '\t'):
			return strings.TrimSpace(value[:i])
		}
	}
	return value
}

// unquote removes one matching pair of quotes, and nothing else.
func unquote(value string) string {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if first == last && (first == '"' || first == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}
