package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

const worktreeAliases = `[alias.tree]
harness = "agent"
worktree = true

[alias.plain]
harness = "agent"

[alias.named]
harness = "agent"
args = ["--worktree=named"]
`

// worktree = true is shown by rewake workers, and refused beside the
// harness's own flag in args, which would be a second answer.
func TestATemplatesWorktreeIsShownAndHasOneAnswer(t *testing.T) {
	workerAliases(t, worktreeAliases)
	t.Setenv("REWAKE_SESSION", "")
	catalog := adapter.NewCatalog(workerSettingsHarness{})
	code, out, errOut := runOn(catalog, "workers", "--json")
	var view workersListing
	if code != ExitOK || json.Unmarshal([]byte(out), &view) != nil {
		t.Fatalf("workers: %d %s %s", code, out, errOut)
	}
	shown := map[string]bool{}
	for _, worker := range view.Workers {
		shown[worker.Name] = worker.Worktree
	}
	if !shown["tree"] || shown["plain"] || shown["named"] {
		t.Fatalf("worktree shown: %v", shown)
	}
	if _, out, _ := runOn(catalog, "workers"); !strings.Contains(out, "tree  harness=agent role=general command=\"agent\" worktree") {
		t.Fatalf("workers lines:\n%s", out)
	}
	workerAliases(t, "[alias.both]\nharness = \"agent\"\nworktree = true\nargs = [\"--worktree\"]\n")
	if code, _, errOut := runOn(catalog, "workers"); code != ExitUsage || !strings.Contains(errOut, "keep one") {
		t.Fatalf("both: %d %s", code, errOut)
	}
}

type enteringHarness struct{ workerSettingsHarness }

func (enteringHarness) EntersWorktree() {}

// enter_worktree = true is shown by rewake workers, and refused beside
// worktree = true and on a harness that cannot enter one; what it resolves to
// is launch's test (TestRaisingResolvesTheWorktreeOnce).
func TestATemplatesEntryIsItsOneCheckout(t *testing.T) {
	workerAliases(t, "[alias.enter]\nharness = \"agent\"\nenter_worktree = true\n")
	t.Setenv("REWAKE_SESSION", "")
	catalog := adapter.NewCatalog(enteringHarness{})
	if _, out, _ := runOn(catalog, "workers"); !strings.Contains(out, "role=general command=\"agent\" enter_worktree") {
		t.Fatalf("workers lines:\n%s", out)
	}
	workerAliases(t, "[alias.both]\nharness = \"agent\"\nworktree = true\nenter_worktree = true\n")
	if code, _, errOut := runOn(catalog, "workers"); code != ExitUsage || !strings.Contains(errOut, "keep one") {
		t.Fatalf("both: %d %s", code, errOut)
	}
	workerAliases(t, "[alias.enter]\nharness = \"agent\"\nenter_worktree = true\n")
	if code, _, errOut := runOn(adapter.NewCatalog(workerSettingsHarness{}), "workers"); code != ExitUsage || !strings.Contains(errOut, "cannot enter a worktree") {
		t.Fatalf("no entry: %d %s", code, errOut)
	}
}
