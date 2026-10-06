package docs

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// rulesDir holds the rules of 2.0, each naming the tests that hold it
// (rules/README.md). The test parses the rules and the Go files they name and
// runs nothing, so it costs the five checks almost nothing.
const rulesDir = "rules"

// ruleGroups are the numbered groups and how many rules each has. The operator
// decision's D1–D8 join in S18, with part B.
var ruleGroups = map[string]int{"E": 8, "O": 2, "T": 11, "C": 8, "L": 3}

// unnumbered are the rules without a number, by document and heading: each such
// section is one rule and ends with one Tests block. A rule is found by its
// heading, not by its Tests line, so deleting the block cannot delete the rule
// from the check.
var unnumbered = map[string][]string{
	"rules/host.md": {"The wrapper", "Grants"},
}

var (
	ruleStart = regexp.MustCompile(`^- \*\*([A-Z]+)(\d+)\.`)
	namedTest = regexp.MustCompile("^- `([^`]+)` `([^`]+)`")
	closedIn  = regexp.MustCompile(`closed in (S\d+)`)
)

// ruleBlock is one rule, or a Tests block that belongs to none.
type ruleBlock struct {
	where   string // document and line, for the messages
	number  string // a numbered rule's number
	section string // an unnumbered rule's heading
	outside bool   // a Tests block in no rule
	tests   []testRef
	gaps    []string
	listed  bool // a Tests: line was found
}

func (b ruleBlock) what(document string) string {
	if b.section != "" {
		return fmt.Sprintf("the rule %q of docs/%s", b.section, document)
	}
	return b.number
}

type testRef struct {
	where, file, name string
}

