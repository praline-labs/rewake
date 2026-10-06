package internal

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"
)

// The layers of 2.0 (docs/v2/design.md#the-layout). An adapter's layer carries
// its name, so that one adapter importing another is told apart from an adapter
// importing its own parts.
const (
	infra   = "infra"
	core    = "core"
	tool    = "tool"
	api     = "adapter"
	catalog = "adapter/catalog"
	host    = "host"
	cli     = "cli"
	command = "cmd"

	// external is any package of another module. The design's table names one
	// layer's limit on them, infra's "the standard library only"; for the others it
	// is silent, and each dependency is its own decision (AGENTS.md, Code).
	external = "external"
)

// mayImport is the import rule's table. A layer may always import itself, and
// an adapter its own parts; infra may import infra (correction 1 of
// docs/v2/stage3.md, agreed in review: state imports boottime).
var mayImport = map[string][]string{
	infra: {},
	core:  {infra, external},
	tool:  {core, infra, external},
	api:   {core, infra, external},
	// Every adapter alike: the API, the tools and the core, never another adapter.
	"adapter/*": {api, tool, core, infra, external},
	catalog:     {api, "adapter/*", external},
	host:        {api, tool, core, infra, external},
	cli:         {host, api, tool, core, infra, external},
	command:     {cli, catalog, external},
}

// named marks the layers whose files rule 2 reads: everything but the
// adapters, the catalog and the command, which are where harness names live.
var named = []string{core, tool, host, cli, infra}

func isAdapter(layer string) bool {
	return layer != catalog && strings.HasPrefix(layer, api+"/")
}

func underAdapter(layer string) bool {
	return layer == api || strings.HasPrefix(layer, api+"/")
}

// allowed answers whether a package of one layer may import one of another.
func allowed(from, to string) bool {
	if from == to {
		return true
	}
	for _, may := range mayImport[layerKey(from)] {
		if may == to || may == "adapter/*" && isAdapter(to) {
			return true
		}
	}
	return false
}

func layerKey(layer string) string {
	if isAdapter(layer) {
		return "adapter/*"
	}
	return layer
}

// transit is a package not yet at its 2.0 path: the layer it is judged in and
// the step that moves it there, or removes it.
type transit struct {
	layer string
	step  string
	goes  bool
}

// place is where the layout test judges a package from.
type place struct {
	layer string
	goes  string // the step that removes it; empty for a package that stays
}

// placeOf finds a package's layer: the transition table first, then its 2.0
// path. A package in neither is a new place nobody decided, and fails.
func placeOf(path string, table map[string]transit) (place, error) {
	if entry, ok := table[path]; ok {
		if entry.goes {
			return place{layer: entry.layer, goes: entry.step}, nil
		}
		return place{layer: entry.layer}, nil
	}
	if layer, ok := layerOfPath(path); ok {
		return place{layer: layer}, nil
	}
	return place{}, fmt.Errorf("%s is neither at a 2.0 path nor in the transition table: give it a layer and the step that moves it, or move it to its place", path)
}

// layerOfPath is the layer a 2.0 path names.
func layerOfPath(path string) (string, bool) {
	switch {
	case path == "cmd/rewake":
		return command, true
	case path == "internal/adapter":
		return api, true
	case path == "internal/adapter/catalog":
		return catalog, true
	case strings.HasPrefix(path, "internal/adapter/"):
		name, _, _ := strings.Cut(strings.TrimPrefix(path, "internal/adapter/"), "/")
		return api + "/" + name, true
	}
	for _, layer := range []string{infra, core, tool, host, cli} {
		if path == "internal/"+layer || strings.HasPrefix(path, "internal/"+layer+"/") {
			return layer, true
		}
	}
	return "", false
}

// goFile is one Go file of a build variant, by its path relative to the module
// root, with the import paths it names.
type goFile struct {
	name    string
	imports []string
}

type goPackage struct {
	path  string
	files []goFile
}

// variant is the module's graph under one build, with the packages the go
// command reports as the standard library's, which every layer may use.
type variant struct {
	build    build
	packages []goPackage
	standard map[string]bool
}

// edge is one forbidden import, found in a file under some variants.
type edge struct {
	file, from, to string
	fromAt, toAt   place
	builds         []build
}

func (e edge) String() string {
	return fmt.Sprintf("%s: %s (%s) imports %s (%s%s), which the import rule forbids (docs/v2/design.md#the-import-rule)%s",
		e.file, e.from, e.fromAt.layer, e.to, e.toAt.layer, describeGoing(e.toAt), underBuilds(e.builds))
}

// underBuilds says which builds an edge exists in when the plain one is not
// among them, since such an edge is fixed in a file a plain build never reads.
func underBuilds(builds []build) string {
	if slices.ContainsFunc(builds, func(b build) bool { return len(b.flags()) == 0 }) {
		return ""
	}
	var names []string
	for _, b := range builds {
		names = append(names, b.String())
	}
	return " (only under " + strings.Join(names, "; ") + ")"
}

