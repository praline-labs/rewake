package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

func TestLegacyLandlockIsSeenInArgumentsAndConfiguration(t *testing.T) {
	plain := t.TempDir()
	for name, args := range map[string][]string{
		"a setting":          {"-c", "features.use_legacy_landlock=true"},
		"a joined setting":   {"--config=features.use_legacy_landlock = true"},
		"an enabled feature": {"--enable", "use_legacy_landlock"},
		"a flag":             {"--use-legacy-landlock"},
	} {
		if why := legacyLandlock(args, plain); !strings.Contains(why, legacyLandlockKey) {
			t.Errorf("%s: %q", name, why)
		}
	}
	if why := legacyLandlock([]string{"-c", "model_reasoning_effort=low"}, plain); why != "" {
		t.Errorf("an unrelated setting: %q", why)
	}
	configured := t.TempDir()
	if err := os.WriteFile(filepath.Join(configured, "config.toml"), []byte("[features]\nuse_legacy_landlock = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if why := legacyLandlock(nil, configured); !strings.Contains(why, "config.toml") {
		t.Errorf("the configuration: %q", why)
	}
}

// A session whose commands may share rewake's namespaces takes no grant at
// all, of directories or of Git: a worker there could confirm one itself.
func TestAGrantIsRefusedUnderLegacyLandlock(t *testing.T) {
	lib := t.TempDir()
	for name, message := range map[string]inbox.Message{
		"directories": {ID: "d", Kind: inbox.Task, GrantDirs: []string{lib}},
		"git":         {ID: "g", Kind: inbox.Task, GrantGit: true},
	} {
		t.Run(name, func(t *testing.T) {
			server, captured := gitDeliveryFixture(t, role.Write, gitReadFixture{thread: gitThreadFixture(lib, nil, "idle")})
			dirGrantSession(t, server)
			server.legacyLandlock = "its launch arguments mention " + legacyLandlockKey
			roots, result := deliverDirs(t, server, captured, message)
			if result.State != inbox.Failed || !strings.Contains(result.Detail, "legacy Landlock") || roots != nil {
				t.Fatalf("roots=%q result=%+v", roots, result)
			}
		})
	}
}
