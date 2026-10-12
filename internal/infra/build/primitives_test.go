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

// acquirer is the one file allowed to refer to them.
const acquirer = "internal/infra/build/lease.go"

const statePath = "github.com/praline-labs/rewake/internal/infra/state"

// The writes that need no lease are the bootstrap's, and only the
// acquisition makes them: a reference from anywhere else in the module's code —
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
		for _, ref := range primitiveRefs(t, path) {
			if rel != acquirer {
				calls = append(calls, rel+": "+ref)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) > 0 {
		t.Fatalf("bootstrap primitives referred to outside %s:\n%s", acquirer, strings.Join(calls, "\n"))
	}
	if len(primitiveRefs(t, filepath.Join(root, acquirer))) < len(primitives) {
		t.Fatalf("%s no longer calls every primitive; the test looks in the wrong place", acquirer)
	}
}

// The check counts a reference to a primitive, not only a call written at
// the place it is named: a primitive kept in a function value and called
// elsewhere, passed as an argument, reached through a dot import or a second
// import name, or bound to a variable inside the state package is as much a
// write without a lease as a call.
func TestBootstrapCallerCheckSeesEveryReference(t *testing.T) {
	sources := map[string]string{
		"a function value": "package outside\nimport s \"" + statePath + "\"\nfunc bad() { boot := s.Bootstrap; _, _ = boot(\"/tmp/other\", \"default\") }\n",
		"an argument":      "package outside\nimport \"" + statePath + "\"\nfunc bad() { run(state.WriteBuildRecord) }\n",
		"a dot import":     "package outside\nimport . \"" + statePath + "\"\nfunc bad() { _, _ = OpenLeaseLock(\"/tmp/other\") }\n",
		"a second name":    "package outside\nimport (\n\t\"" + statePath + "\"\n\tother \"" + statePath + "\"\n)\nvar _ = state.Dir\nvar boot = other.Bootstrap\n",
	}
	for name, source := range sources {
		file := filepath.Join(t.TempDir(), "outside.go")
		if err := os.WriteFile(file, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		if found := primitiveRefs(t, file); len(found) == 0 {
			t.Errorf("the check missed a primitive reached through %s", name)
		}
	}
	inside := filepath.Join(t.TempDir(), "internal", "infra", "state", "inside.go")
	if err := os.MkdirAll(filepath.Dir(inside), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inside, []byte("package state\nvar boot = Bootstrap\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if found := primitiveRefs(t, inside); len(found) == 0 {
		t.Error("the check missed a primitive bound to a variable inside the state package")
	}
}

// primitiveRefs lists the references to a primitive in one file — called,
// kept in a value or passed on alike: through any name the state package is
// imported under, through a dot import, or by name inside the state package
// itself, its own declaration aside. Without type information a dot-imported
// file's local of the same name is counted too; erring that way is the safe
// side.
func primitiveRefs(t *testing.T, path string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	aliases := map[string]bool{}
	bare := file.Name.Name == "state" && strings.Contains(filepath.ToSlash(path), "/internal/infra/state/")
	for _, spec := range file.Imports {
		if value, _ := strconv.Unquote(spec.Path.Value); value == statePath {
			switch {
			case spec.Name == nil:
				aliases["state"] = true
			case spec.Name.Name == ".":
				bare = true
			default:
				aliases[spec.Name.Name] = true
			}
		}
	}
	declared := map[*ast.Ident]bool{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil {
			declared[fn.Name] = true
		}
	}
	var found []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.SelectorExpr:
			if x, ok := node.X.(*ast.Ident); ok && aliases[x.Name] && primitives[node.Sel.Name] {
				found = append(found, node.Sel.Name)
			}
			// The selected name is a field or method of whatever X is, never
			// the package-level primitive; only X can still refer to one.
			ast.Inspect(node.X, func(inner ast.Node) bool {
				if id, ok := inner.(*ast.Ident); ok && bare && primitives[id.Name] {
					found = append(found, id.Name)
				}
				return true
			})
			return false
		case *ast.Ident:
			if bare && primitives[node.Name] && !declared[node] {
				found = append(found, node.Name)
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
