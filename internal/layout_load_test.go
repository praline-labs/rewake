package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// listed is the part of `go list -json` the test reads.
type listed struct {
	ImportPath   string
	Dir          string
	Export       string
	Standard     bool
	ForTest      string
	ImportMap    map[string]string
	GoFiles      []string
	CgoFiles     []string
	TestGoFiles  []string
	XTestGoFiles []string
	Error        *struct{ Err string }
}

func goList(t *testing.T, args ...string) []listed {
	t.Helper()
	// The module's version-control state is not read: a synthetic module lies
	// in a temporary directory, where a stray .git above it would make go list
	// ask git, fail, and turn every rule red for a reason outside the module.
	cmd := exec.Command("go", append([]string{"list", "-e", "-json", "-buildvcs=false"}, args...)...)
	cmd.Dir = moduleRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	var all []listed
	for decoder := json.NewDecoder(bytes.NewReader(out)); ; {
		var one listed
		if err := decoder.Decode(&one); errors.Is(err, io.EOF) {
			return all
		} else if err != nil {
			t.Fatal(err)
		}
		if one.Error != nil {
			t.Fatalf("go list: %s: %s", one.ImportPath, one.Error.Err)
		}
		all = append(all, one)
	}
}

// build is one way the go command compiles the module: its tags and whether
// -race is on, which adds the race tool tag to every file's constraint.
type build struct {
	tags []string
	race bool
}

func (b build) flags() []string {
	var flags []string
	if b.race {
		flags = append(flags, "-race")
	}
	if len(b.tags) > 0 {
		flags = append(flags, "-tags", strings.Join(b.tags, ","))
	}
	return flags
}

func (b build) String() string { return strings.Join(b.flags(), " ") }

// loadModule lists the judged packages of one build with everything they and
// their tests depend on, compiled: the standard library is told apart by the
// go command's own answer, and the test variants carry export data rule 3
// type-checks against.
func loadModule(t *testing.T, b build) []listed {
	t.Helper()
	args := append([]string{"-deps", "-test", "-export"}, b.flags()...)
	return goList(t, append(args, "./internal/...", "./cmd/...")...)
}

// judged answers the module-relative path of a package the layout judges, and
// false for anything else: a dependency, this package, a test binary.
func judged(importPath string) (string, bool) {
	base, _, _ := strings.Cut(importPath, " ")
	path, ok := strings.CutPrefix(base, module+"/")
	if !ok || path == "internal" || strings.HasSuffix(path, ".test") {
		return "", false
	}
	return path, strings.HasPrefix(path, "internal/") || strings.HasPrefix(path, "cmd/")
}

// variants loads the graph of every tag set, test files included: a test that
// imports across the layers ties the layers together as surely as code does.
// Every import is kept, the standard library's and outside modules' too, so the
// rule rather than the loader decides what a layer may reach.
func variants(t *testing.T) []variant {
	t.Helper()
	var all []variant
	for _, b := range builds() {
		v := variant{build: b, standard: map[string]bool{"C": true}}
		pkgs := loadModule(t, b)
		for _, pkg := range pkgs {
			if pkg.Standard {
				v.standard[pkg.ImportPath] = true
			}
		}
		for _, pkg := range pkgs {
			path, ok := judged(pkg.ImportPath)
			if !ok || pkg.ForTest != "" {
				continue
			}
			p := goPackage{path: path}
			for _, name := range slices.Concat(pkg.GoFiles, pkg.CgoFiles, pkg.TestGoFiles, pkg.XTestGoFiles) {
				p.files = append(p.files, goFile{name: path + "/" + name, imports: importsOf(t, path+"/"+name)})
			}
			v.packages = append(v.packages, p)
		}
		all = append(all, v)
	}
	return all
}

// importsOf decodes every import of a file as the compiler does, so a path in
// backquotes is the same edge as one in double quotes.
func importsOf(t *testing.T, name string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(moduleRoot, name), nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	var imports []string
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("%s: import %s: %v", name, spec.Path.Value, err)
		}
		imports = append(imports, path)
	}
	return imports
}

// packageDirs are the package directories under internal/ and cmd/ that a
// ./... pattern reaches, found on disk rather than by the go command: go list
// leaves out a directory whose every file a build constraint excludes, and
// rules 2 and 3 read every file whatever its constraint. Restricted to the
// given layers when any.
func packageDirs(t *testing.T, layers []string) []string {
	t.Helper()
	var dirs []string
	for _, top := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(filepath.Join(moduleRoot, top), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if path != filepath.Join(moduleRoot, top) {
				if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" {
					return filepath.SkipDir
				}
				if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			rel, err := filepath.Rel(moduleRoot, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			if rel == "internal" || len(goFiles(t, rel)) == 0 {
				return nil
			}
			if layers != nil {
				p, err := placeOf(rel, transition)
				if err != nil || !slices.Contains(layers, p.layer) {
					return nil
				}
			}
			dirs = append(dirs, rel)
			return nil
		})
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
	}
	return dirs
}

// goFiles are every Go file of a package directory, whatever its constraint:
// a name in a file only a tag builds is still in the tree.
func goFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(moduleRoot, dir))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
			names = append(names, dir+"/"+entry.Name())
		}
	}
	return names
}
