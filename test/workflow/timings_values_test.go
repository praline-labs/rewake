package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// suiteBuild is the binary the suite builds (buildBinary: go build of
// ./cmd/rewake, no tags) as the go command compiles it: each package of that
// build, the files its constraints select, and the export data of each.
type suiteBuild struct {
	packages map[string]builtPackage
}

type builtPackage struct {
	ImportPath, Dir, Export string
	GoFiles, CgoFiles       []string
	Error                   *struct{ Err string }
}

func loadSuiteBuild(root string) (suiteBuild, error) {
	cmd := exec.Command("go", "list", "-e", "-json", "-deps", "-export", "./cmd/rewake")
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return suiteBuild{}, fmt.Errorf("go list: %w\n%s", err, stderr.String())
	}
	build := suiteBuild{packages: map[string]builtPackage{}}
	for decoder := json.NewDecoder(bytes.NewReader(out)); ; {
		var pkg builtPackage
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			return build, nil
		} else if err != nil {
			return suiteBuild{}, err
		}
		if pkg.Error != nil {
			return suiteBuild{}, fmt.Errorf("go list: %s: %s", pkg.ImportPath, pkg.Error.Err)
		}
		build.packages[pkg.ImportPath] = pkg
	}
}

// settable answers why -X path.name would set nothing in the suite's binary,
// or "" when it sets the variable. The linker sets a package-level variable of
// type string declared uninitialized or initialized to a constant string
// expression (cmd/link's documentation of -X); an initializer that calls a
// function or reads another variable runs after it and wins. A name it does
// not find, in a file the build leaves out among others, it skips silently.
func (b suiteBuild) settable(path, name string) (string, error) {
	pkg, ok := b.packages[path]
	if !ok {
		return "is in no package of the binary the suite builds", nil
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, file := range slices.Concat(pkg.GoFiles, pkg.CgoFiles) {
		parsed, err := parser.ParseFile(fset, filepath.Join(pkg.Dir, file), nil, 0)
		if err != nil {
			return "", err
		}
		files = append(files, parsed)
	}
	imp := importer.ForCompiler(fset, "gc", func(path string) (io.ReadCloser, error) {
		return os.Open(b.packages[path].Export)
	})
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Types: map[ast.Expr]types.TypeAndValue{}}
	checked, err := (&types.Config{Importer: imp, FakeImportC: true}).Check(path, fset, files, info)
	if err != nil {
		return "", err
	}
	variable, ok := checked.Scope().Lookup(name).(*types.Var)
	if !ok {
		return "is no package-level variable of a file the suite's build compiles", nil
	}
	if variable.Type() != types.Typ[types.String] {
		return fmt.Sprintf("is a %s, and the linker sets only a string", variable.Type()), nil
	}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				at := slices.IndexFunc(value.Names, func(ident *ast.Ident) bool { return info.Defs[ident] == variable })
				switch {
				case at < 0, len(value.Values) == 0:
				case len(value.Values) != len(value.Names) || info.Types[value.Values[at]].Value == nil:
					return "is initialized by an expression that is no constant, which runs after the linker sets it", nil
				}
			}
		}
	}
	return "", nil
}

// TestABuildValueTheLinkerWouldIgnoreFails proves the check of the suite's -X
// values on a module of its own, read through the same go list: each name
// below is one the linker sets, or one it skips without a word.
func TestABuildValueTheLinkerWouldIgnoreFails(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"go.mod":             "module example.org/knobs\n\ngo 1.25\n",
		"cmd/rewake/main.go": "package main\n\nimport _ \"example.org/knobs/knob\"\n\nfunc main() {}\n",
		"knob/knob.go": "package knob\n\nimport \"strings\"\n\nconst prefix = \"x\"\n\ntype kind string\n\n" +
			"var (\n\tbare      string\n\tconstant  = \"x\"\n\tjoined    = prefix + \"y\"\n\ttyped     string = \"z\"\n" +
			"\tcalled    = strings.Repeat(\"x\", 2)\n\tcopied    = constant\n\tnumber    int\n\tnamed     kind\n)\n",
		"knob/knob_test.go": "package knob\n\nvar onlyInTests string\n",
		"knob/excluded.go":  "//go:build never_in_the_suite\n\npackage knob\n\nvar excluded string\n",
		"unused/unused.go":  "package unused\n\nvar outside string\n",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	build, err := loadSuiteBuild(root)
	if err != nil {
		t.Fatal(err)
	}
	for target, want := range map[string]string{
		"knob.bare":        "",
		"knob.constant":    "",
		"knob.joined":      "",
		"knob.typed":       "",
		"knob.called":      "no constant",
		"knob.copied":      "no constant",
		"knob.number":      "is a int",
		"knob.named":       "is a example.org/knobs/knob.kind",
		"knob.onlyInTests": "no package-level variable of a file the suite's build compiles",
		"knob.excluded":    "no package-level variable of a file the suite's build compiles",
		"knob.renamed":     "no package-level variable of a file the suite's build compiles",
		"unused.outside":   "is in no package of the binary the suite builds",
	} {
		dot := strings.LastIndex(target, ".")
		pkg, name := target[:dot], target[dot+1:]
		got, err := build.settable("example.org/knobs/"+pkg, name)
		if err != nil {
			t.Fatal(err)
		}
		if want == "" && got != "" || want != "" && !strings.Contains(got, want) {
			t.Errorf("-X %s: got %q, want %q", target, got, want)
		}
	}
}
