package internal

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

// lockedCallers names, for each of mail's functions that run under a mailbox
// lock their caller holds, the only production functions that may refer to
// them (docs/v2/stage3-moves.md#s15-the-delivery-server-to-hostdelivery-codex).
// Each takes no lock of its own, since the lock is not reentrant, so a caller
// outside its section would run it unlocked and nothing at run time would
// say so. The table is a discipline of the repository, not a capability of the
// lock: a new caller is reviewed here before it lands.
var lockedCallers = map[string][]string{
	"internal/core/mail.SweepFinishedLocked": {"internal/core/mail.(*Server).sweepFinished"},
	"internal/core/mail.SettleLocked":        {"internal/core/mail.(*Server).settleOutcome", "internal/core/mail.(*Server).settleUnreadOnly"},
	"internal/core/mail.SweepAwaitingLocked": {"internal/core/mail.(*Server).followEarlierRun"},
}

// TestTheLockedEntryPointsKeepTheirCallers holds lockedCallers over the
// production files of every tag set: a call or a function value outside the
// listed functions fails, and so does a listed caller that no longer refers to
// its function, since an expected set nothing matches proves nothing.
func TestTheLockedEntryPointsKeepTheirCallers(t *testing.T) {
	seen := map[string]map[string]bool{}
	for _, tags := range tagSets() {
		for target, callers := range lockedReferences(t, build{tags: tags}) {
			if seen[target] == nil {
				seen[target] = map[string]bool{}
			}
			for _, caller := range callers {
				seen[target][caller] = true
			}
		}
	}
	for _, problem := range callerProblems(seen, lockedCallers) {
		t.Error(problem)
	}
}

// callerProblems compares the references found with the table, both ways.
func callerProblems(seen map[string]map[string]bool, table map[string][]string) []string {
	var problems []string
	for target, allowed := range table {
		if len(allowed) == 0 {
			problems = append(problems, fmt.Sprintf("%s lists no caller: a function nobody may call is deleted, not listed", target))
		}
		for _, caller := range allowed {
			if !seen[target][caller] {
				problems = append(problems, fmt.Sprintf("%s is listed as a caller of %s and does not refer to it: correct lockedCallers", caller, target))
			}
		}
	}
	for target, callers := range seen {
		for caller := range callers {
			if !slices.Contains(table[target], caller) {
				problems = append(problems, fmt.Sprintf("%s refers to %s, which runs under a mailbox lock its caller holds; only %s may", caller, target, strings.Join(table[target], ", ")))
			}
		}
	}
	sort.Strings(problems)
	return problems
}

// lockedReferences type-checks the production files of every judged package of
// one build and answers, for each function of lockedCallers, the outermost
// declarations that refer to it: a closure counts as the function it is in, a
// package-level initializer as its variable.
func lockedReferences(t *testing.T, b build) map[string][]string {
	t.Helper()
	pkgs := loadModule(t, b)
	exports := map[string]string{}
	for _, pkg := range pkgs {
		exports[pkg.ImportPath] = pkg.Export
	}
	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string][]string{}
	for _, pkg := range pkgs {
		path, ok := judged(pkg.ImportPath)
		if !ok || pkg.ForTest != "" {
			continue
		}
		fset := token.NewFileSet()
		imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
			if mapped, ok := pkg.ImportMap[path]; ok {
				path = mapped
			}
			return os.Open(exports[path])
		})
		dir, err := filepath.Rel(root, pkg.Dir)
		if err != nil {
			t.Fatal(err)
		}
		var files []*ast.File
		for _, name := range slices.Concat(pkg.GoFiles, pkg.CgoFiles) {
			file, err := parser.ParseFile(fset, filepath.Join(moduleRoot, dir, name), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, file)
		}
		info := &types.Info{Uses: map[*ast.Ident]types.Object{}}
		config := types.Config{Importer: imp, FakeImportC: true}
		if _, err := config.Check(pkg.ImportPath, fset, files, info); err != nil {
			t.Fatalf("type-checking %s: %v", pkg.ImportPath, err)
		}
		for _, file := range files {
			for _, decl := range file.Decls {
				owner := declName(path, decl)
				ast.Inspect(decl, func(n ast.Node) bool {
					ident, ok := n.(*ast.Ident)
					if !ok {
						return true
					}
					fn, ok := info.Uses[ident].(*types.Func)
					if !ok || fn.Pkg() == nil {
						return true
					}
					target := strings.TrimPrefix(fn.Pkg().Path(), module+"/") + "." + fn.Name()
					if _, restricted := lockedCallers[target]; restricted && !slices.Contains(found[target], owner) {
						found[target] = append(found[target], owner)
					}
					return true
				})
			}
		}
	}
	return found
}

// declName names a top-level declaration as lockedCallers does: a function
// by its package and name, a method with its receiver, anything else by the
// names it declares.
func declName(path string, decl ast.Decl) string {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Recv == nil || len(d.Recv.List) == 0 {
			return path + "." + d.Name.Name
		}
		return path + ".(" + receiverName(d.Recv.List[0].Type) + ")." + d.Name.Name
	case *ast.GenDecl:
		var names []string
		for _, spec := range d.Specs {
			if value, ok := spec.(*ast.ValueSpec); ok {
				for _, name := range value.Names {
					names = append(names, name.Name)
				}
			}
		}
		return path + "." + d.Tok.String() + " " + strings.Join(names, ",")
	}
	return path
}

func receiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return "*" + receiverName(e.X)
	case *ast.IndexExpr:
		return receiverName(e.X)
	case *ast.IndexListExpr:
		return receiverName(e.X)
	case *ast.Ident:
		return e.Name
	}
	return fmt.Sprintf("%T", expr)
}

// The comparison fails both ways and on an empty list, whatever the module
// holds today.
func TestCallerProblemsSeeEveryMismatch(t *testing.T) {
	table := map[string][]string{"p.F": {"p.G"}, "p.H": nil}
	seen := map[string]map[string]bool{"p.F": {"p.G": true, "p.X": true}}
	problems := callerProblems(seen, table)
	for _, want := range []string{"p.H lists no caller", "p.X refers to p.F"} {
		if !slices.ContainsFunc(problems, func(p string) bool { return strings.HasPrefix(p, want) }) {
			t.Errorf("no problem starting %q in %q", want, problems)
		}
	}
	if problems := callerProblems(map[string]map[string]bool{}, table); !slices.ContainsFunc(problems, func(p string) bool {
		return strings.HasPrefix(p, "p.G is listed as a caller of p.F and does not refer to it")
	}) {
		t.Errorf("a listed caller that refers to nothing passed: %q", problems)
	}
}
