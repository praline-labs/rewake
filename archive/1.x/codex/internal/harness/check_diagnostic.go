package harness

import (
	"path/filepath"
	"strconv"
	"strings"
)

// What a diagnostic of the name check may show is a closed list
// (docs/mail-bridge-launch.md#the-refusal-and-what-may-be-shown): the
// program's base name and the check's label, an outcome class, a scope from
// the vocabulary below with the path of its file, and an argument's position
// and flag. Never a check's output, an argument's value or configuration
// content: they may carry secrets, and a parser's error quotes what it read.
// So these types take only those parts, and their text is built here alone.

// The scopes and layers a diagnostic may name.
const (
	ScopeUser     = "user"
	ScopeLocal    = "local"
	ScopeProject  = "project"
	ScopeManaged  = "managed"
	ScopeSession  = "session flags"
	ScopeDefaults = "packaged defaults"
	// ScopeUnnamed is a scope the harness named in words outside this list.
	ScopeUnnamed = "a scope rewake does not name"
)

// Where says where a server named rewake was found: a scope with the path
// of its file, or an argument by its flag and position.
type Where struct {
	Scope string
	Path  string
	// Flag and Position name an argument: "-c" argument 3. Key, for -c,
	// is cut to mcp_servers.rewake and never carries a value.
	Flag     string
	Position int
	Key      bool
}

func (w Where) String() string {
	if w.Flag != "" {
		text := w.Flag + " argument " + strconv.Itoa(w.Position)
		if w.Key {
			text += ", key mcp_servers.rewake"
		}
		return text
	}
	text := w.Scope
	if w.Path != "" {
		text += " scope, " + filepath.Clean(w.Path)
	} else if text != "" && text != ScopeSession && text != ScopeUnnamed {
		text += " scope"
	}
	return text
}

// NameTakenError refuses a launch whose person already has a server named
// rewake (rule 2).
type NameTakenError struct {
	Where Where
	// Started says the check itself started that server once, as the
	// harness's own check does for a configured server.
	Started bool
}

func (e *NameTakenError) Error() string {
	lines := []string{
		"rewake: not starting: you already have an MCP server named rewake (" + e.Where.String() + ").",
		"rewake adds its own server under that name for this launch, and the harness would",
		"merge the two or keep only one. Rename or remove your entry, then launch again;",
		"rewake changes none of your files.",
	}
	if e.Started {
		lines = append(lines, "Checking for it may have started that server once, as the harness's own check does.")
	}
	return strings.Join(lines, "\n")
}

// CheckFailedError refuses a launch whose check could not establish that
// the name is free: which check, its outcome class, and what to run by hand.
type CheckFailedError struct {
	Program, Label, Outcome string
	// Where, when set, is the argument that could not be read.
	Where *Where
	// Template is the command to run by hand, without the person's values.
	Template string
}

func (e *CheckFailedError) Error() string {
	subject := filepath.Base(e.Program) + " " + e.Label
	if e.Where != nil {
		subject = e.Where.String()
	}
	text := "rewake: not starting: could not establish that you have no MCP server named rewake: " + subject + ": " + e.Outcome + "."
	if e.Template != "" {
		text += "\nRun " + e.Template + " by hand to see it; rewake changes none of your files."
	}
	return text
}
