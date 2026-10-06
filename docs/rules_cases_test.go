package docs

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The cases below prove the rules test on synthetic documents and a synthetic
// module root, each with one known fault and the message promised for it. The
// root is read by readWorld, as the real one is: its checks come from its
// AGENTS.md, its steps from its build order.

const syntheticAgents = "# x\n\n## Checks\n\n```bash\nenv -u A \\\n  go test -race -shuffle=on ./...\n```\n"

// syntheticWorld is a module root with one plain test file and one only the
// rewakefault tag builds, checked by the given AGENTS.md.
func syntheticWorld(t *testing.T, agents string) (rulesWorld, func(name, text string)) {
	t.Helper()
	root := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.org/synthetic\n\ngo 1.25\n")
	write("AGENTS.md", agents)
	write("docs/v2/stage3.md", "## The build order\n\n| Step | What |\n|---|---|\n| S1 | tests |\n| S2 | moves |\n| S3 | stops |\n")
	write("internal/mail/mail_test.go", "package mail\n\nimport \"testing\"\n\nfunc TestHeld(t *testing.T) {}\n\nfunc helper() {}\n")
	write("internal/mail/fault_test.go", "//go:build rewakefault\n\npackage mail\n\nimport \"testing\"\n\nfunc TestUnderFault(t *testing.T) {}\n")
	world, problems := readWorld(root)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	return world, write
}

// wholeGroups states every rule of the real groups once, each held, so a case
// changes one thing and sees one problem.
func wholeGroups(skip string) string {
	var text strings.Builder
	for _, group := range []string{"E", "O", "T", "C", "L"} {
		for n := 1; n <= ruleGroups[group]; n++ {
			number := group + strconv.Itoa(n)
			if number == skip {
				continue
			}
			text.WriteString("- **" + number + ". A rule.**\n  Tests:\n  - `internal/mail/mail_test.go` `TestHeld`\n\n")
		}
	}
	return text.String()
}

const wholeHost = "# The host\n\n## The wrapper\n\nIts run.\n\nTests:\n- `internal/mail/mail_test.go` `TestHeld`\n\n" +
	"## Grants\n\nThe scheme.\n\nTests:\n- `internal/mail/mail_test.go` `TestHeld`\n"

func rulesProblems(t *testing.T, extra string, skip string) []string {
	t.Helper()
	world, _ := syntheticWorld(t, syntheticAgents)
	docs := map[string]string{"rules/all.md": wholeGroups(skip) + "## Under test\n\n" + extra, "rules/host.md": wholeHost}
	return checkRules(docs, world)
}

func oneProblem(t *testing.T, got []string, want string) {
	t.Helper()
	if len(got) != 1 || !strings.Contains(got[0], want) {
		t.Fatalf("got %q, want one problem containing %q", got, want)
	}
}

func held(file, name string) string {
	return "- **E2. A rule.**\n  Tests:\n  - `" + file + "` `" + name + "`\n"
}

func TestTheWholeSyntheticSetPasses(t *testing.T) {
	if got := rulesProblems(t, "", ""); len(got) > 0 {
		t.Fatal(got)
	}
}

func TestARuleWithNoTestFails(t *testing.T) {
	oneProblem(t, rulesProblems(t, "- **E2. A rule.**\n  Tests:\n", "E2"), "E2 at docs/rules/all.md:")
	oneProblem(t, rulesProblems(t, "- **E2. A rule.**\n  Tests:\n", "E2"), "names no test and no gap")
	oneProblem(t, rulesProblems(t, "- **E2. A rule with no block.**\n", "E2"), "has no Tests: line")
}

func TestATestThatDoesNotExistFails(t *testing.T) {
	oneProblem(t, rulesProblems(t, held("internal/mail/mail_test.go", "TestGone"), "E2"),
		"E2 names TestGone in internal/mail/mail_test.go, which declares no such Test, Fuzz or Example function")
	oneProblem(t, rulesProblems(t, held("internal/mail/mail_test.go", "helper"), "E2"),
		"which declares no such Test, Fuzz or Example function")
	oneProblem(t, rulesProblems(t, held("internal/gone_test.go", "TestHeld"), "E2"),
		"E2 names internal/gone_test.go, which cannot be read")
}

