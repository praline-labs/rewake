package internal

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"slices"
	"strings"
	"testing"
)

// The cases below prove each rule before it is trusted on the tree: a synthetic
// input with one known failure, and the message the rule promises for it.

// pkgImporting is a package of the module whose one file imports other
// packages of the module, named by their paths within it.
func pkgImporting(path, file string, imports ...string) goPackage {
	var full []string
	for _, imported := range imports {
		full = append(full, module+"/"+imported)
	}
	return goPackage{path: path, files: []goFile{{name: path + "/" + file, imports: full}}}
}

func oneMessage(t *testing.T, got []string, want string) {
	t.Helper()
	if len(got) != 1 || !strings.Contains(got[0], want) {
		t.Fatalf("got %q, want one message containing %q", got, want)
	}
}

func edgeMessages(found []edge) []string {
	var messages []string
	for _, e := range found {
		messages = append(messages, e.String())
	}
	return messages
}

func TestAForbiddenEdgeFailsNamingBothPackagesAndTheFile(t *testing.T) {
	graph := []variant{{packages: []goPackage{
		pkgImporting("internal/core/mail", "mail.go", "internal/host", "internal/infra/state"),
		pkgImporting("internal/host", "host.go", "internal/core/mail"),
		{path: "internal/infra/state"},
	}}}
	found, unplaced := judgeImports(graph, nil)
	if len(unplaced) > 0 {
		t.Fatal(unplaced)
	}
	oneMessage(t, edgeMessages(found), "internal/core/mail/mail.go: internal/core/mail (core) imports internal/host (host), which the import rule forbids")
}

func TestAnEdgeOnlyATagBuildsFailsNamingTheTag(t *testing.T) {
	graph := []variant{
		{packages: []goPackage{pkgImporting("internal/infra/state", "fault.go"), {path: "internal/core/mail"}}},
		{build: build{tags: []string{"rewakefault"}}, packages: []goPackage{pkgImporting("internal/infra/state", "fault.go", "internal/core/mail"), {path: "internal/core/mail"}}},
	}
	found, _ := judgeImports(graph, nil)
	oneMessage(t, edgeMessages(found), "internal/infra/state/fault.go: internal/infra/state (infra) imports internal/core/mail (core), which the import rule forbids (docs/v2/design.md#the-import-rule) (only under -tags rewakefault)")
}

func TestAPackageThatStaysImportsNothingAStepRemoves(t *testing.T) {
	table := map[string]transit{"internal/old": {layer: core, step: "S2", goes: true}}
	graph := []variant{{packages: []goPackage{pkgImporting("internal/core/mail", "mail.go", "internal/old"), {path: "internal/old"}}}}
	found, _ := judgeImports(graph, table)
	oneMessage(t, edgeMessages(found), "imports internal/old (core, goes in S2)")
}

func TestAPackageInNoPlaceFails(t *testing.T) {
	_, unplaced := judgeImports([]variant{{packages: []goPackage{{path: "internal/elsewhere"}}}}, nil)
	oneMessage(t, unplaced, "internal/elsewhere is neither at a 2.0 path nor in the transition table")
}

func TestOneAdapterMayNotImportAnother(t *testing.T) {
	if allowed(api+"/one", api+"/two") || !allowed(api+"/one", api+"/one") || !allowed(catalog, api+"/two") {
		t.Fatal("an adapter imports only itself among the adapters; the catalog imports them all")
	}
}

func TestANameFailsInACommentAStringAndAnIdentifier(t *testing.T) {
	src := `package mail

import "example.com/relay"

// wait as Relay does
var relayMode = "on relay"

var _ = relay.X
`
	found, err := findMentions("internal/core/mail/mail.go", []byte(src), []string{"relay"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range found {
		got = append(got, m.String())
	}
	slices.Sort(got)
	want := []string{
		`internal/core/mail/mail.go:5: the comment names the harness word "relay"`,
		`internal/core/mail/mail.go:6: the identifier names the harness word "relay"`,
		`internal/core/mail/mail.go:6: the string names the harness word "relay"`,
		`internal/core/mail/mail.go:8: the identifier names the harness word "relay"`,
	}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q: the import path is rule 1's and is not a mention", got, want)
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Fatalf("got %q, want %q", got[i], want[i])
		}
	}
}

