package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

func gitRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	populateGitMetadata(t, filepath.Join(dir, ".git"), true)
	return dir
}

func gitLaunch(t *testing.T, part role.Role, args ...string) harness.LaunchPlan {
	t.Helper()
	plan, err := New().Launch(harness.LaunchRequest{Name: "writer", Dir: t.TempDir(), Role: part, Args: args, Intro: true})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func gitRoots(args []string) []string {
	var roots []string
	visible := harness.BeforeTerminator(args)
	for i := 0; i < len(visible); i++ {
		if visible[i] == "--add-dir" && i+1 < len(visible) {
			i++
			roots = append(roots, visible[i])
		} else if strings.HasPrefix(visible[i], "--add-dir=") {
			roots = append(roots, strings.TrimPrefix(visible[i], "--add-dir="))
		}
	}
	return roots
}

func assertNoImplicitGitGrant(t *testing.T, plan harness.LaunchPlan, directory string) {
	t.Helper()
	roots := gitRoots(plan.Args)
	if plan.Backend.(*serverSession).cwd != directory {
		t.Fatalf("effective cwd changed: %s", plan.Backend.(*serverSession).cwd)
	}
	if len(roots) != 0 {
		t.Fatalf("args=%q notes=%q; no implicit Git roots allowed for %s", plan.Args, plan.Notes, directory)
	}
}

func TestNoRoleAutomaticallyReceivesGitWrites(t *testing.T) {
	codexHome(t, "")
	repo := gitRepository(t)
	for _, part := range []role.Role{role.General, {}, role.Main, role.Write} {
		t.Run(part.ID, func(t *testing.T) {
			plan := gitLaunch(t, part, "-C", repo)
			granted := len(gitRoots(plan.Args)) > 0
			if granted {
				t.Fatalf("role=%+v args=%q notes=%q", part, plan.Args, plan.Notes)
			}
			assertNoImplicitGitGrant(t, plan, repo)
			_, notify := configValue(plan.Args, notifyKey)
			if notify {
				t.Errorf("role=%s notify=%v", part.ID, notify)
			}
			if part.ID == "write" {
				intro, _ := configValue(plan.Args, introKey)
				if !strings.Contains(intro, "commit changes") || !strings.Contains(intro, "end your turn") {
					t.Errorf("writer intro=%s", intro)
				}
			}
		})
	}
}

func TestGitWritesFollowTheEffectiveWorkingDirectory(t *testing.T) {
	codexHome(t, "")
	base := t.TempDir()
	t.Chdir(base)
	repo := filepath.Join(base, "repo with \"quotes\"")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	populateGitMetadata(t, filepath.Join(repo, ".git"), true)
	for _, args := range [][]string{
		{"-C", repo}, {"--cd", repo}, {"--cd=" + repo}, {"-C" + repo}, {"-C=" + repo}, {"-C", filepath.Base(repo)},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) { assertNoImplicitGitGrant(t, gitLaunch(t, role.Write, args...), repo) })
	}
	t.Chdir(repo)
	assertNoImplicitGitGrant(t, gitLaunch(t, role.Write), repo)
}

func TestGitFlagsStayBeforeThePrompt(t *testing.T) {
	codexHome(t, "")
	repo := gitRepository(t)
	prompt := []string{"--", "-C", "/elsewhere", "-pwork", "-c", rootsKey + `=["/elsewhere"]`, "resume"}
	args := append([]string{"-C", repo}, prompt...)
	plan := gitLaunch(t, role.Write, args...)
	assertNoImplicitGitGrant(t, plan, repo)
	if got := plan.Args[len(plan.Args)-len(prompt):]; strings.Join(got, "\x00") != strings.Join(prompt, "\x00") {
		t.Errorf("prompt changed: %q", plan.Args)
	}
}

func TestUnresolvedGitMetadataGetsNoGrant(t *testing.T) {
	for _, kind := range []string{"missing", "file", "symlink", "directory symlink inside"} {
		t.Run(kind, func(t *testing.T) {
			codexHome(t, "")
			repo := t.TempDir()
			gitDir := filepath.Join(repo, ".git")
			switch kind {
			case "file":
				if err := os.WriteFile(gitDir, []byte("gitdir: ../elsewhere"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(t.TempDir(), gitDir); err != nil {
					t.Fatal(err)
				}
			case "directory symlink inside":
				target := filepath.Join(repo, "metadata")
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, gitDir); err != nil {
					t.Fatal(err)
				}
			}
			plan := gitLaunch(t, role.Write, "-C", repo)
			if len(gitRoots(plan.Args)) > 0 {
				t.Fatalf("granted writes to %s: %q", kind, plan.Args)
			}
			if roots, err := gitMetadataDirectories(repo); err == nil || len(roots) != 0 {
				t.Fatalf("unresolved metadata accepted: %q %v", roots, err)
			}
		})
	}
}

func TestAnUnresolvedWorkingDirectoryGetsNoGitGrant(t *testing.T) {
	codexHome(t, "")
	for _, args := range [][]string{{"-C"}, {"--cd="}, {"-C", "/path/that/does/not/exist"}} {
		if _, err := New().Launch(harness.LaunchRequest{Dir: t.TempDir(), Args: args, Role: role.Write}); err == nil {
			t.Fatalf("server accepted an unresolved working directory: %q", args)
		}
	}
}
