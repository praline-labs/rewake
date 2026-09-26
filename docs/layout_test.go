package docs

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The source layout, and the module root it describes.
const (
	layoutFile = "code.md"
	moduleRoot = ".."
)

// TestEveryPackageIsInTheLayout keeps the tree in code.md equal to the Go
// packages of the module. The tree promises to be complete, and a reader
// looking for where something lives trusts it: a package missing from it is
// looked for in the wrong place, and a listed one that is gone sends the
// reader to a directory that is not there.
func TestEveryPackageIsInTheLayout(t *testing.T) {
	listed := layoutTree(t)
	packages := goPackages(t)
	for _, dir := range packages {
		if !slices.Contains(listed, dir) {
			t.Errorf("the package %s is not in the tree in docs/%s: add a line saying what it holds", dir, layoutFile)
		}
	}
	for _, dir := range listed {
		if !slices.Contains(packages, dir) {
			t.Errorf("docs/%s lists %s, which holds no Go package: correct the path or remove the line", layoutFile, dir)
		}
	}
}

// layoutTree reads the first fenced block of the layout: one directory per
// line, ending in a slash, then what it holds. An indented line continues the
// description above it.
func layoutTree(t *testing.T) []string {
	t.Helper()
	file, err := os.Open(layoutFile)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var dirs []string
	fences := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() && fences < 2 {
		line := scanner.Text()
		if strings.HasPrefix(line, "```") {
			fences++
			continue
		}
		if fences == 1 && !strings.HasPrefix(line, " ") {
			if fields := strings.Fields(line); len(fields) > 0 {
				dirs = append(dirs, strings.TrimSuffix(fields[0], "/"))
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatalf("docs/%s has no tree in a fenced block", layoutFile)
	}
	return dirs
}

// goPackages answers every directory of the module holding a Go file, skipped
// as the go command skips them: testdata, names starting with a dot or an
// underscore, and nested modules.
func goPackages(t *testing.T) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(moduleRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if path == moduleRoot {
				return nil
			}
			if name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return filepath.SkipDir
			}
			// A directory with its own go.mod is another module, outside this one's
			// package list — handoffs/ keeps archived snapshots that way.
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		dir, err := filepath.Rel(moduleRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		if dir = filepath.ToSlash(dir); !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return dirs
}
