package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Claude Code's own state gains onboarding done and the project's trust,
// under the root as given and resolved, and keeps every other key and
// number as it was; a file holding both already is not written again, and
// one rewake cannot read is refused and left as it is.
func TestClaudeStateGainsOnlyWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	resolved := t.TempDir()
	root := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(resolved, root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, claudeStateFile)
	text := `{"numStartups": 7, "firstStartTime": "x", "changelogLastFetched": 1792778823536, "projects": {"` + root + `": {"hasTrustDialogAccepted": false, "lastCost": 0.25}, "/other": {"hasTrustDialogAccepted": true}}}`
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareClaudeState(dir, root); err != nil {
		t.Fatal(err)
	}
	saved := readBack(t, path)
	projects := saved["projects"].(map[string]any)
	trusted := func(key string) bool {
		project, _ := projects[key].(map[string]any)
		return project["hasTrustDialogAccepted"] == true
	}
	if saved["hasCompletedOnboarding"] != true || !trusted(root) || !trusted(resolved) || !trusted("/other") {
		t.Fatalf("%v", saved)
	}
	if saved["changelogLastFetched"] != json.Number("1792778823536") || saved["numStartups"] != json.Number("7") || projects[root].(map[string]any)["lastCost"] != json.Number("0.25") {
		t.Fatalf("a value changed: %v", saved)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("%v %v", info, err)
	}
	before, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(before, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareClaudeState(dir, root); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before)+" " {
		t.Fatal("a file holding both was written again")
	}
	broken := `{"projects": [`
	if err := os.WriteFile(path, []byte(broken), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepareClaudeState(dir, root); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("a broken file: %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != broken {
		t.Fatal("a broken file was replaced")
	}
}

// A fresh folder gets a state file with the two keys alone.
func TestClaudeStateMadeFresh(t *testing.T) {
	dir, root := t.TempDir(), t.TempDir()
	if err := prepareClaudeState(dir, root); err != nil {
		t.Fatal(err)
	}
	saved := readBack(t, filepath.Join(dir, claudeStateFile))
	if len(saved) != 2 || saved["hasCompletedOnboarding"] != true || len(saved["projects"].(map[string]any)) != 1 {
		t.Fatalf("%v", saved)
	}
}
