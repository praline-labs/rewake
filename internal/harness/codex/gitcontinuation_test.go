package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

func TestResumedAndForkedSessionsKeepGitWrites(t *testing.T) {
	codexHome(t, "")
	repo := gitRepository(t)
	nested := filepath.Join(repo, "src")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"resume", "fork"} {
		for _, part := range []role.Role{role.Main, role.Write} {
			t.Run(mode+"/"+part.ID, func(t *testing.T) {
				args := []string{mode, "--last", "-C", nested, "-c", rootsKey + `=["/caller"]`}
				plan := gitLaunch(t, part, args...)
				assertGitGrant(t, plan, repo)
				if strings.Join(plan.Args[:len(args)], "\x00") != strings.Join(args, "\x00") {
					t.Fatalf("caller arguments changed: %q", plan.Args)
				}
			})
		}
	}
}

func TestManagedWorktreesExplainTheUnknownPrivateMetadata(t *testing.T) {
	codexHome(t, "")
	_, err := New().Launch(harness.LaunchRequest{Dir: t.TempDir(), Args: []string{"--worktree"}})
	if err == nil || !strings.Contains(err.Error(), "checkout") {
		t.Fatalf("unknown server checkout accepted: %v", err)
	}
}
