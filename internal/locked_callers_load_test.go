package internal

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

const (
	module     = "github.com/praline-labs/rewake"
	moduleRoot = ".."
)

// The caller check covers the production files of every tag set. It needs
// their export data to resolve function references, but no test binaries.
func tagSets() [][]string {
	return [][]string{nil, {"rewakefault"}, {"rewakefixture"}, {"rewakefault", "rewakefixture"}}
}

type build struct{ tags []string }

type listed struct {
	ImportPath string
	Dir        string
	Export     string
	ImportMap  map[string]string
	GoFiles    []string
	CgoFiles   []string
	Error      *struct{ Err string }
}

func loadModule(t *testing.T, b build) []listed {
	t.Helper()
	// The caller set does not depend on VCS state. In particular, a checkout
	// copied without its metadata must still be checkable.
	args := []string{"list", "-e", "-json", "-deps", "-export", "-buildvcs=false"}
	if len(b.tags) > 0 {
		args = append(args, "-tags", strings.Join(b.tags, ","))
	}
	cmd := exec.Command("go", append(args, "./internal/...", "./cmd/...")...)
	cmd.Dir = moduleRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v\n%s", err, stderr.String())
	}
	var all []listed
	for decoder := json.NewDecoder(bytes.NewReader(out)); ; {
		var pkg listed
		if err := decoder.Decode(&pkg); errors.Is(err, io.EOF) {
			return all
		} else if err != nil {
			t.Fatal(err)
		}
		if pkg.Error != nil {
			t.Fatalf("go list: %s: %s", pkg.ImportPath, pkg.Error.Err)
		}
		all = append(all, pkg)
	}
}

// judged excludes dependencies and this test-only package from the callers.
func judged(importPath string) (string, bool) {
	path, ok := strings.CutPrefix(importPath, module+"/")
	return path, ok && (strings.HasPrefix(path, "internal/") || strings.HasPrefix(path, "cmd/"))
}