func TestEveryRuleNamesTestsThatExistAndRun(t *testing.T) {
	documents := map[string]string{}
	entries, err := os.ReadDir(rulesDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".md") {
			text, err := os.ReadFile(filepath.Join(rulesDir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			documents[filepath.Join(rulesDir, entry.Name())] = string(text)
		}
	}
	world, problems := readWorld(moduleRoot)
	for _, problem := range append(problems, checkRules(documents, world)...) {
		t.Error(problem)
	}
}

// checkRules returns every way the documents break the rules of rules/README.md.
func checkRules(documents map[string]string, world rulesWorld) []string {
	var problems []string
	seen := map[string][]string{}
	var names []string
	for name := range documents {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		blocks, refused := parseRules(name, documents[name])
		problems = append(problems, refused...)
		for _, block := range blocks {
			switch {
			case block.outside:
				problems = append(problems, fmt.Sprintf("%s: a Tests block in no rule: number the rule, or name its section in unnumbered of docs/rules_test.go", block.where))
				continue
			case block.number != "":
				seen[block.number] = append(seen[block.number], block.where)
			default:
				seen[name+"#"+block.section] = append(seen[name+"#"+block.section], block.where)
			}
			problems = append(problems, checkBlock(block, block.what(name), world)...)
		}
	}
	var expected []string
	for group, count := range ruleGroups {
		for n := 1; n <= count; n++ {
			expected = append(expected, fmt.Sprintf("%s%d", group, n))
		}
	}
	for document, sections := range unnumbered {
		for _, section := range sections {
			expected = append(expected, document+"#"+section)
		}
	}
	for _, id := range expected {
		switch len(seen[id]) {
		case 0:
			problems = append(problems, fmt.Sprintf("%s is in no document of docs/%s: a rule cannot be lost in a move", id, rulesDir))
		case 1:
		default:
			problems = append(problems, fmt.Sprintf("%s appears %d times (%s): a rule is stated once", id, len(seen[id]), strings.Join(seen[id], ", ")))
		}
	}
	for id, places := range seen {
		if !slices.Contains(expected, id) {
			problems = append(problems, fmt.Sprintf("%s at %s is no rule of 2.0: correct the number or add its group to ruleGroups", id, places[0]))
		}
	}
	slices.Sort(problems)
	return problems
}

func checkBlock(block ruleBlock, what string, world rulesWorld) []string {
	var problems []string
	if !block.listed {
		problems = append(problems, fmt.Sprintf("%s at %s has no Tests: line: a rule ends with the tests that hold it", what, block.where))
	} else if len(block.tests)+len(block.gaps) == 0 {
		problems = append(problems, fmt.Sprintf("%s at %s names no test and no gap", what, block.where))
	}
	for _, gap := range block.gaps {
		match := closedIn.FindStringSubmatch(gap)
		if stray, ok := strings.CutPrefix(gap, "unreadable: "); ok {
			problems = append(problems, fmt.Sprintf("%s: a line of the Tests block of %s is neither a test nor a gap: %q", block.where, what, stray))
		} else if match == nil {
			problems = append(problems, fmt.Sprintf("%s: the gap of %s names no step: write \"closed in S<n>\"", block.where, what))
		} else if !slices.Contains(world.steps, match[1]) {
			problems = append(problems, fmt.Sprintf("%s: the gap of %s is closed in %s, which is no step of docs/v2/stage3.md#the-build-order", block.where, what, match[1]))
		}
	}
	for _, ref := range block.tests {
		problems = append(problems, checkTest(ref, what, world)...)
	}
	return problems
}

// checkTest finds the named function by parsing its file, whatever the file's
// build constraint, holds it to go test's own rules for what it runs, then asks
// whether a check's go test reaches the file's package and builds the file.
func checkTest(ref testRef, rule string, world rulesWorld) []string {
	refuse := func(format string, args ...any) []string {
		return []string{fmt.Sprintf("%s: %s names %s in %s, "+format, append([]any{ref.where, rule, ref.name, ref.file}, args...)...)}
	}
	clean := path.Clean(ref.file)
	if clean != ref.file || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return refuse("which is no path inside the module: name it from the module root")
	}
	dir := path.Dir(clean)
	if nested := nestedModule(world.root, dir); nested != "" {
		return refuse("which belongs to the module of %s/go.mod, which no check of this module tests", nested)
	}
	full := filepath.Join(world.root, filepath.FromSlash(clean))
	file, err := parser.ParseFile(token.NewFileSet(), full, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return []string{fmt.Sprintf("%s: %s names %s, which cannot be read: %v", ref.where, rule, ref.file, err)}
	}
	if !strings.HasSuffix(clean, "_test.go") {
		return refuse("which is no _test.go file, so go test runs nothing of it")
	}
	if found, why := runnable(file, ref.name); !found {
		return refuse("which declares no such Test, Fuzz or Example function")
	} else if why != "" {
		return refuse("which go test does not run: %s", why)
	}
	reached := false
	for _, check := range world.checks {
		if !check.reaches(dir) {
			continue
		}
		reached = true
		context := build.Default
		context.BuildTags = check.tags
		// What go test -race adds, set rather than inherited, so the answer is
		// the same whether this test itself runs under -race or not.
		context.ToolTags = slices.DeleteFunc(slices.Clone(build.Default.ToolTags), func(tag string) bool { return tag == "race" })
		if check.race {
			context.ToolTags = append(context.ToolTags, "race")
		}
		if ok, err := context.MatchFile(filepath.Dir(full), filepath.Base(full)); err == nil && ok {
			return nil
		}
	}
	if !reached {
		return refuse("a package no go test of AGENTS.md's checks reaches (./... skips a directory named with a leading . or _, and testdata): move the test, or name another")
	}
	return refuse("a file no go test of AGENTS.md's checks builds: add the check with its tag, or name another test")
}

// nestedModule answers the directory between the root and dir, inclusive of
// dir, that has a go.mod of its own; ./... stops there.
func nestedModule(root, dir string) string {
	if dir == "." {
		return ""
	}
	elements := strings.Split(dir, "/")
	for i := range elements {
		prefix := strings.Join(elements[:i+1], "/")
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(prefix), "go.mod")); err == nil {
			return prefix
		}
	}
	return ""
}

