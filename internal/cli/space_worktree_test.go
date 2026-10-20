package cli

import (
	"encoding/json"
	"slices"
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

// The worker's worktree is resolved once, from the template and what space
// open was given, and the launch carries exactly that one answer: a typed
// --worktree, on space open or after --, wins; --no-worktree leaves the
// template's out; two typed requests, or one beside --no-worktree, are
// refused.
func TestSpaceOpenResolvesTheWorktreeOnce(t *testing.T) {
	workerAliases(t, worktreeAliases)
	ctx := &Context{table: newTable(adapter.NewCatalog(workerSettingsHarness{}))}
	cases := []struct {
		argv    []string
		want    string
		refused string
	}{
		{[]string{"--alias", "tree"}, "true", ""},
		{[]string{"--alias", "tree", "--worktree=fix"}, "fix", ""},
		{[]string{"--alias", "tree", "--no-worktree"}, "", ""},
		{[]string{"--alias", "tree", "--", "--worktree=after"}, "after", ""},
		{[]string{"--alias", "plain"}, "", ""},
		{[]string{"--alias", "plain", "--worktree"}, "true", ""},
		{[]string{"--alias", "named"}, "named", ""},
		{[]string{"--alias", "named", "--worktree=other"}, "other", ""},
		{[]string{"--alias", "named", "--no-worktree"}, "", ""},
		{[]string{"--alias", "plain", "--worktree", "--", "--worktree=again"}, "", "given both"},
		{[]string{"--alias", "tree", "--no-worktree", "--worktree"}, "", "ask opposite things"},
		{[]string{"--alias", "plain", "--no-worktree", "--", "--worktree"}, "", "ask opposite things"},
	}
	for _, c := range cases {
		parsed, err := ctx.table.parse(append([]string{"space", "open", "api"}, c.argv...))
		if err != nil {
			t.Fatalf("%v: %v", c.argv, err)
		}
		target, err := ctx.spaceTarget(parsed.Call)
		if c.refused != "" {
			if err == nil || !strings.Contains(err.Error(), c.refused) {
				t.Errorf("%v: %v, want a refusal with %q", c.argv, err, c.refused)
			}
			continue
		}
		if err != nil || target.worktree != c.want {
			t.Errorf("%v: worktree %q, %v; want %q", c.argv, target.worktree, err, c.want)
			continue
		}
		words, err := workerLaunch(target, parsed.Call, "fresh-id")
		if err != nil {
			t.Fatalf("%v: %v", c.argv, err)
		}
		var flags []string
		for _, word := range words {
			if strings.HasPrefix(word, "--worktree") {
				flags = append(flags, word)
			}
		}
		want := []string{}
		switch c.want {
		case "":
		case "true":
			want = []string{"--worktree"}
		default:
			want = []string{"--worktree=" + c.want}
		}
		if !slices.Equal(flags, want) {
			t.Errorf("%v: the launch carries %q, want %q: %q", c.argv, flags, want, words)
		}
	}
}

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