// TestATestGoTestWouldNotRunFails holds the names to cmd/go's discovery: each
// function below is legal Go that go test compiles and never runs.
func TestATestGoTestWouldNotRunFails(t *testing.T) {
	world, write := syntheticWorld(t, syntheticAgents)
	write("internal/mail/odd_test.go", "package mail\n\nimport (\n\t\"fmt\"\n\tt \"testing\"\n)\n\n"+
		"func Testhelper(x *t.T) {}\n\nfunc TestTakesB(b *t.B) {}\n\nfunc TestReturns(x *t.T) error { return nil }\n\n"+
		"func FuzzWithT(x *t.T) {}\n\nfunc ExampleSilent() { fmt.Println(1) }\n\nfunc ExampleSays() {\n\tfmt.Println(1)\n\t// Output: 1\n}\n\n"+
		"func TestRenamedImport(x *t.T) {}\n")
	for name, want := range map[string]string{
		"Testhelper":    "a lower-case letter follows Test",
		"TestTakesB":    "a Test function is func(*testing.T)",
		"TestReturns":   "a Test function is func(*testing.T)",
		"FuzzWithT":     "a Fuzz function is func(*testing.F)",
		"ExampleSilent": "an example without an output comment is compiled, never run",
	} {
		docs := map[string]string{"rules/all.md": wholeGroups("E2") + held("internal/mail/odd_test.go", name), "rules/host.md": wholeHost}
		oneProblem(t, checkRules(docs, world), want)
	}
	for _, name := range []string{"ExampleSays", "TestRenamedImport"} {
		docs := map[string]string{"rules/all.md": wholeGroups("E2") + held("internal/mail/odd_test.go", name), "rules/host.md": wholeHost}
		if got := checkRules(docs, world); len(got) > 0 {
			t.Fatalf("%s runs, got %q", name, got)
		}
	}
}

// TestATestNoCheckReachesFails: go test ./... skips a directory starting with
// a dot and a nested module, and a path outside the root is no test of it.
func TestATestNoCheckReachesFails(t *testing.T) {
	world, write := syntheticWorld(t, syntheticAgents)
	test := "package x\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {}\n"
	write(".scratch/probe/x_test.go", test)
	write("_old/x_test.go", test)
	write("tools/testdata/x_test.go", test)
	write("nested/go.mod", "module example.org/nested\n")
	write("nested/inner/x_test.go", test)
	for file, want := range map[string]string{
		".scratch/probe/x_test.go":  "a package no go test of AGENTS.md's checks reaches",
		"_old/x_test.go":            "a package no go test of AGENTS.md's checks reaches",
		"tools/testdata/x_test.go":  "a package no go test of AGENTS.md's checks reaches",
		"nested/inner/x_test.go":    "belongs to the module of nested/go.mod",
		"../outside/x_test.go":      "which is no path inside the module",
		"internal/./mail/x_test.go": "which is no path inside the module",
	} {
		docs := map[string]string{"rules/all.md": wholeGroups("E2") + held(file, "TestX"), "rules/host.md": wholeHost}
		oneProblem(t, checkRules(docs, world), want)
	}
}

func TestATestUnderATagNoCheckBuildsFails(t *testing.T) {
	doc := held("internal/mail/fault_test.go", "TestUnderFault")
	oneProblem(t, rulesProblems(t, doc, "E2"), "a file no go test of AGENTS.md's checks builds")
	docs := map[string]string{"rules/all.md": wholeGroups("E2") + doc, "rules/host.md": wholeHost}
	for agents, want := range map[string]string{
		// The tag on a command scoped to another package builds nothing here.
		"## Checks\n\n```bash\ngo test ./...\ngo test -tags rewakefault ./docs\n```\n":            "a file no go test of AGENTS.md's checks builds",
		"## Checks\n\n```bash\ngo test ./...\ngo test -tags rewakefault ./internal/...\n```\n":    "",
		"## Checks\n\n```bash\ngo test ./... && go test -tags=rewakefault ./internal/mail\n```\n": "",
	} {
		world, _ := syntheticWorld(t, agents)
		got := checkRules(docs, world)
		if want == "" && len(got) > 0 {
			t.Fatalf("%q runs the test, got %q", agents, got)
		} else if want != "" {
			oneProblem(t, got, want)
		}
	}
}

func TestACheckThatMayLeaveATestOutFails(t *testing.T) {
	for command, want := range map[string]string{
		"go test -run Crosswise ./...":    "uses -run",
		"go test -short ./...":            "uses -short",
		"go test github.com/x/y/...":      "names github.com/x/y/..., which is not a directory pattern",
		"go test -race -count=1 ./... -p": "ends with -p and no value",
	} {
		_, got := checksOf("## Checks\n\n```bash\n" + command + "\n```\n")
		oneProblem(t, got, want)
	}
}

