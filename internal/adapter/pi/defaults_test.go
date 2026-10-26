package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
)

// pi's keys are read from the settings files as Claude Code's are: a key the
// files drop is a default the person wrote and silently did not get.
func TestPiTakesItsDefaultsFromTheSettingsFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(t.TempDir())
	for _, key := range []string{"REWAKE_PI_MODEL", "REWAKE_PI_EFFORT"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	dir := filepath.Join(home, ".config", "rewake")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings"), []byte("REWAKE_PI_MODEL=from-the-file\nREWAKE_PI_EFFORT=low\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args, _ := adapter.ApplyDefaults(nil, defaults())
	if line := strings.Join(args, " "); !strings.Contains(line, "--model from-the-file") || !strings.Contains(line, "--thinking low") {
		t.Errorf("the settings file's pi defaults were not applied: %v", args)
	}
}
