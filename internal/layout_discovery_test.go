package internal

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The cases below prove the rules through their discovery: a synthetic module
// is written to disk and the test's own loaders read it — go list, the walk of
// the directories, the parser — so a fault the loaders would drop cannot pass
// as a fault the rule judged.

// syntheticModule writes a module with this module's path, the adapter
// interface the registry rule looks for and a command, plus the given files, and makes it
// the module the rules read. It returns a writer for later edits.
func syntheticModule(t *testing.T, files map[string]string) func(name, src string) {
	t.Helper()
	return syntheticModuleIn(t, t.TempDir(), files)
}

// syntheticModuleIn is syntheticModule written at root.
func syntheticModuleIn(t *testing.T, root string, files map[string]string) func(name, src string) {
	t.Helper()
	write := func(name, src string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module "+module+"\n\ngo 1.25\n")
	write("internal/adapter/harness.go", "package adapter\n\ntype Harness interface{ ID() string }\n")
	write("cmd/rewake/main.go", "package main\n\nfunc main() {}\n")
	for name, src := range files {
		write(name, src)
	}
	t.Chdir(filepath.Join(root, "internal"))
	return write
}

// registrationProblems are the registry rule's findings as the test reports them.
func registrationProblems(t *testing.T) []string {
	t.Helper()
	var problems []string
	for _, r := range registrations(t) {
		problems = append(problems, r.String())
	}
	return problems
}

func oneContaining(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != 1 || slices.ContainsFunc(want, func(w string) bool { return !strings.Contains(got[0], w) }) {
		t.Fatalf("got %q, want one problem containing %q", got, want)
	}
}

// TestAStrayGitAboveTheModuleIsNotRead: a temporary directory may lie below an
// empty .git that is no repository, as a sandbox can leave one; the rules
// judge the module's files and never its version-control state.
func TestAStrayGitAboveTheModuleIsNotRead(t *testing.T) {
	above := t.TempDir()
	if err := os.Mkdir(filepath.Join(above, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	syntheticModuleIn(t, filepath.Join(above, "module"), map[string]string{
		"internal/core/mail/mail.go":    "package mail\n",
		"internal/infra/state/state.go": "package state\n\nimport _ \"" + module + "/internal/core/mail\"\n",
	})
	oneContaining(t, importProblems(t),
		"internal/infra/state/state.go: internal/infra/state (infra) imports internal/core/mail (core)")
}

// TestEveryFileIsReadWhateverItsConstraint: a package only a tag builds, or a
// file no build ever reads, is still read by the registry rule.
func TestEveryFileIsReadWhateverItsConstraint(t *testing.T) {
	syntheticModule(t, map[string]string{
		"internal/adapter/fake/fault.go": "//go:build rewakefault\n\npackage fake\n\nimport \"" + module + "/internal/adapter\"\n\nvar all []adapter.Harness\n",
		"internal/adapter/fake/never.go": "//go:build ignore\n\npackage fake\n\nfunc init() {}\n",
		"internal/adapter/fake/plain.go": "package fake\n",
	})
	got := registrationProblems(t)
	slices.Sort(got)
	if len(got) != 2 || !strings.HasPrefix(got[0], "internal/adapter/fake/fault.go:7: package-level variable all holds adapters") ||
		!strings.HasPrefix(got[1], "internal/adapter/fake/never.go:5: func init") {
		t.Fatalf("got %q, want the tagged variable and the ignored init", got)
	}
}

func TestARegistryInATestFileFails(t *testing.T) {
	syntheticModule(t, map[string]string{
		"internal/adapter/registry_test.go": "package adapter\n\nvar hidden []Harness\n",
		"internal/adapter/fake/fake.go":     "package fake\n",
		"internal/adapter/fake/x_test.go":   "package fake_test\n\nimport \"" + module + "/internal/adapter\"\n\nvar byName map[string]adapter.Harness\n",
	})
	got := registrationProblems(t)
	slices.Sort(got)
	if len(got) != 2 || !strings.HasPrefix(got[0], "internal/adapter/fake/x_test.go:5: package-level variable byName holds adapters") ||
		!strings.HasPrefix(got[1], "internal/adapter/registry_test.go:3: package-level variable hidden holds adapters") {
		t.Fatalf("got %q, want the internal and the external test file's registries", got)
	}
}

func TestEveryImportReachesTheRule(t *testing.T) {
	syntheticModule(t, map[string]string{
		"go.mod":                         "module " + module + "\n\ngo 1.25\n\nrequire example.org/external v0.0.0\n\nreplace example.org/external => ./external\n",
		"external/go.mod":                "module example.org/external\n\ngo 1.25\n",
		"external/external.go":           "package external\n",
		"internal/host/host.go":          "package host\n\nimport _ \"example.org/external\"\n",
		"internal/core/mail/mail.go":     "package mail\n\nimport (\n\t\"strings\"\n\n\t_ `" + module + "/internal/host`\n)\n\nvar _ = strings.Cut\n",
		"internal/infra/state/a.go":      "package state\n\nimport (\n\t_ \"example.org/external\"\n\t\"os\"\n)\n\nvar _ = os.Getpid\n",
		"internal/infra/state/a_test.go": "package state\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) {}\n",
	})
	got := importProblems(t)
	slices.Sort(got)
	want := []string{
		"internal/core/mail/mail.go: internal/core/mail (core) imports internal/host (host), which the import rule forbids",
		"internal/infra/state/a.go: internal/infra/state (infra) imports example.org/external (external), which the import rule forbids",
	}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q: the standard library and a host's dependency are allowed", got, want)
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Fatalf("got %q, want %q", got[i], want[i])
		}
	}
}

// TestTwoDeclarationsOnOneLineAreTwo: a registration is known by its byte
// offset, so a second name in a var spec, or a second init on its line, is a
// finding of its own, and a //line directive moves neither.
func TestTwoDeclarationsOnOneLineAreTwo(t *testing.T) {
	const registry = "internal/adapter/registry.go"
	const adapter = "internal/adapter/fake/fake.go"
	write := syntheticModule(t, map[string]string{
		registry: "package adapter\n\nvar registered, additional []Harness\n",
		adapter:  "package fake\n",
	})
	if got := registrationProblems(t); len(got) != 2 {
		t.Fatalf("got %q, want both names of the var spec", got)
	}
	write(registry, "package adapter\n")
	write(adapter, "package fake\n\nfunc init() {}; func init() {}\n")
	if got := registrationProblems(t); len(got) != 2 {
		t.Fatalf("got %q, want both inits on the line", got)
	}
	write(adapter, "package fake\n\n//line elsewhere.go:40\nfunc init() {}\n\n//line elsewhere.go:40\nfunc init() {}\n")
	if got := registrationProblems(t); len(got) != 2 {
		t.Fatalf("got %q, want both inits under one //line", got)
	}
}

// TestARaceOnlyFileIsJudged: go test builds with -race, which selects
// files of its own; their imports and registries are judged like any other.
func TestARaceOnlyFileIsJudged(t *testing.T) {
	syntheticModule(t, map[string]string{
		"internal/core/mail/mail.go":   "package mail\n",
		"internal/infra/state/a.go":    "package state\n",
		"internal/infra/state/race.go": "//go:build race\n\npackage state\n\nimport _ \"" + module + "/internal/core/mail\"\n",
		"internal/adapter/race.go":     "//go:build race\n\npackage adapter\n\nvar raced []Harness\n",
		"internal/adapter/norace.go":   "//go:build !race\n\npackage adapter\n\nvar plain []Harness\n",
	})
	oneContaining(t, importProblems(t),
		"internal/infra/state/race.go: internal/infra/state (infra) imports internal/core/mail (core)", "(only under -race; -race -tags rewakefault; -race -tags rewakefixture; -race -tags rewakefault,rewakefixture)")
	got := registrationProblems(t)
	slices.Sort(got)
	if len(got) != 2 || !strings.HasPrefix(got[0], "internal/adapter/norace.go:5: package-level variable plain") ||
		!strings.HasPrefix(got[1], "internal/adapter/race.go:5: package-level variable raced") {
		t.Fatalf("got %q, want the registries of the plain and the race build", got)
	}
}

// TestADirectoryWithItsOwnModuleIsNoPackage: the walk that finds packages on
// disk stops at a go.mod of its own, as ./... does, so a module kept inside
// the tree — a record, never built — is judged by none of the rules.
func TestADirectoryWithItsOwnModuleIsNoPackage(t *testing.T) {
	syntheticModule(t, map[string]string{
		"internal/old/go.mod":        "module example.org/old\n\ngo 1.25\n",
		"internal/old/relay/old.go":  "package relay\n",
		"internal/core/mail/mail.go": "package mail\n",
	})
	got := packageDirs(t)
	if !slices.Contains(got, "internal/core/mail") || slices.ContainsFunc(got, func(dir string) bool { return strings.HasPrefix(dir, "internal/old") }) {
		t.Fatalf("got %q, want internal/core/mail and nothing under the nested module internal/old", got)
	}
}
