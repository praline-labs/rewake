package docs

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The table of the suite's controls, and where the controls are declared.
const (
	controlsFile    = "testing-cases.md"
	controlsHeading = "## Every control"
	suiteDir        = "../test/workflow"
)

// backticked finds a name written as code: `name`.
var backticked = regexp.MustCompile("`([^`]+)`")

// TestEveryControlIsListed keeps the table of controls equal to the suite. A
// product mutant is a mutation value, so the two sets must match exactly: a
// mutant nobody lists is a control the reader of the table does not know is
// there, and a listed name no mutation carries teaches one that is gone. A
// fixture switch has no single shape in the code — a field of a control
// struct, an argument of a helper — so a listed switch is only required to be
// a name the suite spells; the reverse direction is left to the review.
func TestEveryControlIsListed(t *testing.T) {
	listedMutants, listedSwitches := controlTable(t)
	mutants, literals := suiteControls(t)

	for _, name := range mutants {
		if !slices.Contains(listedMutants, name) {
			t.Errorf("the mutant %q in test/workflow is not in the table under %q in docs/%s: add it to its scenario's row", name, controlsHeading, controlsFile)
		}
	}
	for _, name := range listedMutants {
		if !slices.Contains(mutants, name) {
			t.Errorf("docs/%s lists the mutant %q, which no mutation value in test/workflow carries: correct the name or remove it", controlsFile, name)
		}
	}
	for _, name := range listedSwitches {
		if slices.Contains(mutants, name) {
			t.Errorf("docs/%s lists %q as a fixture switch, but it is a mutant: move it to the mutants column", controlsFile, name)
		} else if !slices.Contains(literals, name) {
			t.Errorf("docs/%s lists the fixture switch %q, a name test/workflow never spells: correct the name or remove it", controlsFile, name)
		}
	}
}

// controlTable reads the table under controlsHeading: a scenario per row, its
// mutants in the second column and its fixture switches in the third.
func controlTable(t *testing.T) (mutants, switches []string) {
	t.Helper()
	file, err := os.Open(controlsFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	inside, rows := false, 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == controlsHeading:
			inside = true
		case inside && strings.HasPrefix(line, "#"):
			inside = false
		case inside && strings.HasPrefix(line, "|"):
			cells := strings.Split(line, "|")
			if len(cells) < 5 || strings.HasPrefix(strings.TrimSpace(cells[1]), "-") {
				continue
			}
			rows++
			mutants = append(mutants, names(cells[2])...)
			switches = append(switches, names(cells[3])...)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if rows == 0 {
		t.Fatalf("docs/%s has no table under %q", controlsFile, controlsHeading)
	}
	return mutants, switches
}

func names(cell string) []string {
	var found []string
	for _, match := range backticked.FindAllStringSubmatch(cell, -1) {
		found = append(found, match[1])
	}
	return found
}

// suiteControls parses the suite and answers the name of every mutation value
// and every string literal it spells.
func suiteControls(t *testing.T) (mutants, literals []string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(suiteDir, "*.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Go files in %s: %v", suiteDir, err)
	}
	files := token.NewFileSet()
	for _, path := range paths {
		parsed, err := parser.ParseFile(files, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.CompositeLit:
				for _, value := range mutationValues(node) {
					name := fieldString(value, "name")
					if name == "" {
						t.Errorf("%s: a mutation value without a name spelled as a string; the table cannot be checked against it", files.Position(value.Pos()))
					}
					mutants = append(mutants, name)
				}
			case *ast.BasicLit:
				if value, err := strconv.Unquote(node.Value); node.Kind == token.STRING && err == nil {
					literals = append(literals, value)
				}
			}
			return true
		})
	}
	return mutants, literals
}

// mutationValues answers the mutation values a composite literal spells: itself
// when its type is mutation, and the elements whose type is elided because the
// literal is a slice, array or map of mutations. An elided element has no type of
// its own to recognize it by, so without the second case a mutant declared in a
// list would pass unseen.
func mutationValues(literal *ast.CompositeLit) []*ast.CompositeLit {
	if isMutation(literal.Type) {
		return []*ast.CompositeLit{literal}
	}
	var element ast.Expr
	switch kind := literal.Type.(type) {
	case *ast.ArrayType:
		element = kind.Elt
	case *ast.MapType:
		element = kind.Value
	}
	if star, ok := element.(*ast.StarExpr); ok {
		element = star.X
	}
	if !isMutation(element) {
		return nil
	}
	var values []*ast.CompositeLit
	for _, item := range literal.Elts {
		if pair, ok := item.(*ast.KeyValueExpr); ok {
			item = pair.Value
		}
		if value, ok := item.(*ast.CompositeLit); ok && value.Type == nil {
			values = append(values, value)
		}
	}
	return values
}

// isMutation reports whether a type expression names the suite's mutation type.
func isMutation(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "mutation"
}

// fieldString answers the string a composite literal gives one of its fields.
func fieldString(literal *ast.CompositeLit, field string) string {
	for _, element := range literal.Elts {
		pair, ok := element.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := pair.Key.(*ast.Ident)
		if !ok || key.Name != field {
			continue
		}
		if value, ok := pair.Value.(*ast.BasicLit); ok && value.Kind == token.STRING {
			unquoted, _ := strconv.Unquote(value.Value)
			return unquoted
		}
	}
	return ""
}
