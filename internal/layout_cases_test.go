package internal

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
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
	found, unplaced := judgeImports(graph)
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
	found, _ := judgeImports(graph)
	oneMessage(t, edgeMessages(found), "internal/infra/state/fault.go: internal/infra/state (infra) imports internal/core/mail (core), which the import rule forbids (docs/v2/design.md#the-import-rule) (only under -tags rewakefault)")
}

func TestAConfinedLibraryIsImportedByItsPackageAlone(t *testing.T) {
	library := "github.com/charmbracelet/x/vt"
	graph := []variant{{packages: []goPackage{
		{path: "internal/host/terminal", files: []goFile{{name: "internal/host/terminal/vt.go", imports: []string{library}}}},
		{path: "internal/host/keeper", files: []goFile{{name: "internal/host/keeper/screen.go", imports: []string{library}}}},
		{path: "internal/core/mail", files: []goFile{{name: "internal/core/mail/mail.go", imports: []string{"github.com/pelletier/go-toml/v2"}}}},
	}}}
	found, _ := judgeImports(graph)
	oneMessage(t, edgeMessages(found), "internal/host/keeper/screen.go: internal/host/keeper imports github.com/charmbracelet/x/vt, which only internal/host/terminal may import")
}

func TestAPackageInNoPlaceFails(t *testing.T) {
	_, unplaced := judgeImports([]variant{{packages: []goPackage{{path: "internal/elsewhere"}}}})
	oneMessage(t, unplaced, "internal/elsewhere is at no 2.0 path")
}

func TestOneAdapterMayNotImportAnother(t *testing.T) {
	if allowed(api+"/one", api+"/two") || !allowed(api+"/one", api+"/one") || !allowed(catalog, api+"/two") {
		t.Fatal("an adapter imports only itself among the adapters; the catalog imports them all")
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

type adapterSet struct {
	name     string
	adapters []Adapter
}

var kept = adapterSet{}

type node struct{ next *node }

var list node
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
	if len(got) != 3 || !strings.HasPrefix(got[0], "fake.go:9: package-level variable all holds adapters") ||
		!strings.HasPrefix(got[1], "fake.go:11: package-level variable byName holds adapters") ||
		!strings.HasPrefix(got[2], "fake.go:22: package-level variable kept holds adapters") {
		t.Fatalf("got %q", got)
	}
}
