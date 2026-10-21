package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The worker's kind of work is resolved at space open: a typed --type wins,
// --no-type leaves the template's out, else the template's type; one that is
// not there, or that an alias file turned off, is refused before anything
// starts, saying where.
func TestSpaceOpenResolvesTheType(t *testing.T) {
	workerAliases(t, "[types]\noff = [\"process\"]\n\n[alias.coder]\nharness = \"agent\"\ntype = \"code\"\n\n"+
		"[alias.plain]\nharness = \"agent\"\n\n[alias.stale]\nharness = \"agent\"\ntype = \"process\"\n")
	project := filepath.Join(".rewake", "types", "study.md")
	if err := os.MkdirAll(filepath.Dir(project), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("+++\ndescription = \"ours\"\n+++\nthe project's study\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := &Context{table: newTable(adapter.NewCatalog(workerSettingsHarness{}))}
	for _, c := range []struct {
		argv          []string
		want, refused string
	}{
		{[]string{"--alias", "coder"}, "code", ""},
		{[]string{"--alias", "coder", "--type", "study"}, "study", ""},
		{[]string{"--alias", "coder", "--no-type"}, "", ""},
		{[]string{"--alias", "plain"}, "", ""},
		{[]string{"--alias", "plain", "--type", "review"}, "review", ""},
		{[]string{"--alias", "plain", "--type", "process"}, "", "turned off in the user alias file"},
		{[]string{"--alias", "plain", "--type", "nothing"}, "", `no type "nothing"`},
		{[]string{"--alias", "stale"}, "", `alias "stale" names type = "process"`},
		{[]string{"--alias", "stale", "--no-type"}, "", ""},
		{[]string{"--alias", "coder", "--type", "study", "--no-type"}, "", "ask opposite things"},
	} {
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
		if err != nil || target.work.Name != c.want || (c.want != "" && target.work.Text == "") {
			t.Errorf("%v: %+v %v, want type %q", c.argv, target.work, err, c.want)
		}
	}
}

// --type and --no-type are space open's: list and close refuse them, as
// they refuse the other flags only open takes, rather than list or close as
// if they were not there.
func TestTheTypeFlagsBelongToSpaceOpen(t *testing.T) {
	// A room with a build, so that close, a change, reaches its handler.
	liveSessionAs(t, "main-"+agentID, "main")
	t.Setenv(state.SessionEnv, "main-"+agentID)
	for _, argv := range [][]string{
		{"space", "list", "--type", "review"},
		{"space", "list", "--no-type"},
		{"space", "close", "api", "--type", "review"},
		{"space", "close", "api", "--no-type"},
	} {
		code, _, errOut := run(argv...)
		if code != ExitUsage || !strings.Contains(errOut, "belongs to space open") {
			t.Errorf("%v: exit %d, %s", argv, code, errOut)
		}
	}
}
