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

// configString reads a top-level string value.
func configString(home, key string) (string, error) {
	lines, err := readConfig(home)
	if err != nil {
		return "", err
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
			text, consumed := multiline(lines[index:], value)
			index += consumed
			return text, nil
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted, nil
		}
		// A form this reader does not recognise: say nothing rather than a guess.
		return "", nil
	}
	return "", nil
}

// configBool reads a boolean from the sandbox section.
func configBool(home, key string) bool {
	value, ok := sectionValue(home, key)
	if !ok {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	return err == nil && parsed
}

// configArray reads a flat array of strings from the sandbox section.
func configArray(home, key string) []string {
	value, ok := sectionValue(home, key)
	if !ok {
		return nil
	}
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		// A multi-line array, or something else; adding to what we cannot read
		// whole would drop the part we did not see.
		return nil
	}

	var out []string
	for _, item := range strings.Split(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]"), ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		unquoted, err := strconv.Unquote(item)
		if err != nil {
			return nil
		}
		out = append(out, unquoted)
	}
	return out
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

// keyValue splits a "key = value" line, ignoring comments and blank lines.
func keyValue(line string) (string, string, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	name, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}
	return strings.TrimSpace(name), strings.TrimSpace(value), true
}

// multiline reads a """ quoted block and returns it with the lines it consumed.
func multiline(lines []string, first string) (string, int) {
	body := strings.TrimPrefix(strings.TrimSpace(first), `"""`)
	if closing := strings.Index(body, `"""`); closing >= 0 {
		return body[:closing], 0
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
			return strings.Join(collected, "\n"), offset
		}
		collected = append(collected, line)
	}
	// No closing marker: the file is not what it claims to be, so nothing is
	// reported rather than half a value.
	return "", len(lines) - 1
}
