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
	"strings"
	"testing"
)

// registration is a way an adapter is registered rather than passed: an init
// in a package under adapter, or a package-level variable holding adapters.
type registration struct {
	file   string
	line   int
	offset int // the byte offset in file, which tells apart two on one line
	what   string
	body   string // an init's body, spacing folded; a variable is known by what
}

func (r registration) String() string {
	return fmt.Sprintf("%s:%d: %s: an adapter is a value cmd/rewake builds and passes down, never registered (docs/v2/design.md#the-test-that-holds-it, rule 3)",
		r.file, r.line, r.what)
}

// identity is what an exception records of a registration: an init by its
// body, so a second init or a new body in an excepted file is a new finding.
func (r registration) identity() string {
	return strings.TrimSpace(r.what + " " + r.body)
}

// findInits reports every func init of a file; the caller passes only files of
// packages under adapter, whatever their build constraint.
func findInits(name string, src []byte) ([]registration, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var found []registration
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "init" && fn.Body != nil {
			body := src[fset.Position(fn.Body.Lbrace).Offset : fset.Position(fn.Body.Rbrace).Offset+1]
			pos := fset.PositionFor(fn.Pos(), false)
			found = append(found, registration{
				file:   name,
				line:   pos.Line,
				offset: pos.Offset,
				what:   "func init in a package under adapter",
				body:   strings.Join(strings.Fields(string(body)), " "),
			})
		}
	}
	return found, nil
}

// findAdapterVars reports every package-level variable of a checked package
// whose type holds an adapter: one, or a pointer, slice, array, map or channel
// of them. A blank variable is a compile-time assertion and holds nothing.
func findAdapterVars(fset *token.FileSet, files []*ast.File, info *types.Info, adapters []*types.Interface) []registration {
	var found []registration
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				for _, name := range spec.(*ast.ValueSpec).Names {
					obj := info.Defs[name]
					if name.Name == "_" || obj == nil || !holdsAdapter(obj.Type(), adapters) {
						continue
					}
					// The physical position: a //line directive moves neither
					// the file nor the declaration.
					pos := fset.PositionFor(name.Pos(), false)
					found = append(found, registration{
						file:   pos.Filename,
						line:   pos.Line,
						offset: pos.Offset,
						what:   fmt.Sprintf("package-level variable %s holds adapters (%s)", name.Name, obj.Type()),
					})
				}
			}
		}
	}
	return found
}

func holdsAdapter(typ types.Type, adapters []*types.Interface) bool {
	switch t := typ.Underlying().(type) {
	case *types.Pointer:
		return holdsAdapter(t.Elem(), adapters)
	case *types.Slice:
		return holdsAdapter(t.Elem(), adapters)
	case *types.Array:
		return holdsAdapter(t.Elem(), adapters)
	case *types.Chan:
		return holdsAdapter(t.Elem(), adapters)
	case *types.Map:
		return holdsAdapter(t.Key(), adapters) || holdsAdapter(t.Elem(), adapters)
	}
	for _, iface := range adapters {
		if iface.NumMethods() > 0 && (types.Implements(typ, iface) || types.Implements(types.NewPointer(typ), iface)) {
			return true
		}
	}
	return false
}

// adapterVars type-checks every judged package of one build three ways — as
// built, with its internal tests, and its external test package — against the
// compiled export data of what each imports, and reports its variables holding
// adapters. A registry in a _test.go file is a registry too.
func adapterVars(t *testing.T, b build) []registration {
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
	var found []registration
	for _, pkg := range pkgs {
		if !typeChecked(pkg) {
			continue
		}
		// One importer per package: a test variant's imports are its own
		// compilations (ImportMap), and types from two of them never match.
		fset := token.NewFileSet()
		imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
			if mapped, ok := pkg.ImportMap[path]; ok {
				path = mapped
			}
			return os.Open(exports[path])
		})
		adapters := adapterTypes(t, imp)
		dir, err := filepath.Rel(root, pkg.Dir)
		if err != nil {
			t.Fatal(err)
		}
		var files []*ast.File
		for _, name := range slices.Concat(pkg.GoFiles, pkg.CgoFiles) {
			path := filepath.ToSlash(filepath.Join(dir, name))
			src, err := os.ReadFile(filepath.Join(moduleRoot, path))
			if err != nil {
				t.Fatal(err)
			}
			file, err := parser.ParseFile(fset, path, src, 0)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, file)
		}
		base, _, _ := strings.Cut(pkg.ImportPath, " ")
		info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
		config := types.Config{Importer: imp, FakeImportC: true}
		checked, err := config.Check(base, fset, files, info)
		if err != nil {
			t.Fatalf("type-checking %s: %v", pkg.ImportPath, err)
		}
		found = append(found, findAdapterVars(fset, files, info, ownInterfaces(checked, adapters))...)
	}
	return found
}

// typeChecked picks the compilations whose files are a judged package's own: the
// package as built, its variant with internal tests, its external test package.
// A package recompiled for another package's test has the same files as built.
func typeChecked(pkg listed) bool {
	path, ok := judged(pkg.ImportPath)
	if !ok {
		return false
	}
	if pkg.ForTest == "" {
		return true
	}
	return strings.TrimSuffix(module+"/"+path, "_test") == pkg.ForTest
}

// adapterTypes imports the adapter interfaces through one package's importer,
// so they are the types that package's code sees.
func adapterTypes(t *testing.T, imp types.Importer) map[string]*types.Interface {
	t.Helper()
	adapters := map[string]*types.Interface{}
	for _, name := range adapterInterfaces {
		path, typeName, _ := strings.Cut(name, ".")
		pkg, err := imp.Import(module + "/" + path)
		if err != nil {
			t.Fatal(err)
		}
		obj := pkg.Scope().Lookup(typeName)
		if obj == nil {
			t.Fatalf("the adapter interface %s does not exist: correct adapterInterfaces", name)
		}
		adapters[name] = obj.Type().Underlying().(*types.Interface)
	}
	return adapters
}

// ownInterfaces gives the package that declares an adapter interface its own
// copy of it: checked from source, its types are not the imported ones, and
// method sets naming them would never match.
func ownInterfaces(checked *types.Package, adapters map[string]*types.Interface) []*types.Interface {
	var own []*types.Interface
	for name, iface := range adapters {
		path, typeName, _ := strings.Cut(name, ".")
		if checked.Path() == module+"/"+path {
			iface = checked.Scope().Lookup(typeName).Type().Underlying().(*types.Interface)
		}
		own = append(own, iface)
	}
	return own
}