// TestAStringIsReadByItsValue: an escape spells the same name, a decoded
// newline is not a line of the source, and a raw string's lines are.
func TestAStringIsReadByItsValue(t *testing.T) {
	src := "package mail\n\nvar a = \"\\x72elay\"\n\nvar b = \"one\\nrelay\"\n\nvar c = `one\ntwo relay`\n"
	found, err := findMentions("internal/core/mail/mail.go", []byte(src), []string{"relay"})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range found {
		got = append(got, fmt.Sprintf("%d %s %q", m.line, m.kind, m.text))
	}
	want := []string{`3 string "relay"`, `5 string "one\nrelay"`, `8 string "one\ntwo relay"`}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAWordContainingAnotherFailsOnce(t *testing.T) {
	got := harnessWords([]string{"claude", "Claude Code", "relay", "Relay"}, ownWords)
	want := []string{".agents", ".codex", "app-server", "claude", "mcp__", "relay"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAnInitThatRegistersAnAdapterFails(t *testing.T) {
	src := "package fake\n\nfunc init() { adapter.Register(fake{}) }\n"
	found, err := findInits("internal/adapter/fake/fake.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || !strings.HasPrefix(found[0].String(), "internal/adapter/fake/fake.go:3: func init in a package under adapter") {
		t.Fatalf("got %v", found)
	}
}

func TestAPackageLevelVariableHoldingAdaptersFails(t *testing.T) {
	src := `package fake

type Adapter interface{ ID() string }

type one struct{}

func (one) ID() string { return "one" }

var all []Adapter

var byName = map[string]*one{}

var _ Adapter = one{}

var count int
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fake.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	info := &types.Info{Defs: map[*ast.Ident]types.Object{}}
	checked, err := new(types.Config).Check("fake", fset, []*ast.File{file}, info)
	if err != nil {
		t.Fatal(err)
	}
	iface := checked.Scope().Lookup("Adapter").Type().Underlying().(*types.Interface)
	var got []string
	for _, r := range findAdapterVars(fset, []*ast.File{file}, info, []*types.Interface{iface}) {
		got = append(got, r.String())
	}
	if len(got) != 2 || !strings.HasPrefix(got[0], "fake.go:9: package-level variable all holds adapters") ||
		!strings.HasPrefix(got[1], "fake.go:11: package-level variable byName holds adapters") {
		t.Fatalf("got %q", got)
	}
}

func TestAnExceptionNothingMatchesFails(t *testing.T) {
	x := importException{from: "internal/cli", to: "internal/harness/claude", files: []string{"internal/cli/a.go", "internal/cli/b.go"}}
	found := []edge{{file: "internal/cli/a.go", from: "internal/cli", to: "internal/harness/claude"}}
	oneMessage(t, unmatchedImports([]importException{x}, found), "the import exception internal/cli -> internal/harness/claude for internal/cli/b.go matches nothing any more")

	names := map[nameException]excuse{{file: "internal/cli/a.go", word: "relay"}: {"why", "S8", 1, "000000000000"}}
	oneMessage(t, excused([]mention{}, nameKey, names), `the name exception for "relay" in internal/cli/a.go matches nothing any more`)

	left := excused([]mention{{file: "internal/cli/b.go", line: 3, word: "relay", kind: "comment"}}, nameKey, map[nameException]excuse{})
	oneMessage(t, left, `internal/cli/b.go:3: the comment names the harness word "relay"`)
}

// TestATransitionEntryAtItsPlaceOrGoneFails takes its steps from the real build
// order, so a step that looks like one but is not, S40, fails like "later".
func TestATransitionEntryAtItsPlaceOrGoneFails(t *testing.T) {
	table := map[string]transit{
		"internal/core/mail": {layer: core, step: "S13"},
		"internal/vanished":  {layer: core, step: "S13"},
		"internal/inbox":     {layer: core, step: "later"},
		"internal/receipt":   {layer: core, step: "S40"},
	}
	got := staleTransitions(table, []string{"internal/core/mail", "internal/inbox", "internal/receipt"}, buildSteps(t))
	want := []string{
		`the transition entry of internal/inbox names the step "later", which is no step of docs/v2/stage3.md#the-build-order`,
		`the transition entry of internal/receipt names the step "S40", which is no step of docs/v2/stage3.md#the-build-order`,
		"the transition table maps internal/core/mail, which is at its 2.0 path: remove the entry",
		"the transition table maps internal/vanished, which is gone: remove the entry",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
