/*
Package workmode holds a session's one runtime work mode: the text that tells
it how to work on the task at hand, an optional model and thinking level per
mode, and the selection of the mode a launch starts in (docs/work-modes.md).

A mode is a Markdown file, <name>.md, opening with a TOML front matter between
two +++ lines that holds its one-line description; the rest is the text, and it
is the complete behavioral section a session reads: it replaces the previous
one, never adds to it. rewake ships advisor, build, review and process in
builtin/; the person's ~/.config/rewake/modes/ and a project's .rewake/modes/
replace one of the same name.
*/
package workmode

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// The modes rewake ships, by the name each answers to. They are the four
// disciplines a worker is raised for; nothing else is built in, and no plan
// mode exists.
const (
	Advisor = "advisor"
	Build   = "build"
	Review  = "review"
	Process = "process"
)

// builtInOrder is the cycle's order, the built-ins first and always in this
// order: an overridden built-in keeps its place, a custom mode follows them.
var builtInOrder = []string{Review, Advisor, Build, Process}

// Where a mode was read from, weakest first.
const (
	BuiltIn = "built-in"
	User    = "user"
	Project = "project"
)

// MaxSize bounds a mode's file. The text joins the session's briefing, which
// travels in one argument of the harness's launch beside the role's text and a
// person's instructions; a mode is a page, not a manual.
const MaxSize = 32 << 10

// Mode is one runtime work mode, its complete behavioral text included.
type Mode struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Text is what the session is told, after the front matter.
	Text string `json:"-"`
	// Source is BuiltIn, User or Project; Path the file, empty for a built-in.
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
}

// name is what a mode may be called: it may name a directory of the worker's
// memory, so as a template is.
var name = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidName says whether a mode may be called this.
func ValidName(value string) bool { return name.MatchString(value) }

// NoPlan is the refusal of the plan mode there is none of: rewake's modes are
// behavioral texts, not plan mode's read-only planning (the owner's
// decision, November 3, 2026).
const NoPlan = "there is no plan mode: a session's runtime work mode is one of rewake's or a mode file of the person's or the project's own (docs/work-modes.md)"

const fence = "+++"

// Parse reads one mode's file. A file that is not what it should be is
// refused, not skipped: a session told nothing, or half of a text, is worse
// than a refusal at space open.
func Parse(modeName string, raw []byte) (Mode, error) {
	if modeName == "plan" {
		return Mode{}, errors.New(NoPlan)
	}
	if !ValidName(modeName) {
		return Mode{}, fmt.Errorf("%q cannot name a mode: letters, digits, dot, dash or underscore, not starting with a dot or dash", modeName)
	}
	if len(raw) > MaxSize {
		return Mode{}, fmt.Errorf("mode %q is %d bytes, over the %d KiB a mode may hold", modeName, len(raw), MaxSize>>10)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	head, body, ok := strings.Cut(strings.TrimPrefix(text, fence+"\n"), "\n"+fence+"\n")
	if !strings.HasPrefix(text, fence+"\n") || !ok {
		return Mode{}, fmt.Errorf("mode %q does not open with a front matter between two %s lines holding its description", modeName, fence)
	}
	var front struct {
		Description string `toml:"description"`
	}
	decoder := toml.NewDecoder(bytes.NewReader([]byte(head)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&front); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return Mode{}, fmt.Errorf("mode %q has a field a mode does not: %s; it has description only", modeName, strings.TrimSpace(strict.String()))
		}
		return Mode{}, fmt.Errorf("mode %q has a front matter that is not TOML: %w", modeName, err)
	}
	description := strings.TrimSpace(front.Description)
	if description == "" || strings.ContainsAny(description, "\r\n") {
		return Mode{}, fmt.Errorf("mode %q needs description = \"<one line>\" in its front matter", modeName)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Mode{}, fmt.Errorf("mode %q has no text after its front matter", modeName)
	}
	return Mode{Name: modeName, Description: description, Text: body}, nil
}

//go:embed builtin/*.md
var builtin embed.FS

// BuiltIns are the modes rewake ships, by name.
func BuiltIns() map[string]Mode {
	out := map[string]Mode{}
	entries, _ := fs.ReadDir(builtin, "builtin")
	for _, entry := range entries {
		modeName, ok := strings.CutSuffix(entry.Name(), ".md")
		if !ok {
			continue
		}
		raw, _ := fs.ReadFile(builtin, "builtin/"+entry.Name())
		value, err := Parse(modeName, raw)
		if err != nil {
			// A test parses every built-in; a broken one is a build that
			// must not ship.
			panic(err)
		}
		value.Source = BuiltIn
		out[modeName] = value
	}
	return out
}

// Names are the modes of a set, in order.
func Names(modes map[string]Mode) []string {
	out := make([]string, 0, len(modes))
	for modeName := range modes {
		out = append(out, modeName)
	}
	sort.Strings(out)
	return out
}

// Inventory is a catalog in cycle order: the built-ins as builtInOrder puts
// them, whether rewake ships one or a file replaced it, then the names that
// only the person or the project defines, sorted.
func Inventory(modes map[string]Mode) []Mode {
	out := make([]Mode, 0, len(modes))
	for _, modeName := range builtInOrder {
		if value, ok := modes[modeName]; ok {
			out = append(out, value)
		}
	}
	var custom []string
	for modeName := range modes {
		if !isBuiltIn(modeName) {
			custom = append(custom, modeName)
		}
	}
	sort.Strings(custom)
	for _, modeName := range custom {
		out = append(out, modes[modeName])
	}
	return out
}

func isBuiltIn(modeName string) bool {
	for _, known := range builtInOrder {
		if known == modeName {
			return true
		}
	}
	return false
}

// Find answers the catalog's mode of a name.
func Find(catalog []Mode, modeName string) (Mode, bool) {
	for _, value := range catalog {
		if value.Name == modeName {
			return value, true
		}
	}
	return Mode{}, false
}

// Next is the shared cycle for native and face requests: the catalog's
// order, wrapping to its first mode.
func Next(current string, catalog []Mode) string {
	if len(catalog) == 0 {
		return current
	}
	for index, value := range catalog {
		if value.Name == current {
			return catalog[(index+1)%len(catalog)].Name
		}
	}
	return catalog[0].Name
}
