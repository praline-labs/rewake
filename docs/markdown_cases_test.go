package docs

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestTheMarkdownReaderIsOne holds the two copies of the Markdown reader to one
// text, so the layout test and the rules test never read a fence or a build
// order differently.
func TestTheMarkdownReaderIsOne(t *testing.T) {
	var bodies [][]byte
	for _, name := range []string{"markdown_test.go", filepath.Join(moduleRoot, "internal", "layout_markdown_test.go")} {
		text, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		_, body, _ := bytes.Cut(text, []byte("\n"))
		bodies = append(bodies, body)
	}
	if !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatal("docs/markdown_test.go and internal/layout_markdown_test.go differ below the package clause: change both alike")
	}
}

const table40 = "| Step | What |\n|---|---|\n| S40 | an example |\n"

// TestAnExampleInAnyFenceNamesNoStep: a fence is closed only by a run of its own
// character at least as long, so a tilde fence, or a longer backtick fence
// holding a shorter one, keeps its example out of the build order and the gap
// it would allow fails.
func TestAnExampleInAnyFenceNamesNoStep(t *testing.T) {
	for _, example := range []string{
		"~~~markdown\n" + table40 + "~~~\n",
		"````markdown\n```\n" + table40 + "```\n````\n",
		"~~~\n```\n" + table40 + "```\n~~~\n",
		"```\n```go\n" + table40 + "```\n",
	} {
		world, write := syntheticWorld(t, syntheticAgents)
		write("docs/v2/stage3.md", "# Stage\n\n## The build order\n\n"+example+"\n| Step | What |\n|---|---|\n| S1 | tests |\n")
		world, problems := readWorld(world.root)
		if len(problems) > 0 || !slices.Equal(world.steps, []string{"S1"}) {
			t.Fatalf("%q: got %q %q, want only S1", example, world.steps, problems)
		}
		documents := map[string]string{
			"rules/all.md":  wholeGroups("E2") + "- **E2. A rule.**\n  Tests:\n  - Gap: not tested — closed in S40.\n",
			"rules/host.md": wholeHost,
		}
		oneProblem(t, checkRules(documents, world), "closed in S40, which is no step")
	}
}

// TestARepeatedHeadingDoesNotReopenTheBuildOrder: the first heading of the
// title is the build order, and a later one of the same title is not read.
func TestARepeatedHeadingDoesNotReopenTheBuildOrder(t *testing.T) {
	world, write := syntheticWorld(t, syntheticAgents)
	write("docs/v2/stage3.md", "## The build order\n\nNo table.\n\n## Later\n\n## The build order\n\n"+table40)
	if _, problems := readWorld(world.root); len(problems) != 1 || !strings.Contains(problems[0], "no table of steps under the heading The build order") {
		t.Fatalf("got %q, want the first section's missing table", problems)
	}
}

// TestWhatTheReaderCannotClassifyIsRefused: a form the reader cannot place
// without modeling the containers around it fails, whichever document it is
// in, rather than being read as prose or skipped as code.
func TestWhatTheReaderCannotClassifyIsRefused(t *testing.T) {
	cases := map[string]string{
		"~~~\n" + table40:                                "the fence of line 3 is never closed",
		"\t```\nx\n```\n":                                "a tab before a fence",
		"    ```\nx\n```\n":                              "a fence indented 4 spaces",
		"```go`\n" + table40:                             "a backtick after its run",
		"  ```\n  x\n```\n":                              "closes the fence of line 3 at another indentation",
		"  ```\nx\n  ```\n":                              "less indented than the fence of line 3",
		"<!--\n" + table40 + "-->\n":                     "an HTML comment",
		"Example\n-------\n\n" + table40:                 "into a heading this check does not read",
		"  | Step | What |\n  |---|---|\n  | S1 | x |\n": "an indented table under The build order",
	}
	for text, want := range cases {
		world, write := syntheticWorld(t, syntheticAgents)
		write("docs/v2/stage3.md", "## The build order\n\n"+text)
		if _, problems := readWorld(world.root); len(problems) != 1 || !strings.Contains(problems[0], want) {
			t.Fatalf("stage3.md %q: got %q, want %q", text, problems, want)
		}
	}
	world, write := syntheticWorld(t, syntheticAgents)
	write("AGENTS.md", syntheticAgents+"\n~~~\nunclosed\n")
	if _, problems := readWorld(world.root); len(problems) != 2 || !strings.Contains(problems[0], "AGENTS.md: the fence of line 10 is never closed") {
		t.Fatalf("AGENTS.md: got %q, want the fence refused", problems)
	}
	documents := map[string]string{"rules/all.md": wholeGroups(""), "rules/host.md": wholeHost + "\n<!-- hidden -->\n"}
	got := checkRules(documents, world)
	if !slices.ContainsFunc(got, func(p string) bool { return strings.HasPrefix(p, "docs/rules/host.md: line 17: an HTML comment") }) {
		t.Fatalf("host.md: got %q, want the comment refused", got)
	}
}