func TestTheChecksAreTheFirstBlockOfTheSection(t *testing.T) {
	agents := "# x\n\n## Checks\n\n```bash\ngo vet -tags rewakefault ./...   # vet\nenv -u A \\\n  go test -race ./...\ngo test -tags rewakefault,other ./x\n```\n\n" +
		"```bash\nREWAKE_WORKFLOW=1 go test -run Crosswise ./test/workflow/...\n```\n\n## Later\n\n```bash\ngo test -tags elsewhere ./...\n```\n"
	got, problems := checksOf(agents)
	if len(problems) > 0 {
		t.Fatal(problems)
	}
	if len(got) != 2 || len(got[0].tags) != 0 || !slices.Equal(got[0].patterns, []string{"./..."}) ||
		!slices.Equal(got[1].tags, []string{"rewakefault", "other"}) || !slices.Equal(got[1].patterns, []string{"./x"}) {
		t.Fatalf("got %+v", got)
	}
}

func TestANumberTwiceFails(t *testing.T) {
	oneProblem(t, rulesProblems(t, held("internal/mail/mail_test.go", "TestHeld"), ""), "E2 appears 2 times")
	oneProblem(t, rulesProblems(t, "", "E2"), "E2 is in no document")
	oneProblem(t, rulesProblems(t, "- **E9. More.**\n  Tests:\n  - `internal/mail/mail_test.go` `TestHeld`\n", ""),
		"E9 at docs/rules/all.md:")
}

func TestAGapWithNoStepFails(t *testing.T) {
	oneProblem(t, rulesProblems(t, "- **E2. A rule.**\n  Tests:\n  - Gap: nothing holds it yet.\n", "E2"), "names no step")
	oneProblem(t, rulesProblems(t, "- **E2. A rule.**\n  Tests:\n  - Gap: nothing holds it yet — closed in S40.\n", "E2"),
		"closed in S40, which is no step")
	if got := rulesProblems(t, "- **E2. A rule.**\n  Tests:\n  - Gap: nothing holds it yet — closed in S3.\n", "E2"); len(got) > 0 {
		t.Fatal(got)
	}
}

// TestAnUnnumberedRuleCannotLoseItsTests: the host's rules are known by their
// headings, so deleting a Tests block, or the whole section, fails.
func TestAnUnnumberedRuleCannotLoseItsTests(t *testing.T) {
	world, _ := syntheticWorld(t, syntheticAgents)
	for host, want := range map[string]string{
		strings.Replace(wholeHost, "Tests:\n- `internal/mail/mail_test.go` `TestHeld`\n\n## Grants", "## Grants", 1):         `the rule "The wrapper" of docs/rules/host.md at docs/rules/host.md:3 has no Tests: line`,
		strings.Replace(wholeHost, "- `internal/mail/mail_test.go` `TestHeld`\n\n## Grants", "\n## Grants", 1):               `the rule "The wrapper" of docs/rules/host.md at docs/rules/host.md:3 names no test and no gap`,
		strings.Replace(wholeHost, "## Grants\n\nThe scheme.\n\nTests:\n- `internal/mail/mail_test.go` `TestHeld`\n", "", 1): "rules/host.md#Grants is in no document",
		wholeHost + "\n## Grants\n\nTests:\n- `internal/mail/mail_test.go` `TestHeld`\n":                                     "rules/host.md#Grants appears 2 times",
		wholeHost + "\n## Notes\n\nTests:\n- `internal/mail/mail_test.go` `TestHeld`\n":                                      "a Tests block in no rule",
		strings.Replace(wholeHost, "`TestHeld`\n\n## Grants", "`TestHeld`\n- stray words\n\n## Grants", 1):                   "neither a test nor a gap",
	} {
		oneProblem(t, checkRules(map[string]string{"rules/all.md": wholeGroups(""), "rules/host.md": host}, world), want)
	}
}

func TestAnExampleInAFenceIsNotARule(t *testing.T) {
	if got := rulesProblems(t, "```markdown\n- **E2. An example.**\n  Tests:\n  - `x_test.go` `TestX`\n```\n", ""); len(got) > 0 {
		t.Fatal(got)
	}
}

// TestARaceConstraintFollowsTheCommand: -race adds the race tool tag to every
// file's constraint, set from the command rather than from how this test runs.
func TestARaceConstraintFollowsTheCommand(t *testing.T) {
	files := map[string]string{
		"internal/mail/norace_test.go": "//go:build !race\n\npackage mail\n\nimport \"testing\"\n\nfunc TestNeverInRace(t *testing.T) {}\n",
		"internal/mail/race_test.go":   "//go:build race\n\npackage mail\n\nimport \"testing\"\n\nfunc TestOnlyInRace(t *testing.T) {}\n",
	}
	for _, c := range []struct{ command, refused, runs string }{
		{"go test -race -shuffle=on ./...", "TestNeverInRace", "TestOnlyInRace"},
		{"go test -race=true ./...", "TestNeverInRace", "TestOnlyInRace"},
		{"go test ./...", "TestOnlyInRace", "TestNeverInRace"},
		{"go test -race -race=false ./...", "TestOnlyInRace", "TestNeverInRace"},
	} {
		world, write := syntheticWorld(t, "## Checks\n\n```bash\n"+c.command+"\n```\n")
		for name, text := range files {
			write(name, text)
		}
		file := map[string]string{"TestNeverInRace": "internal/mail/norace_test.go", "TestOnlyInRace": "internal/mail/race_test.go"}
		docs := func(name string) map[string]string {
			return map[string]string{"rules/all.md": wholeGroups("E2") + held(file[name], name), "rules/host.md": wholeHost}
		}
		oneProblem(t, checkRules(docs(c.refused), world), "a file no go test of AGENTS.md's checks builds")
		if got := checkRules(docs(c.runs), world); len(got) > 0 {
			t.Fatalf("%q runs %s, got %q", c.command, c.runs, got)
		}
	}
}