// judgeImports checks every edge of every variant against the import rule; the
// placement errors are packages the tables do not place.
func judgeImports(variants []variant, table map[string]transit) ([]edge, []string) {
	var found []edge
	var unplaced []string
	for _, v := range variants {
		for _, pkg := range v.packages {
			from, err := placeOf(pkg.path, table)
			if err != nil {
				unplaced = appendNew(unplaced, err.Error())
				continue
			}
			for _, file := range pkg.files {
				for _, path := range file.imports {
					if v.standard[path] {
						continue
					}
					imported, to := path, place{layer: external}
					if within, ok := strings.CutPrefix(path, module+"/"); ok {
						imported = within
						if to, err = placeOf(imported, table); err != nil {
							unplaced = appendNew(unplaced, err.Error())
							continue
						}
					}
					if forbidden(from, to) {
						found = addEdge(found, edge{file: file.name, from: pkg.path, to: imported, fromAt: from, toAt: to}, v.build)
					}
				}
			}
		}
	}
	return found, unplaced
}

// forbidden adds one condition to the table: a package that stays imports
// nothing a step removes, since the removal would otherwise break it.
func forbidden(from, to place) bool {
	if !allowed(from.layer, to.layer) {
		return true
	}
	return to.goes != "" && (from.goes == "" || stepNumber(from.goes) > stepNumber(to.goes))
}

func describeGoing(p place) string {
	if p.goes == "" {
		return ""
	}
	return ", goes in " + p.goes
}

func addEdge(found []edge, e edge, b build) []edge {
	for i := range found {
		if found[i].file == e.file && found[i].from == e.from && found[i].to == e.to {
			found[i].builds = append(found[i].builds, b)
			return found
		}
	}
	e.builds = []build{b}
	return append(found, e)
}

func appendNew(list []string, item string) []string {
	if slices.Contains(list, item) {
		return list
	}
	return append(list, item)
}

func stepNumber(step string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(step, "S"))
	if err != nil || !strings.HasPrefix(step, "S") {
		return -1
	}
	return n
}

// mention is a harness word found in a file outside the adapters.
type mention struct {
	file string
	line int
	word string
	kind string // identifier, string or comment
	text string // the identifier, the string's value or the comment
}

// identity is what an exception records of a mention: its kind and its text
// with the spacing folded, not its line, which moves with every edit above it.
func (m mention) identity() string {
	return m.kind + " " + strings.Join(strings.Fields(m.text), " ")
}

func (m mention) String() string {
	return fmt.Sprintf("%s:%d: the %s names the harness word %q: the core knows no harness, so the text belongs in the adapter",
		m.file, m.line, m.kind, m.word)
}

// harnessWords are the words rule 2 looks for: the catalog's ids and titles
// and the test's own list, lower case. A word containing another is dropped,
// because every place it matches the shorter one matches too, and one mention
// should fail once.
func harnessWords(catalog, own []string) []string {
	var all []string
	for _, word := range append(slices.Clone(catalog), own...) {
		all = appendNew(all, strings.ToLower(word))
	}
	var words []string
	for _, word := range all {
		if !slices.ContainsFunc(all, func(other string) bool { return other != word && strings.Contains(word, other) }) {
			words = append(words, word)
		}
	}
	slices.Sort(words)
	return words
}

// findMentions parses one file and reports every identifier, string literal
// and comment naming a harness word. Import paths are left to rule 1, which
// judges them with their layers.
func findMentions(name string, src []byte, words []string) ([]mention, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var found []mention
	// spans says the text is the source's own lines, so a newline in it moves
	// the line; a decoded escape does not.
	note := func(pos token.Pos, text, kind string, spans bool) {
		lower := strings.ToLower(text)
		for _, word := range words {
			for offset := 0; ; {
				at := strings.Index(lower[offset:], word)
				if at < 0 {
					break
				}
				line := fset.Position(pos).Line
				if spans {
					line += strings.Count(lower[:offset+at], "\n")
				}
				found = append(found, mention{file: name, line: line, word: word, kind: kind, text: text})
				offset += at + len(word)
			}
		}
	}
	imports := map[*ast.BasicLit]bool{}
	for _, spec := range file.Imports {
		imports[spec.Path] = true
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			note(n.Pos(), n.Name, "identifier", false)
		case *ast.BasicLit:
			// The value, not the spelling: "\x63odex" names the harness as
			// surely as "codex". Only a raw string's newlines are the source's.
			if n.Kind == token.STRING && !imports[n] {
				value, err := strconv.Unquote(n.Value)
				if err != nil {
					value = n.Value
				}
				note(n.Pos(), value, "string", strings.HasPrefix(n.Value, "`"))
			}
		}
		return true
	})
	for _, group := range file.Comments {
		for _, comment := range group.List {
			note(comment.Pos(), comment.Text, "comment", true)
		}
	}
	return found, nil
}