// TestAnExampleOfTheRulesIsNoRule: rules and Tests blocks inside any fence are
// an example, so the rules they show are missing, not stated.
func TestAnExampleOfTheRulesIsNoRule(t *testing.T) {
	world, _ := syntheticWorld(t, syntheticAgents)
	documents := map[string]string{
		"rules/all.md":  wholeGroups(""),
		"rules/host.md": "~~~markdown\n" + wholeHost + "\n## End of the example\n~~~\n",
	}
	got := checkRules(documents, world)
	if len(got) != 2 || !strings.Contains(got[0], "rules/host.md#Grants is in no document") || !strings.Contains(got[1], "rules/host.md#The wrapper is in no document") {
		t.Fatalf("got %q, want both host rules missing", got)
	}
	documents = map[string]string{
		"rules/all.md":  wholeGroups("E2") + "````markdown\n```\n" + held("internal/mail/mail_test.go", "TestHeld") + "```\n````\n",
		"rules/host.md": wholeHost,
	}
	oneProblem(t, checkRules(documents, world), "E2 is in no document")
	documents["rules/all.md"] = wholeGroups("E2") + "- **E2. A rule.**\n\n      Tests:\n      - `internal/mail/mail_test.go` `TestHeld`\n"
	oneProblem(t, checkRules(documents, world), "E2 at docs/rules/all.md")
	documents["rules/all.md"] = wholeGroups("E2") + "- **E2. A rule.**\n  Tests:\n      - `internal/mail/mail_test.go` `TestHeld`\n"
	oneProblem(t, checkRules(documents, world), "is neither a test nor a gap")
}

// TestOnlyThePlainGoTestFormIsRead: a prefix other than env -u can change which
// tests run without a flag after go test showing it, so it is refused.
func TestOnlyThePlainGoTestFormIsRead(t *testing.T) {
	for command, refused := range map[string]bool{
		"env GOFLAGS=-count=0 go test ./...":        true,
		"GOFLAGS=-run=X go test ./...":              true,
		"env -u A GOFLAGS=-short go test ./...":     true,
		"timeout 600 go test ./...":                 true,
		"env -i go test ./...":                      true,
		"go -C internal test ./...":                 true,
		"/usr/local/go/bin/go test ./...":           true,
		"env -u A -u B go test -race ./...":         false,
		"env go test ./...":                         false,
		"go vet ./... && go test -shuffle=on ./...": false,
	} {
		world, write := syntheticWorld(t, syntheticAgents)
		// A sentence about a command is no command.
		write("AGENTS.md", "## Checks\n\nProse: go test -run X ./... is no check.\n\n```bash\n"+command+"\n```\n")
		world, problems := readWorld(world.root)
		got := checkRules(map[string]string{"rules/all.md": wholeGroups(""), "rules/host.md": wholeHost}, world)
		if !refused {
			if len(problems)+len(got) > 0 {
				t.Fatalf("%q is the plain form, got %q %q", command, problems, got)
			}
			continue
		}
		if len(problems) != 2 || !strings.Contains(problems[0], "runs go test other than as go test") || !strings.Contains(problems[1], "have no go test command") {
			t.Fatalf("%q: got %q, want the form refused", command, problems)
		}
		if len(got) == 0 || !strings.Contains(got[0], "a package no go test of AGENTS.md's checks reaches") {
			t.Fatalf("%q: every named test passes, got %q", command, got)
		}
	}
}

// TestALinkInAnyFenceIsNoLink: the map reads fences with the same reader.
func TestALinkInAnyFenceIsNoLink(t *testing.T) {
	text, err := withoutCode("~~~\n[gone](gone.md)\n```\n~~~\n[kept](kept.md)\n")
	if err != nil || strings.Contains(text, "gone.md") || !strings.Contains(text, "kept.md") {
		t.Fatalf("got %q %v, want only the link outside the fence", text, err)
	}
	if _, err := withoutCode("```\n[x](x.md)\n"); err == nil {
		t.Fatal("an unclosed fence is read, want it refused")
	}
}
