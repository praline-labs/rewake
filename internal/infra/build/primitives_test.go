package build

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// primitives are the writes of infra/state that work without a lease
// (docs/v2/stage3-state.md#the-bootstrap).
var primitives = map[string]bool{"Bootstrap": true, "OpenLeaseLock": true, "WriteBuildRecord": true}

// acquirer is the one file allowed to call them.
const acquirer = "internal/infra/build/lease.go"

const statePath = "github.com/praline-labs/rewake/internal/infra/state"

// The writes that need no lease are the bootstrap's, and only the
// acquisition makes them: a call from anywhere else in the module's code —
// tests aside — would be a write no lease covers.
func TestOnlyAcquireCallsBootstrapPrimitives(t *testing.T) {
	root := moduleRoot(t)
	var calls []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != root && (strings.HasPrefix(name, ".") || hasModule(path)) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		for _, call := range primitiveCalls(t, path) {
			if rel != acquirer {
				calls = append(calls, rel+": "+call)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) > 0 {
		t.Fatalf("bootstrap primitives called outside %s:\n%s", acquirer, strings.Join(calls, "\n"))
	}
	if len(primitiveCalls(t, filepath.Join(root, acquirer))) < len(primitives) {
		t.Fatalf("%s no longer calls every primitive; the test looks in the wrong place", acquirer)
	}
}

// primitiveCalls lists the calls of a primitive in one file: through the
// state package's import, or by name inside the state package itself.
func primitiveCalls(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	alias := ""
	for _, spec := range file.Imports {
		if value, _ := strconv.Unquote(spec.Path.Value); value == statePath {
			alias = "state"
			if spec.Name != nil {
				alias = spec.Name.Name
			}
		}
	}
	inState := file.Name.Name == "state" && strings.Contains(filepath.ToSlash(path), "/internal/infra/state/")
	var found []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if x, ok := fun.X.(*ast.Ident); ok && alias != "" && x.Name == alias && primitives[fun.Sel.Name] {
				found = append(found, fun.Sel.Name)
			}
		case *ast.Ident:
			if inState && primitives[fun.Name] {
				found = append(found, fun.Name)
			}
		}
		return true
	})
	return found
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for !hasModule(dir) {
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test")
		}
		dir = parent
	}
	return dir
}

func hasModule(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil
}
