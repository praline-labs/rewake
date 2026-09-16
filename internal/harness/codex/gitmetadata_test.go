package codex

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/role"
)

func writeGitPointer(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestGitPointersGrantTheActualMetadataDirectories(t *testing.T) {
	for _, kind := range []string{"submodule", "worktree", "absolute worktree", "same common directory"} {
		t.Run(kind, func(t *testing.T) {
			codexHome(t, "")
			base := t.TempDir()
			cwd := filepath.Join(base, "checkout")
			common := filepath.Join(base, "main.git")
			metadata := filepath.Join(common, "worktrees", "branch")
			if err := os.MkdirAll(metadata, 0700); err != nil {
				t.Fatal(err)
			}
			pointer := "../main.git/worktrees/branch"
			if kind == "absolute worktree" {
				pointer = metadata
			}
			writeGitPointer(t, filepath.Join(cwd, ".git"), "gitdir: "+pointer+"\r\n")
			expected := []string{metadata}
			switch kind {
			case "worktree":
				writeGitPointer(t, filepath.Join(metadata, "commondir"), "../..\n")
				expected = append(expected, common)
			case "absolute worktree":
				writeGitPointer(t, filepath.Join(metadata, "commondir"), common+"\n")
				expected = append(expected, common)
			case "same common directory":
				writeGitPointer(t, filepath.Join(metadata, "commondir"), ".\n")
			}
			plan := gitLaunch(t, role.Write, "-C", cwd)
			got := gitRoots(plan.Args)
			if strings.Join(got, "\x00") != strings.Join(expected, "\x00") {
				t.Fatalf("roots=%q want=%q notes=%q", got, expected, plan.Notes)
			}
		})
	}
}

func TestMalformedGitPointersNeverGrantPartialAccess(t *testing.T) {
	for _, kind := range []string{"bad prefix", "empty gitdir", "missing gitdir", "multiline", "oversize", "common missing", "common malformed", "common symlink", "metadata symlink", "parent symlink", "symlink then parent"} {
		t.Run(kind, func(t *testing.T) {
			codexHome(t, "")
			base := t.TempDir()
			cwd := filepath.Join(base, "checkout")
			metadata := filepath.Join(base, "metadata")
			if err := os.Mkdir(metadata, 0700); err != nil {
				t.Fatal(err)
			}
			pointer := "gitdir: ../metadata\n"
			switch kind {
			case "bad prefix":
				pointer = "gitdir ../metadata\n"
			case "empty gitdir":
				pointer = "gitdir: \n"
			case "missing gitdir":
				pointer = "gitdir: ../missing\n"
			case "multiline":
				pointer = "gitdir: ../metadata\n../other\n"
			case "oversize":
				pointer = "gitdir: " + strings.Repeat("x", 64*1024)
			case "common missing":
				writeGitPointer(t, filepath.Join(metadata, "commondir"), "../missing\n")
			case "common malformed":
				writeGitPointer(t, filepath.Join(metadata, "commondir"), "\n")
			case "common symlink":
				other := filepath.Join(base, "elsewhere")
				writeGitPointer(t, other, ".\n")
				if err := os.Symlink(other, filepath.Join(metadata, "commondir")); err != nil {
					t.Fatal(err)
				}
			case "metadata symlink":
				if err := os.Symlink(metadata, filepath.Join(base, "linked")); err != nil {
					t.Fatal(err)
				}
				pointer = "gitdir: ../linked\n"
			case "symlink then parent":
				outside := t.TempDir()
				for _, name := range []string{"child", "metadata"} {
					if err := os.Mkdir(filepath.Join(outside, name), 0700); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.Symlink(filepath.Join(outside, "child"), filepath.Join(base, "linked")); err != nil {
					t.Fatal(err)
				}
				pointer = "gitdir: ../linked/../metadata\n"
			case "parent symlink":
				if err := os.Mkdir(filepath.Join(metadata, "child"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(metadata, filepath.Join(base, "linked")); err != nil {
					t.Fatal(err)
				}
				pointer = "gitdir: ../linked/child\n"
			}
			writeGitPointer(t, filepath.Join(cwd, ".git"), pointer)
			plan := gitLaunch(t, role.Write, "-C", cwd)
			if got := gitRoots(plan.Args); len(got) != 0 {
				t.Fatalf("invalid pointer granted %q", got)
			}
			if !strings.Contains(strings.Join(plan.Notes, " "), "not granting Git metadata writes") {
				t.Errorf("missing reason: %q", plan.Notes)
			}
		})
	}
}

// Compare with Git's own interpretation of real layouts, without requiring Git
// for production discovery or for environments that cannot run this check.
func TestMetadataResolutionMatchesGitLayouts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	codexHome(t, "")
	base := t.TempDir()
	git := func(directory string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "-c", "protocol.file.allow=always"}, args...)...)
		cmd.Dir = directory
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %q: %v: %s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	main := filepath.Join(base, "main")
	source := filepath.Join(base, "source")
	for _, dir := range []string{main, source} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		git(dir, "init", "-q")
		git(dir, "commit", "--allow-empty", "-m", "Initialize metadata fixture")
	}
	linked := filepath.Join(base, "linked")
	git(main, "worktree", "add", "-b", "linked", linked)
	git(main, "submodule", "add", source, "child")
	for _, cwd := range []string{main, linked, filepath.Join(main, "child")} {
		paths := strings.Split(git(cwd, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir"), "\n")
		expected := paths[:1]
		if paths[1] != paths[0] {
			expected = append(expected, paths[1])
		}
		plan := gitLaunch(t, role.Write, "-C", cwd)
		if got := gitRoots(plan.Args); strings.Join(got, "\x00") != strings.Join(expected, "\x00") {
			t.Errorf("cwd=%s roots=%q want=%q notes=%q", cwd, got, expected, plan.Notes)
		}
	}
}
