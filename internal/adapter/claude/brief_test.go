package claude

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/role"
)

// The role texts name no harness, so how a session on Claude Code commits
// comes from this adapter: a role that may commit is told it does so within
// its own permissions, and a role that may not hears nothing of it
// (docs/v2/design-api.md#launch).
func TestABriefingCarriesTheCommitWordsOfARoleThatMayCommit(t *testing.T) {
	for _, part := range role.All() {
		plan, err := New().Launch(adapter.LaunchRequest{
			Name: "api", Dir: t.TempDir(), Role: part, Intro: true,
			Socket: filepath.Join(t.TempDir(), "api.sock"),
		})
		if err != nil {
			t.Fatal(err)
		}
		at := indexOf(plan.Args, introFlag)
		if at < 0 || at+1 >= len(plan.Args) {
			t.Fatalf("%s: no briefing in %v", part.ID, plan.Args)
		}
		told := strings.Contains(plan.Args[at+1], "Claude Code takes no --grant-git")
		if told != part.GitWrite {
			t.Fatalf("%s, which may commit: %v, was told the commit words: %v", part.ID, part.GitWrite, told)
		}
	}
}
