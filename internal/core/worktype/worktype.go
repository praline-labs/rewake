/*
Package worktype holds the kinds of work a worker can be raised for: a text,
independent of the model and the harness, that tells the worker how to work on
one kind of task (docs/worker-types.md).

A type is a Markdown file, <name>.md, opening with a TOML front matter
between two +++ lines that holds its one-line description; the rest is the
text, sent as written after the role's briefing. rewake ships the first set
in builtin/; the person's ~/.config/rewake/types/ and a project's
.rewake/types/ replace one of the same name.
*/
package worktype

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

// Where a type was read from, weakest first.
const (
	BuiltIn = "built-in"
	User    = "user"
	Project = "project"
)

// MaxSize bounds a type's file. The text joins the briefing in one argument
// of the harness's launch, which also carries the role's text and a person's
// instructions; a type is a page, not a manual.
const MaxSize = 32 << 10

// Type is one kind of work.
type Type struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Text is what the worker is told, after the front matter.
	Text string `json:"-"`
	// Source is BuiltIn, User or Project; Path the file, empty for a built-in.
	Source string `json:"source"`
	Path   string `json:"path,omitempty"`
}

// name is what a type may be called: it may name a directory of the
// worker's memory, so as a template is.
var name = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// ValidName says whether a type may be called this.
func ValidName(value string) bool { return name.MatchString(value) }

const fence = "+++"

// Parse reads one type's file. A file that is not what it should be is
// refused, not skipped: a worker told nothing, or half of a text, is worse
// than a refusal at space open.
func Parse(typeName string, raw []byte) (Type, error) {
	if !ValidName(typeName) {
		return Type{}, fmt.Errorf("%q cannot name a type: letters, digits, dot, dash or underscore, not starting with a dot or dash", typeName)
	}
	if len(raw) > MaxSize {
		return Type{}, fmt.Errorf("type %q is %d bytes, over the %d KiB a type may hold", typeName, len(raw), MaxSize>>10)
	}
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	head, body, ok := strings.Cut(strings.TrimPrefix(text, fence+"\n"), "\n"+fence+"\n")
	if !strings.HasPrefix(text, fence+"\n") || !ok {
		return Type{}, fmt.Errorf("type %q does not open with a front matter between two %s lines holding its description", typeName, fence)
	}
	var front struct {
		Description string `toml:"description"`
	}
	decoder := toml.NewDecoder(bytes.NewReader([]byte(head)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&front); err != nil {
		var strict *toml.StrictMissingError
		if errors.As(err, &strict) {
			return Type{}, fmt.Errorf("type %q has a field a type does not: %s; it has description only", typeName, strings.TrimSpace(strict.String()))
		}
		return Type{}, fmt.Errorf("type %q has a front matter that is not TOML: %w", typeName, err)
	}
	description := strings.TrimSpace(front.Description)
	if description == "" || strings.ContainsAny(description, "\r\n") {
		return Type{}, fmt.Errorf("type %q needs description = \"<one line>\" in its front matter", typeName)
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return Type{}, fmt.Errorf("type %q has no text after its front matter", typeName)
	}
	return Type{Name: typeName, Description: description, Text: body}, nil
}

//go:embed builtin/*.md
var builtin embed.FS

// BuiltIns are the types rewake ships, by name.
func BuiltIns() map[string]Type {
	out := map[string]Type{}
	entries, _ := fs.ReadDir(builtin, "builtin")
	for _, entry := range entries {
		typeName, ok := strings.CutSuffix(entry.Name(), ".md")
		if !ok {
			continue
		}
		raw, _ := fs.ReadFile(builtin, "builtin/"+entry.Name())
		value, err := Parse(typeName, raw)
		if err != nil {
			// A test parses every built-in; a broken one is a build that
			// must not ship.
			panic(err)
		}
		value.Source = BuiltIn
		out[typeName] = value
	}
	return out
}

// Names are the types of a set, in order.
func Names(types map[string]Type) []string {
	out := make([]string, 0, len(types))
	for typeName := range types {
		out = append(out, typeName)
	}
	sort.Strings(out)
	return out
}