// TestACountThatRunsNoTestFails: -count=0 runs nothing, and the last -count
// is the one go test uses.
func TestACountThatRunsNoTestFails(t *testing.T) {
	for command, refused := range map[string]bool{
		"go test -count=0 ./...":          true,
		"go test -count=1 -count 0 ./...": true,
		"go test -count=x ./...":          true,
		"go test -count=0 -count=2 ./...": false,
		"go test -count 3 -race ./...":    false,
		"go test -race -shuffle=on ./...": false,
	} {
		world, write := syntheticWorld(t, syntheticAgents)
		write("AGENTS.md", "## Checks\n\n```bash\n"+command+"\n```\n")
		world, problems := readWorld(world.root)
		got := checkRules(map[string]string{"rules/all.md": wholeGroups(""), "rules/host.md": wholeHost}, world)
		if !refused {
			if len(problems)+len(got) > 0 {
				t.Fatalf("%q runs every test once at least, got %q %q", command, problems, got)
			}
			continue
		}
		if len(problems) != 2 || !strings.Contains(problems[0], "times, so not once") || !strings.Contains(problems[1], "have no go test command") {
			t.Fatalf("%q: got %q, want the count refused", command, problems)
		}
		if len(got) == 0 || !strings.Contains(got[0], "a package no go test of AGENTS.md's checks reaches") {
			t.Fatalf("%q: every named test passes, got %q", command, got)
		}
	}
}

// TestOnlyTheBuildOrderNamesSteps: a step row in another table, or in a fence,
// is no step, and a build order that cannot be read is a problem.
func TestOnlyTheBuildOrderNamesSteps(t *testing.T) {
	world, write := syntheticWorld(t, syntheticAgents)
	write("docs/v2/stage3.md", "# Stage\n\n## The build order\n\n```markdown\n| S41 | an example |\n```\n\n"+
		"| Step | What |\n|---|---|\n| S1 | tests |\n| S3 | stops |\n\n| S42 | after the table |\n\n"+
		"## An unrelated example\n\n| Step | What |\n|---|---|\n| S40 | not a step |\n\n```markdown\n| S43 | fenced |\n```\n")
	world, problems := readWorld(world.root)
	if len(problems) > 0 || !slices.Equal(world.steps, []string{"S1", "S3"}) {
		t.Fatalf("got %q %q, want the build order's own rows", world.steps, problems)
	}
	for _, step := range []string{"S40", "S41", "S42", "S43"} {
		documents := map[string]string{
			"rules/all.md":  wholeGroups("E2") + "- **E2. A rule.**\n  Tests:\n  - Gap: none yet — closed in " + step + ".\n",
			"rules/host.md": wholeHost,
		}
		oneProblem(t, checkRules(documents, world), "closed in "+step+", which is no step")
	}
	for text, want := range map[string]string{
		"## Elsewhere\n\n| Step | What |\n|---|---|\n| S1 | tests |\n":                           "no table of steps under the heading The build order",
		"## The build order\n\nNo table.\n\n## Next\n\n| Step | What |\n|---|---|\n| S1 | x |\n": "no table of steps under the heading The build order",
		"## The build order\n\n| Name | What |\n|---|---|\n| S1 | x |\n":                         "no table of steps",
		"## The build order\n\n| Step | What |\n| S1 | x |\n":                                    "has no divider row",
		"## The build order\n\n| Step | What |\n|---|---|\n| X1 | x |\n":                         "names no step",
		"## The build order\n\n| Step | What |\n|---|---|\n| S1 | x |\n| S1 | y |\n":             "names S1 twice",
	} {
		write("docs/v2/stage3.md", text)
		if _, problems := readWorld(world.root); len(problems) != 1 || !strings.Contains(problems[0], want) {
			t.Fatalf("%q: got %q, want %q", text, problems, want)
		}
	}
}
