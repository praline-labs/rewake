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
// interface rule 3 looks for and a command, plus the given files, and makes it
// the module the rules read. It returns a writer for later edits.
func syntheticModule(t *testing.T, files map[string]string) func(name, src string) {
	t.Helper()
	root := t.TempDir()
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
	write("internal/harness/harness.go", "package harness\n\ntype Harness interface{ ID() string }\n")
	write("cmd/rewake/main.go", "package main\n\nfunc main() {}\n")
	for name, src := range files {
		write(name, src)
	}
	t.Chdir(filepath.Join(root, "internal"))
	return write
}

// snapshot records findings as an exception table would on its first run.
func snapshot[F finding, K comparable](found []F, key func(F) K) map[K]excuse {
	groups := map[K][]F{}
	for _, f := range found {
		groups[key(f)] = append(groups[key(f)], f)
	}
	table := map[K]excuse{}
	for k, group := range groups {
		count, sum := fingerprint(group)
		table[k] = excuse{reason: "recorded", step: "S8", count: count, sum: sum}
	}
	return table
}

func oneContaining(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != 1 || slices.ContainsFunc(want, func(w string) bool { return !strings.Contains(got[0], w) }) {
		t.Fatalf("got %q, want one problem containing %q", got, want)
	}
}

func TestANameExceptionAdmitsNoNewMention(t *testing.T) {
	const name = "internal/core/mail/mail.go"
	const before = "package mail\n\n// wait as codex does\n\n// and codex again\nvar mode int\n"
	write := syntheticModule(t, map[string]string{name: before})
	words := []string{"codex"}
	table := snapshot(mentions(t, words), nameKey)
	if got := nameProblems(t, words, table); len(got) > 0 {
		t.Fatalf("the recorded file fails: %q", got)
	}
	for _, c := range []struct{ what, src, want string }{
		{"a new identifier beside an excepted comment", before + "var codexMode int\n", "and the file now has 3"},
		{"a new string beside an excepted comment", before + "var mode2 = \"on codex\"\n", "and the file now has 3"},
		{"a new escaped string", before + "var mode2 = \"\\x63odex\"\n", "and the file now has 3"},
		{"a partial deletion", "package mail\n\n// wait as codex does\nvar mode int\n", "and the file now has 1"},
		{"an old mention replaced by another", strings.Replace(before, "and codex again", "or codex once", 1), "and the file now has 2"},
	} {
		write(name, c.src)
		oneContaining(t, nameProblems(t, words, table), `the name exception for "codex" in internal/core/mail/mail.go records 2`, c.want)
	}
	write(name, "package mail\n")
	oneContaining(t, nameProblems(t, words, table), "matches nothing any more")
}

func TestAnInitExceptionAdmitsNoSecondInit(t *testing.T) {
	const name = "internal/adapter/fake/fake.go"
	const before = "package fake\n\nfunc init() { setup() }\n\nfunc setup() {}\n"
	write := syntheticModule(t, map[string]string{name: before})
	table := snapshot(registrations(t), registryKey)
	if got := registryProblems(t, table); len(got) > 0 {
		t.Fatalf("the recorded file fails: %q", got)
	}
	for _, c := range []struct{ what, src, want string }{
		{"a second init", before + "\nfunc init() { setup() }\n", "and the file now has 2"},
		{"a new body", strings.Replace(before, "{ setup() }", "{ setup(); setup() }", 1), "and the file now has 1"},
	} {
		write(name, c.src)
		oneContaining(t, registryProblems(t, table), `the registry exception "func init in a package under adapter" in internal/adapter/fake/fake.go records 1`, c.want)
	}
}

