package workflow

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// The suite's binary runs a few of the product's waits shorter than a
// release, set at build through package buildtime: each is waited out in
// dozens of cases, and at the real length it adds minutes to a run while
// proving nothing the shorter one does not. The code is a release's; only
// these numbers differ, and docs/testing.md lists them.
//
// Each stays long enough for what the cases need of it under a loaded
// machine: the cases run side by side (pool_test.go), and a wait cut to the
// bone on an idle machine is a flaky case on a busy one.
const (
	// suiteQuiet and suiteCap are the coalescing window's (internal/inbox,
	// three and four seconds). The scenario that observes the window,
	// batch-arrival, spaces its heads-ups 600 ms apart and needs them inside
	// this quiet and cap. At 1.5 and 2 seconds, one crosswise run in four lost
	// its third heads-up to the cap: with six cases running, the sends that
	// write the heads-ups started over a second late.
	suiteQuiet = 2 * time.Second
	suiteCap   = 3 * time.Second
	// suitePickup is how long rewake compact and rewake interrupt wait for
	// the target to take a request (internal/cli, five seconds). The plugin
	// polls four times a second and the Codex wrapper faster; the cases where
	// nobody takes it wait out all of it.
	suitePickup = 1500 * time.Millisecond
	// suiteLetterWait is how long a compaction letter waits for its other half
	// (internal/wrap, three seconds). Both halves come within moments of each
	// other; the cases where one never comes wait out all of it.
	suiteLetterWait = 1500 * time.Millisecond
	// suiteNoticeScan is how often main's wrapper looks for other sessions'
	// comings, goings and compactions (internal/wrap, one second). The cases
	// that prove no notice was sent wait out a scan and the coalescing cap.
	suiteNoticeScan = 250 * time.Millisecond
	// suiteCompactionStart and suiteCompactionRun are the bounds of a Codex
	// compaction's mark (internal/harness/codex/gateway, 80 seconds while its
	// turn has not been seen to start, 10 minutes once it has). compact-hold
	// runs one compaction past the first and one past the second; the steered
	// cases' compactions start at once and end well inside the second.
	suiteCompactionStart = 2 * time.Second
	suiteCompactionRun   = 6 * time.Second
)

// suiteFlags are the build flags that set those values, for the binary under
// test and every mutant alike: a mutant built at the real lengths would be
// judged against windows its cases no longer wait for.
var suiteFlags = []string{"-ldflags", strings.Join([]string{
	buildValue("inbox", "builtQuiet", suiteQuiet),
	buildValue("inbox", "builtCap", suiteCap),
	buildValue("cli", "builtPickup", suitePickup),
	buildValue("wrap", "builtLetterWait", suiteLetterWait),
	buildValue("wrap", "builtNoticeScan", suiteNoticeScan),
	buildValue("harness/codex/gateway", "builtCompactionStart", suiteCompactionStart),
	buildValue("harness/codex/gateway", "builtCompactionRun", suiteCompactionRun),
}, " ")}

// buildFlags are suiteFlags with the process tree the launch's look for
// earlier-build writers lists: an empty one of the build's own under dir, so
// no launch of the suite depends on what else runs on the machine
// (docs/protocol-cutover.md). The look's own judgements are the unit tests'
// of internal/cutover, over trees they describe.
func buildFlags(dir string) ([]string, error) {
	tree := filepath.Join(dir, "proc")
	if err := os.MkdirAll(tree, 0o700); err != nil {
		return nil, err
	}
	return []string{"-ldflags", suiteFlags[1] + " -X github.com/praline-labs/rewake/internal/cutover.builtProcRoot=" + tree}, nil
}

func buildValue(pkg, name string, value time.Duration) string {
	return fmt.Sprintf("-X github.com/praline-labs/rewake/internal/%s.%s=%s", pkg, name, value)
}

// The linker ignores -X for a variable that does not exist, silently: a knob
// renamed in the product would leave the suite running at the real length with
// nothing to say so. So each one is looked up in its package's source.
func TestSuiteBuildValuesNameRealVariables(t *testing.T) {
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	flags, err := buildFlags(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range strings.Fields(flags[1]) {
		if flag == "-X" {
			continue
		}
		target, _, _ := strings.Cut(flag, "=")
		dot := strings.LastIndex(target, ".")
		pkg := strings.TrimPrefix(target[:dot], "github.com/praline-labs/rewake/")
		name := target[dot+1:]
		found, err := declaresString(filepath.Join(root, pkg), name)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Errorf("-X %s names no string variable of %s: the build would ignore it", flag, pkg)
		}
	}
}

// declaresString reports whether a non-test file of dir declares name as an
// uninitialized string variable, the only kind -X sets.
func declaresString(dir, name string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			return false, err
		}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				value := spec.(*ast.ValueSpec)
				kind, ok := value.Type.(*ast.Ident)
				if !ok || kind.Name != "string" || len(value.Values) > 0 {
					continue
				}
				if slices.ContainsFunc(value.Names, func(ident *ast.Ident) bool { return ident.Name == name }) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
