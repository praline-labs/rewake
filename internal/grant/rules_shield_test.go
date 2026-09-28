package grant

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A grant reaches everything below its root except the directories a
// checkout keeps its metadata or a harness its configuration in, however
// deep and in whatever case: a drive that ignores case would take .GIT for
// .git, and a nested checkout's .git is as much a checkout's as the top one.
func TestAGrantCoversNothingShieldedAtAnyDepthOrCase(t *testing.T) {
	root := "/work/proj"
	for path, covered := range map[string]bool{
		"/work/proj":                         true,
		"/work/proj/src/main.go":             true,
		"/work/proj/gitignore":               true,
		"/work/proj/src/.gitkeep":            true,
		"/work/proj/.git/config":             false,
		"/work/proj/.GIT/hooks/pre-commit":   false,
		"/work/proj/vendor/lib/.git/config":  false,
		"/work/proj/.Claude/settings.json":   false,
		"/work/proj/deep/er/.codex/cfg.toml": false,
		"/work/proj/pkg/.agents/skill.md":    false,
		"/work/other/src":                    false,
		"/work/project/src":                  false,
	} {
		if got := Covers(root, path); got != covered {
			t.Errorf("Covers(%s, %s) = %v, want %v", root, path, got, covered)
		}
	}
}

// rewake's own configuration names what a session runs: a worker writing
// there would configure every session that starts after it.
func TestRewakesConfigurationIsNeverGranted(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(root, "home")
	config := filepath.Join(home, ".config", "rewake")
	if err := os.MkdirAll(filepath.Join(config, "rooms"), 0o755); err != nil {
		t.Fatal(err)
	}
	rules := Env{Home: home}.Rules()
	for _, path := range []string{config, filepath.Join(config, "rooms")} {
		var refusal *Refusal
		if err := rules.Check(path, path, false); !errors.As(err, &refusal) || refusal.Code != 2 || !strings.Contains(refusal.Message, "rewake's configuration") {
			t.Errorf("%s: %v", path, err)
		}
	}
}

// The rules a session builds for itself know the machine's temporary
// directory without being told: a directory made there is refused.
func TestTheCurrentRulesKnowTheTemporaryDirectory(t *testing.T) {
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var refusal *Refusal
	if err := CurrentEnv("", nil).Rules().Check(path, path, false); !errors.As(err, &refusal) || !strings.Contains(refusal.Message, "temporary directory") {
		t.Fatalf("%s: %v", path, err)
	}
}
