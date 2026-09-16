package codex

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/role"
)

func fixtureGit(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git: %v %s", err, out)
	}
	return strings.TrimSpace(string(out))
}
func TestGitPointersCannotGrantOrdinaryDirectories(t *testing.T) {
	for _, kind := range []string{"gitdir", "commondir"} {
		t.Run(kind, func(t *testing.T) {
			codexHome(t, "")
			base := t.TempDir()
			repo := filepath.Join(base, "checkout")
			os.Mkdir(repo, 0700)
			if kind == "gitdir" {
				writeGitPointer(t, filepath.Join(repo, ".git"), "gitdir: ..\n")
			} else {
				fixtureGit(t, repo, "init", "-q")
				writeGitPointer(t, filepath.Join(repo, ".git", "commondir"), "../..\n")
			}
			cmd := exec.Command("git", "-C", repo, "rev-parse", "--absolute-git-dir")
			out, err := cmd.CombinedOutput()
			t.Logf("Git validation: err=%v output=%q", err, out)
			if err == nil {
				t.Fatal("fixture unexpectedly accepted by Git")
			}
			plan := gitLaunch(t, role.Write, "-C", repo)
			roots := gitRoots(plan.Args)
			t.Logf("granted roots=%q notes=%q", roots, plan.Notes)
			if len(roots) > 0 {
				t.Error("invalid metadata pointer expands write access outside checkout")
			}
		})
	}
}
func TestGitMetadataIsFoundFromSubdirectories(t *testing.T) {
	codexHome(t, "")
	repo := t.TempDir()
	fixtureGit(t, repo, "init", "-q")
	nested := filepath.Join(repo, "src")
	os.Mkdir(nested, 0700)
	want := fixtureGit(t, nested, "rev-parse", "--absolute-git-dir")
	plan := gitLaunch(t, role.Write, "-C", nested)
	roots := gitRoots(plan.Args)
	t.Logf("roots=%q expected=%s notes=%q", roots, want, plan.Notes)
	if len(roots) != 1 || roots[0] != want {
		t.Error("repository metadata not granted from its subdirectory")
	}
}

func TestInvalidNestedMetadataDoesNotFallBackToTheParent(t *testing.T) {
	codexHome(t, "")
	repo := gitRepository(t)
	nested := filepath.Join(repo, "nested")
	writeGitPointer(t, filepath.Join(nested, ".git"), "gitdir: ..\n")
	plan := gitLaunch(t, role.Write, "-C", nested)
	if len(gitRoots(plan.Args)) != 0 {
		t.Fatalf("broken nested checkout granted outer metadata: %q", plan.Args)
	}
}

func TestSharedMetadataRequiresAllStructuralMarkers(t *testing.T) {
	for _, missing := range []string{"HEAD", "objects", "refs"} {
		t.Run(missing, func(t *testing.T) {
			codexHome(t, "")
			repo := gitRepository(t)
			if err := os.Remove(filepath.Join(repo, ".git", missing)); err != nil {
				t.Fatal(err)
			}
			if roots := gitRoots(gitLaunch(t, role.Write, "-C", repo).Args); len(roots) != 0 {
				t.Fatalf("metadata without %s granted: %q", missing, roots)
			}
		})
	}
}
