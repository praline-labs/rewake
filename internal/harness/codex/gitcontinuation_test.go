package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	repo := gitRepository(t)
	for _, part := range []role.Role{role.Main, role.Write} {
		t.Run(part.ID, func(t *testing.T) {
			plan := gitLaunch(t, part, "-C", repo, "--worktree")
			if roots := gitRoots(plan.Args); len(roots) != 0 {
				t.Fatalf("unknown worktree metadata granted: %q", roots)
			}
			note := strings.Join(plan.Notes, " ")
			if !strings.Contains(note, "private") || !strings.Contains(note, "create the worktree first") {
				t.Fatalf("skip lacks the concrete limitation and remedy: %q", plan.Notes)
			}
			if !strings.Contains(strings.Join(plan.Args, " "), "--worktree") {
				t.Fatalf("worktree flag lost: %q", plan.Args)
			}
		})
	}
}
