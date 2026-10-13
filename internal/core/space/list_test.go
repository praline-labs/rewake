package space

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/infra/build/buildtest"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A kept checkout is reminded while it is there (SP7): with what was found
// in it and the Git commands that clear it, unlocking first a checkout a dead
// keeper left locked. A checkout that cannot be looked at is reminded too;
// one seen gone is not, before any change records it cleared.
func TestAKeptCheckoutIsRemindedUntilItIsGone(t *testing.T) {
	dir := buildtest.Dir(t)
	seated(t, dir, "open")
	gone, goneCheckout := seated(t, dir, "gone")
	if _, err := Close(dir, gone.ID, observed(WorkerEnded, Seen{})); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(goneCheckout); err != nil {
		t.Fatal(err)
	}
	kept, keptCheckout := seated(t, dir, "kept")
	if _, err := Close(dir, kept.ID, observed(WorkerEnded, Seen{Left: true, Changes: true, Commits: true, Locked: true})); err != nil {
		t.Fatal(err)
	}
	removed, removedCheckout := seated(t, dir, "removed")
	if _, err := Close(dir, removed.ID, observed(WorkerEnded, Seen{Left: true, Commits: true})); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(removedCheckout); err != nil {
		t.Fatal(err)
	}

	records, problems, err := List(dir)
	if err != nil || len(problems) != 0 {
		t.Fatalf("list: %v %v", problems, err)
	}
	var names []string
	for _, r := range records {
		names = append(names, r.Name+":"+string(r.State))
	}
	if want := []string{"open:open", "gone:closed", "kept:kept", "removed:kept"}; !slices.Equal(names, want) {
		t.Fatalf("listed %v, want %v", names, want)
	}
	reminders := Reminders(records)
	if len(reminders) != 1 || reminders[0].ID != kept.ID || reminders[0].Presence != There || !reminders[0].Found.Locked {
		t.Fatalf("reminders %+v, want the kept checkout alone", reminders)
	}
	want := []string{
		"git worktree unlock " + keptCheckout,
		"git worktree remove " + keptCheckout,
		"git branch -d worktree-agent-kept",
	}
	if !slices.Equal(reminders[0].Clear, want) {
		t.Fatalf("clear %q, want %q", reminders[0].Clear, want)
	}

	if err := os.Remove(keptCheckout); err != nil {
		t.Fatal(err)
	}
	records, _, _ = List(dir)
	if reminders := Reminders(records); len(reminders) != 0 {
		t.Fatalf("a checkout seen gone is still reminded: %+v", reminders)
	}
}

// A checkout whose presence cannot be told is reminded, never forgotten.
func TestAnUnknownCheckoutIsReminded(t *testing.T) {
	parent := t.TempDir()
	checkout := filepath.Join(parent, "checkout")
	if err := os.Mkdir(checkout, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o700) })
	if _, err := os.Lstat(checkout); err == nil {
		t.Skip("this user reads past a directory's mode")
	}
	r := Record{State: Kept, Name: "x", Checkout: Checkout{Path: checkout}, End: &End{Checkout: Seen{Left: true}}}
	if got := Reminders([]Record{r}); len(got) != 1 || got[0].Presence != Unknown {
		t.Fatalf("reminders %+v, want the checkout unknown and reminded", got)
	}
}

// A word the shell would split or expand is quoted in a command to clear.
func TestTheCommandsToClearQuoteTheirWords(t *testing.T) {
	checkout := filepath.Join(t.TempDir(), "a b", "it's")
	if err := os.MkdirAll(checkout, 0o700); err != nil {
		t.Fatal(err)
	}
	r := Record{State: Kept, Name: "x", Checkout: Checkout{Path: checkout}, End: &End{Checkout: Seen{Left: true}}}
	quoted := "'" + strings.ReplaceAll(checkout, "'", `'\''`) + "'"
	if got := Reminders([]Record{r}); len(got) != 1 || !slices.Equal(got[0].Clear, []string{"git worktree remove " + quoted}) {
		t.Fatalf("reminders %+v, want the path quoted as %s", got, quoted)
	}
}

// The listing reads as a view does: it writes nothing, takes no lock and
// names apart what does not read; a room with no spaces has none.
func TestTheListingWritesNothing(t *testing.T) {
	dir := buildtest.Dir(t)
	if records, problems, err := List(dir); records != nil || problems != nil || err != nil {
		t.Fatalf("a room with no spaces: %v %v %v", records, problems, err)
	}
	record, _ := seated(t, dir, "api")
	if err := os.WriteFile(state.SpacePath(dir, strings.Repeat("cd", 16)), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state.SpacesPath(dir), "stray.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	before := tree(t, dir)
	leave := state.EnterView()
	records, problems, err := List(dir)
	_, loadErr := Load(dir, record.ID)
	if err := leave(); err != nil {
		t.Fatalf("the listing tried to write: %v", err)
	}
	if err != nil || loadErr != nil || len(records) != 1 || records[0].ID != record.ID || len(problems) != 2 {
		t.Fatalf("listed %v, problems %v, %v %v", records, problems, err, loadErr)
	}
	if after := tree(t, dir); after != before {
		t.Fatalf("the listing changed the room:\n%s\n%s", before, after)
	}
}

func tree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		lines = append(lines, path+" "+info.Mode().String()+" "+strconv.FormatInt(info.Size(), 10)+" "+info.ModTime().String())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(lines, "\n")
}

// rewake removes no checkout (SP7): the package that keeps the records and
// names the commands to clear a kept checkout runs no command and removes no
// file, so a reminder can only be read, never carried out by it.
func TestThePackageRemovesNoCheckout(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	set := token.NewFileSet()
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(set, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			if path, _ := strconv.Unquote(spec.Path.Value); path == "os/exec" || path == "syscall" {
				t.Errorf("%s imports %s", name, path)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if selector, ok := call.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "os" && strings.HasPrefix(selector.Sel.Name, "Remove") {
					t.Errorf("%s calls os.%s at %s", name, selector.Sel.Name, set.Position(call.Pos()))
				}
				if pkg, ok := selector.X.(*ast.Ident); ok && pkg.Name == "state" && (strings.HasPrefix(selector.Sel.Name, "Remove") || selector.Sel.Name == "Discard") {
					t.Errorf("%s calls state.%s at %s", name, selector.Sel.Name, set.Position(call.Pos()))
				}
			}
			return true
		})
	}
}

// Whether a main still runs is told only from its own pid namespace: a
// record of another, or of none, is unknown whatever its pid says here.
func TestAMainRunIsJudgedOnlyFromItsNamespace(t *testing.T) {
	t.Cleanup(func() { observeRun, namespace = registry.ObserveRun, proc.Namespace })
	namespace = func() string { return "pid:[1]" }
	observeRun = func(string) proc.IdentityEvidence { return proc.IdentityEnded }
	for ns, want := range map[string]proc.IdentityEvidence{"pid:[1]": proc.IdentityEnded, "pid:[2]": proc.IdentityUnknown, "": proc.IdentityUnknown} {
		if got := MainRuns(Record{Main: Run{Name: "lead", Run: "1.1", PIDNamespace: ns}}); got != want {
			t.Errorf("a main of namespace %q: %v, want %v", ns, got, want)
		}
	}
	namespace = func() string { return "" }
	if got := MainRuns(Record{Main: Run{Run: "1.1"}}); got != proc.IdentityUnknown {
		t.Errorf("a reader that cannot tell its own namespace: %v", got)
	}
}