// TestEveryFileIsReadWhateverItsConstraint: a package only a tag builds, or one
// no build ever reads, is still read by rules 2 and 3 and still exists for the
// transition table.
func TestEveryFileIsReadWhateverItsConstraint(t *testing.T) {
	write := syntheticModule(t, map[string]string{
		"internal/core/tagonly/name.go":  "//go:build rewakefault\n\npackage tagonly\n\nvar mode = \"codex\"\n",
		"internal/core/never/name.go":    "//go:build ignore\n\npackage never\n\nvar mode = \"codex\"\n",
		"internal/adapter/fake/fault.go": "//go:build rewakefault\n\npackage fake\n\nimport \"" + module + "/internal/harness\"\n\nvar all []harness.Harness\n",
		"internal/adapter/fake/never.go": "//go:build ignore\n\npackage fake\n\nfunc init() {}\n",
		"internal/adapter/fake/plain.go": "package fake\n",
	})
	got := nameProblems(t, []string{"codex"}, nil)
	slices.Sort(got)
	if len(got) != 2 || !strings.HasPrefix(got[0], "internal/core/never/name.go:5: the string") ||
		!strings.HasPrefix(got[1], "internal/core/tagonly/name.go:5: the string") {
		t.Fatalf("got %q, want the strings of both constrained packages", got)
	}
	got = registryProblems(t, nil)
	slices.Sort(got)
	if len(got) != 2 || !strings.HasPrefix(got[0], "internal/adapter/fake/fault.go:7: package-level variable all holds adapters") ||
		!strings.HasPrefix(got[1], "internal/adapter/fake/never.go:5: func init") {
		t.Fatalf("got %q, want the tagged variable and the ignored init", got)
	}
	write("internal/tagonly/a.go", "//go:build rewakefault\n\npackage tagonly\n")
	table := map[string]transit{"internal/tagonly": {layer: core, step: "S13"}}
	if got := staleTransitions(table, packageDirs(t, nil), []string{"S13"}); len(got) > 0 {
		t.Fatalf("a package only a tag builds is not gone: %q", got)
	}
}

