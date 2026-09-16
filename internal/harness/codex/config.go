package codex

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

/*
A very small reader for the three values rewake needs out of the Codex
configuration: the developer instructions it must not overwrite, and whether the
sandbox has been told to keep /tmp out of reach.

It is not a TOML parser and does not pretend to be one. It reads what it
recognises — a key at the top level or in [sandbox_workspace_write], a basic
string, a boolean, a flat array of strings — and answers "nothing" for anything
else. That bias is deliberate: rewake only ever adds to what it reads here, so
reading too little costs a briefing, while guessing wrong would replace a user's
configuration with a misparse.
*/

// sandboxSection holds the write permissions of the Codex sandbox.
const sandboxSection = "sandbox_workspace_write"

// configPath is the user's Codex configuration.
func configPath(home string) string { return filepath.Join(home, "config.toml") }

// readConfig returns the configuration lines, or nothing when there is no file.
func readConfig(home string) ([]string, error) {
	raw, err := os.ReadFile(configPath(home))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return strings.Split(string(raw), "\n"), nil
}

// Reading has three outcomes, and the third one is the point: a value that is
// there but not understood. Treating that as absent would be fine if rewake only
// added to what it reads, but it passes a replacing override — so an unread
// value must stop the override, not become an empty string.
type reading int

const (
	// missing means the key is not in the file.
	missing reading = iota
	// read means the value was understood.
	read
	// unreadable means the key is there in a form this reader does not parse.
	unreadable
)

// configString reads a top-level string value.
func configString(home, key string) (string, reading, error) {
	lines, err := readConfig(home)
	if err != nil {
		return "", missing, err
	}

	section := ""
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if next, ok := sectionOf(line); ok {
			section = next
			continue
		}
		name, value, ok := keyValue(line)
		if !ok || section != "" || name != key {
			continue
		}

		if strings.HasPrefix(value, `"""`) {
			text, consumed, ok := multiline(lines[index:], value)
			index += consumed
			if !ok {
				return "", unreadable, nil
			}
			return text, read, nil
		}
		if text, ok := basicString(value); ok {
			return text, read, nil
		}
		if text, ok := literalString(value); ok {
			return text, read, nil
		}
		return "", unreadable, nil
	}
	return "", missing, nil
}

// configBool reads a boolean from the sandbox section.
func configBool(home, key string) (bool, reading) {
	value, ok := sectionValue(home, key)
	if !ok {
		return false, missing
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	if err != nil {
		return false, unreadable
	}
	return parsed, read
}

// configArray reads a flat array of strings from the sandbox section. A comma
// inside a quoted path is not a separator, so the array is scanned rather than
// split.
func configArray(home, key string) ([]string, reading) {
	value, ok := sectionValue(home, key)
	if !ok {
		return nil, missing
	}
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		// A multi-line array, or something else: adding to what cannot be read
		// whole would drop the part that was not seen.
		return nil, unreadable
	}

	var out []string
	body := strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	for _, item := range splitTopLevel(body) {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		text, ok := basicString(item)
		if !ok {
			if text, ok = literalString(item); !ok {
				return nil, unreadable
			}
		}
		out = append(out, text)
	}
	return out, read
}

// splitTopLevel splits on commas that are not inside a quoted string.
func splitTopLevel(body string) []string {
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false

	for _, symbol := range body {
		switch {
		case escaped:
			escaped = false
		case quote == '"' && symbol == '\\':
			escaped = true
		case quote != 0:
			if symbol == quote {
				quote = 0
			}
		case symbol == '"' || symbol == '\'':
			quote = symbol
		case symbol == ',':
			parts = append(parts, current.String())
			current.Reset()
			continue
		}
		current.WriteRune(symbol)
	}
	return append(parts, current.String())
}

// basicString decodes a TOML basic string: "..." with escapes.
func basicString(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || !strings.HasPrefix(value, `"`) || !strings.HasSuffix(value, `"`) {
		return "", false
	}
	text, err := strconv.Unquote(value)
	if err != nil {
		return "", false
	}
	return text, true
}

// literalString decodes a TOML literal string: '...' with no escapes at all.
func literalString(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || !strings.HasPrefix(value, "'") || !strings.HasSuffix(value, "'") {
		return "", false
	}
	return value[1 : len(value)-1], true
}

// sectionValue reads a raw value from the sandbox section.
func sectionValue(home, key string) (string, bool) {
	lines, err := readConfig(home)
	if err != nil {
		return "", false
	}

	section := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if next, ok := sectionOf(line); ok {
			section = next
			continue
		}
		name, value, ok := keyValue(line)
		if ok && section == sandboxSection && name == key {
			return value, true
		}
	}
	return "", false
}

// sectionOf recognises a section header.
func sectionOf(line string) (string, bool) {
	if !strings.HasPrefix(line, "[") || !strings.HasSuffix(line, "]") {
		return "", false
	}
	return strings.Trim(line, "[]"), true
}

// keyValue splits a "key = value" line, ignoring comments and blank lines. A
// trailing comment is cut from the value — but only outside quotes, where a #
// is part of the text rather than the end of it.
func keyValue(line string) (string, string, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	name, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(name), strings.TrimSpace(stripComment(value)), true
}

// stripComment removes a trailing comment that starts outside a quoted string.
func stripComment(value string) string {
	var quote rune
	escaped := false
	for index, symbol := range value {
		switch {
		case escaped:
			escaped = false
		case quote == '"' && symbol == '\\':
			escaped = true
		case quote != 0:
			if symbol == quote {
				quote = 0
			}
		case symbol == '"' || symbol == '\'':
			quote = symbol
		case symbol == '#':
			return value[:index]
		}
	}
	return value
}

// multiline reads a """ quoted block and returns it with the lines it consumed.
// The third result says whether the block was closed at all.
func multiline(lines []string, first string) (string, int, bool) {
	body := strings.TrimPrefix(strings.TrimSpace(first), `"""`)
	if closing := strings.Index(body, `"""`); closing >= 0 {
		return body[:closing], 0, true
	}

	collected := []string{}
	if body != "" {
		collected = append(collected, body)
	}
	for offset := 1; offset < len(lines); offset++ {
		line := lines[offset]
		if closing := strings.Index(line, `"""`); closing >= 0 {
			if trimmed := line[:closing]; trimmed != "" {
				collected = append(collected, trimmed)
			}
			return strings.Join(collected, "\n"), offset, true
		}
		collected = append(collected, line)
	}
	// No closing marker: the file is not what it claims to be, so nothing is
	// reported rather than half a value.
	return "", len(lines) - 1, false
}