// runnable holds a function to cmd/go's discovery: a Test, Fuzz or Example name
// whose next letter is not lower case, a test or fuzz target taking only
// *testing.T or *testing.F and returning nothing, an example with an output
// comment, without which it is compiled and never run.
func runnable(file *ast.File, name string) (found bool, why string) {
	i := slices.IndexFunc(file.Decls, func(decl ast.Decl) bool {
		fn, ok := decl.(*ast.FuncDecl)
		return ok && fn.Recv == nil && fn.Name.Name == name
	})
	if i < 0 {
		return false, ""
	}
	fn := file.Decls[i].(*ast.FuncDecl)
	prefix := ""
	for _, p := range []string{"Test", "Fuzz", "Example"} {
		if strings.HasPrefix(name, p) {
			prefix = p
		}
	}
	if prefix == "" {
		return false, ""
	}
	if next, _ := utf8.DecodeRuneInString(name[len(prefix):]); len(name) > len(prefix) && unicode.IsLower(next) {
		return true, fmt.Sprintf("a lower-case letter follows %s, so it is no %s function", prefix, prefix)
	}
	results := fn.Type.Results != nil && len(fn.Type.Results.List) > 0
	if prefix == "Example" {
		if fn.Type.Params.NumFields() > 0 || results || fn.Type.TypeParams != nil {
			return true, "an example takes and returns nothing"
		}
		for _, example := range doc.Examples(file) {
			if "Example"+example.Name == name && (example.Output != "" || example.EmptyOutput) {
				return true, ""
			}
		}
		return true, "an example without an output comment is compiled, never run"
	}
	want := map[string]string{"Test": "T", "Fuzz": "F"}[prefix]
	if fn.Type.Params.NumFields() != 1 || results || fn.Type.TypeParams != nil || !isTesting(file, fn.Type.Params.List[0].Type, want) {
		return true, fmt.Sprintf("a %s function is func(*testing.%s)", prefix, want)
	}
	return true, ""
}

// isTesting answers whether expr is *testing.<name>, under whatever name the
// file imports the package.
func isTesting(file *ast.File, expr ast.Expr, name string) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	for _, spec := range file.Imports {
		if spec.Path.Value != `"testing"` {
			continue
		}
		local := "testing"
		if spec.Name != nil {
			local = spec.Name.Name
		}
		if ident, ok := star.X.(*ast.Ident); ok && local == "." {
			return ident.Name == name
		}
		if sel, ok := star.X.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == local && sel.Sel.Name == name {
				return true
			}
		}
	}
	return false
}

// parseRules reads one document's rules and Tests blocks in prose: fenced code,
// where an example of the format is not a rule, is skipped, and a document the
// Markdown reader refuses yields that refusal. A line of a Tests block indented
// four or more may be code, so it is unreadable rather than a test.
func parseRules(name, text string) ([]ruleBlock, []string) {
	lines, err := mdLines(text)
	if err != nil {
		return nil, []string{fmt.Sprintf("docs/%s: %v", name, err)}
	}
	var blocks []ruleBlock
	current := -1
	inTests := false
	for i, line := range lines {
		where := fmt.Sprintf("docs/%s:%d", name, i+1)
		trimmed := strings.TrimSpace(line.text)
		title, isHeading := headingTitle(line.text)
		switch {
		case line.kind != proseLine:
		case isHeading:
			current, inTests = -1, false
			if slices.Contains(unnumbered[name], title) {
				blocks = append(blocks, ruleBlock{where: where, section: title})
				current = len(blocks) - 1
			}
		case ruleStart.MatchString(line.text):
			match := ruleStart.FindStringSubmatch(line.text)
			blocks = append(blocks, ruleBlock{where: where, number: match[1] + match[2]})
			current, inTests = len(blocks)-1, false
		case trimmed == "Tests:" && indentOf(line.text) < 4:
			if current < 0 {
				blocks = append(blocks, ruleBlock{where: where, outside: true})
				current = len(blocks) - 1
			}
			blocks[current].listed, inTests = true, true
		case inTests && trimmed != "" && indentOf(line.text) >= 4:
			blocks[current].gaps = append(blocks[current].gaps, "unreadable: "+trimmed)
			inTests = false
		case inTests && strings.HasPrefix(trimmed, "- Gap:"):
			blocks[current].gaps = append(blocks[current].gaps, trimmed)
		case inTests && namedTest.MatchString(trimmed):
			match := namedTest.FindStringSubmatch(trimmed)
			blocks[current].tests = append(blocks[current].tests, testRef{where: where, file: match[1], name: match[2]})
		case inTests && trimmed != "":
			blocks[current].gaps = append(blocks[current].gaps, "unreadable: "+trimmed)
			inTests = false
		}
	}
	return blocks, nil
}
