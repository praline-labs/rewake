package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

func TestLaunchPreservesCallerGitConfiguration(t *testing.T) {
	codexHome(t, "")
	repo := gitRepository(t)
	for _, value := range []string{
		rootsKey + `=["/caller"]`,
		`sandbox_workspace_write={writable_roots=["/caller"]}`,
		` sandbox_workspace_write.writable_roots = ["/caller"]`,
		`default_permissions="custom"`,
		`permissions={custom={filesystem={":root"="read"}}}`,
		`permissions.custom.filesystem={":root"="read"}`,
		`"sandbox_workspace_wr\u0069te"={writable_roots=[]}`,
	} {
		for _, flags := range [][]string{{"-c", value}, {"--config", value}, {"--config=" + value}, {"-c" + value}, {"-c=" + value}} {
			t.Run(strings.Join(flags, " "), func(t *testing.T) {
				args := append([]string{"-C", repo}, flags...)
				plan := gitLaunch(t, role.Write, args...)
				// The caller keeps every flag; adding metadata must never append
				// a replacement roots array.
				if strings.Join(plan.Args[:len(args)], "\x00") != strings.Join(args, "\x00") {
					t.Fatalf("caller args changed: %q", plan.Args)
				}
				assertNoImplicitGitGrant(t, plan, repo)
				if _, ok := configValue(plan.Args[len(args):], rootsKey); ok {
					t.Fatalf("roots override appended: %q", plan.Args)
				}
			})
		}
	}
}

func TestGitWritesSkipRemoteExecution(t *testing.T) {
	codexHome(t, "")
	for _, args := range [][]string{{"--remote", "server"}, {"--remote=server"}} {
		if _, err := New().Launch(harness.LaunchRequest{Dir: t.TempDir(), Args: args, Role: role.Write}); err == nil {
			t.Fatalf("unsupported server configuration accepted: %q", args)
		}
	}
}

func TestGitWritesDoNotReplaceAnyConfigSpelling(t *testing.T) {
	for _, config := range []string{
		"[sandbox_workspace_write]\nwritable_roots = []\n",
		"sandbox_workspace_write.writable_roots=[]\n",
		"sandbox_workspace_write={writable_roots=[]}\n",
		"[sandbox_workspace_write]\nexclude_slash_tmp=true\n",
		"[\"sandbox_workspace_write\"]\n\"writable_roots\"=[]\n",
		"[sandbox_workspace_wr\\u0069te]\nwritable_roots=[]\n",
		"default_permissions=\"custom\"\n",
		"[permissions.custom]\nextends=\":workspace\"\n",
		"# writable_roots are configured elsewhere\n",
		"[\"sandbox_worksp\\u0061ce_write\"]\n\"writable_ro\\u006fts\"=[]\n",
	} {
		t.Run(config, func(t *testing.T) {
			codexHome(t, config)
			repo := gitRepository(t)
			plan := gitLaunch(t, role.Write, "-C", repo)
			assertNoImplicitGitGrant(t, plan, repo)
		})
	}
}

func TestGitWritesDoNotNeedToReadConfig(t *testing.T) {
	home := codexHome(t, "")
	// A directory at the file path fails even when the tests run as root.
	if err := os.Mkdir(filepath.Join(home, "config.toml"), 0o700); err != nil {
		t.Fatal(err)
	}
	repo := gitRepository(t)
	plan := gitLaunch(t, role.Write, "-C", repo)
	assertNoImplicitGitGrant(t, plan, repo)
}

func TestGitWritesKeepProjectConfiguration(t *testing.T) {
	for _, location := range []string{"cwd", "project", "ancestor"} {
		t.Run(location, func(t *testing.T) {
			codexHome(t, "")
			base := t.TempDir()
			repo := filepath.Join(base, "repo")
			if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
				t.Fatal(err)
			}
			populateGitMetadata(t, filepath.Join(repo, ".git"), true)
			path := filepath.Join(repo, "config.toml")
			if location == "project" {
				path = filepath.Join(repo, ".codex", "config.toml")
			}
			if location == "ancestor" {
				path = filepath.Join(base, ".codex", "config.toml")
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("[sandbox_workspace_write]\nwritable_roots=[]\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			plan := gitLaunch(t, role.Write, "-C", repo)
			assertNoImplicitGitGrant(t, plan, repo)
			raw, err := os.ReadFile(path)
			if err != nil || string(raw) != "[sandbox_workspace_write]\nwritable_roots=[]\n" {
				t.Fatalf("project config changed: %q %v", raw, err)
			}
		})
	}
}

func TestGitWritesExtendSelectedProfiles(t *testing.T) {
	codexHome(t, "")
	for _, args := range [][]string{{"-p", "work"}, {"-pwork"}, {"-p=work"}, {"--profile", "work"}, {"--profile=work"}} {
		if _, err := New().Launch(harness.LaunchRequest{Dir: t.TempDir(), Args: args, Role: role.Write}); err == nil {
			t.Fatalf("unsupported server configuration accepted: %q", args)
		}
	}
}

func TestLaunchPreservesExplicitDirectoryFlags(t *testing.T) {
	codexHome(t, "")
	repo := gitRepository(t)
	for _, existing := range []string{filepath.Join(repo, ".git"), "/another/allowed/path"} {
		for _, flags := range [][]string{{"--add-dir", existing}, {"--add-dir=" + existing}} {
			plan := gitLaunch(t, role.Write, append([]string{"-C", repo}, flags...)...)
			got := gitRoots(plan.Args)
			if len(got) != 1 || got[0] != existing {
				t.Errorf("caller root lost: %q", plan.Args)
			}
		}
	}
}