func TestARegistryInATestFileFails(t *testing.T) {
	syntheticModule(t, map[string]string{
		"internal/harness/registry_test.go": "package harness\n\nvar hidden []Harness\n",
		"internal/adapter/fake/fake.go":     "package fake\n",
		"internal/adapter/fake/x_test.go":   "package fake_test\n\nimport \"" + module + "/internal/harness\"\n\nvar byName map[string]harness.Harness\n",
	})
	got := registryProblems(t, nil)
	slices.Sort(got)
	if len(got) != 2 || !strings.HasPrefix(got[0], "internal/adapter/fake/x_test.go:5: package-level variable byName holds adapters") ||
		!strings.HasPrefix(got[1], "internal/harness/registry_test.go:3: package-level variable hidden holds adapters") {
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
	got := importProblems(t, transition, nil)
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
// offset, so a second name in an excepted var spec, or a second init on its
// line, is a finding of its own.
func TestTwoDeclarationsOnOneLineAreTwo(t *testing.T) {
	const registry = "internal/harness/registry.go"
	const adapter = "internal/adapter/fake/fake.go"
	write := syntheticModule(t, map[string]string{
		registry: "package harness\n\nvar registered []Harness\n",
		adapter:  "package fake\n\nfunc init() {}\n",
	})
	table := snapshot(registrations(t), registryKey)
	if got := registryProblems(t, table); len(got) > 0 {
		t.Fatalf("the recorded files fail: %q", got)
	}
	write(registry, "package harness\n\nvar registered, additional []Harness\n")
	oneContaining(t, registryProblems(t, table), "internal/harness/registry.go:3: package-level variable additional holds adapters")
	write(registry, "package harness\n\nvar registered []Harness\n")
	write(adapter, "package fake\n\nfunc init() {}; func init() {}\n")
	oneContaining(t, registryProblems(t, table), `the registry exception "func init in a package under adapter" in internal/adapter/fake/fake.go records 1`, "and the file now has 2")
	write(adapter, "package fake\n\n//line elsewhere.go:40\nfunc init() {}\n\n//line elsewhere.go:40\nfunc init() {}\n")
	oneContaining(t, registryProblems(t, table), "and the file now has 2")
}

// TestARaceOnlyFileIsJudged: the five checks build with -race, which selects
// files of its own; their imports and registries are judged like any other.
func TestARaceOnlyFileIsJudged(t *testing.T) {
	syntheticModule(t, map[string]string{
		"internal/core/mail/mail.go":   "package mail\n",
		"internal/infra/state/a.go":    "package state\n",
		"internal/infra/state/race.go": "//go:build race\n\npackage state\n\nimport _ \"" + module + "/internal/core/mail\"\n",
		"internal/harness/race.go":     "//go:build race\n\npackage harness\n\nvar raced []Harness\n",
		"internal/harness/norace.go":   "//go:build !race\n\npackage harness\n\nvar plain []Harness\n",
	})
	oneContaining(t, importProblems(t, transition, nil),
		"internal/infra/state/race.go: internal/infra/state (infra) imports internal/core/mail (core)", "(only under -race; -race -tags rewakefault)")
	got := registryProblems(t, nil)
	slices.Sort(got)
	if len(got) != 2 || !strings.HasPrefix(got[0], "internal/harness/norace.go:5: package-level variable plain") ||
		!strings.HasPrefix(got[1], "internal/harness/race.go:5: package-level variable raced") {
		t.Fatalf("got %q, want the registries of the plain and the race build", got)
	}
}

func TestOnlyTheBuildOrderNamesSteps(t *testing.T) {
	write := syntheticModule(t, nil)
	const order = "## The build order\n\n```markdown\n| S41 | an example |\n```\n\n| Step | What |\n|---|---|\n| S1 | tests |\n| S2 | moves |\n"
	write("docs/v2/stage3.md", "# Stage\n\n"+order+"\n| S42 | after the table |\n\n## An unrelated example\n\n| Step | What |\n|---|---|\n| S40 | not a step |\n\n```markdown\n| S43 | fenced |\n```\n")
	if got := buildSteps(t); !slices.Equal(got, []string{"S1", "S2"}) {
		t.Fatalf("got %q, want the build order's own rows", got)
	}
	for text, want := range map[string]string{
		"## Elsewhere\n\n| Step | What |\n|---|---|\n| S1 | tests |\n":                           "no table of steps under the heading The build order",
		"## The build order\n\nNo table.\n\n## Next\n\n| Step | What |\n|---|---|\n| S1 | x |\n": "no table of steps under the heading The build order",
		"## The build order\n\n| Name | What |\n|---|---|\n| S1 | x |\n":                         "no table of steps",
		"## The build order\n\n| Step | What |\n| S1 | x |\n":                                    "has no divider row",
		"## The build order\n\n| Step | What |\n|---|---|\n| X1 | x |\n":                         "names no step",
		"## The build order\n\n| Step | What |\n|---|---|\n| S1 | x |\n| S1 | y |\n":             "names S1 twice",
		// A second heading of the same title does not reopen the section.
		"## The build order\n\nNo table.\n\n## Later\n\n## The build order\n\n" + table40: "no table of steps under the heading The build order",
		"## The build order\n\n~~~markdown\n| S40 | an example |\n":                       "the fence of line 3 is never closed",
	} {
		write("docs/v2/stage3.md", text)
		if _, err := readSteps(); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: got %v, want %q", text, err, want)
		}
	}
}

const table40 = "| Step | What |\n|---|---|\n| S40 | an example |\n"

// TestAnExampleInAnyFenceNamesNoStep: a fence is closed only by a run of its
// own character at least as long, so a tilde fence, or a longer backtick fence
// holding a shorter one, keeps its example out of the build order.
func TestAnExampleInAnyFenceNamesNoStep(t *testing.T) {
	write := syntheticModule(t, nil)
	for _, example := range []string{
		"~~~markdown\n" + table40 + "~~~\n",
		"````markdown\n```\n" + table40 + "```\n````\n",
		"~~~\n```\n" + table40 + "```\n~~~\n",
		"```\n```go\n" + table40 + "```\n",
	} {
		write("docs/v2/stage3.md", "# Stage\n\n## The build order\n\n"+example+"\n| Step | What |\n|---|---|\n| S1 | tests |\n")
		if got := buildSteps(t); !slices.Equal(got, []string{"S1"}) {
			t.Fatalf("%q: got %q, want only S1", example, got)
		}
	}
}
