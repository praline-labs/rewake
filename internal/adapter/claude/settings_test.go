package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// settingsFile writes text as the folder's settings.json, or none when text
// is empty, and returns the folder.
func settingsFile(t *testing.T, text string) string {
	t.Helper()
	dir := t.TempDir()
	if text != "" {
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func readBack(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := parseSettings(raw)
	if err != nil {
		t.Fatalf("%s: %v", raw, err)
	}
	return settings
}

// fileDenied is how many tools the file denies at this version: all but
// EnterWorktree, which left it in version 3 for each run's own settings.
var fileDenied = len(deniedTools) - 1

func denied(settings map[string]any) []any {
	return settings["permissions"].(map[string]any)["deny"].([]any)
}

// No file and {} alike get every default at this build's version; the
// owner's folder holds {} from the build before.
func TestSettingsFreshAndEmpty(t *testing.T) {
	for _, text := range []string{"", "{}\n"} {
		path, _, wrote, err := workerSettings(settingsFile(t, text), "/home/p")
		if err != nil || !wrote {
			t.Fatalf("%q: %v %v", text, wrote, err)
		}
		settings := readBack(t, path)
		if version, _ := settingsVersion(settings); version != defaultsVersion {
			t.Fatalf("%q: version %d", text, version)
		}
		if len(denied(settings)) != fileDenied || settings["idleCompaction"] != false || !slices.Equal(settings["claudeMdExcludes"].([]any), []any{"/home/p/.claude/CLAUDE.md"}) {
			t.Fatalf("%q: %v", text, settings)
		}
	}
}

// A version-0 file keeps the person's values and lists and gains the rest;
// a file at this version is passed as it is, the tool taken out staying out.
func TestSettingsKeepThePersons(t *testing.T) {
	dir := settingsFile(t, `{"idleCompaction": true, "permissions": {"deny": ["WebFetch"], "allow": ["Bash(go:*)"]}, "env": {"X": "a&b"}, "n": 1.50}`)
	path, _, _, err := workerSettings(dir, "/home/p")
	if err != nil {
		t.Fatal(err)
	}
	settings := readBack(t, path)
	if settings["idleCompaction"] != true || settings["n"] != json.Number("1.50") || settings["env"].(map[string]any)["X"] != "a&b" || settings["env"].(map[string]any)["DISABLE_AUTOUPDATER"] != "1" {
		t.Fatalf("%v", settings)
	}
	if deny := denied(settings); deny[0] != "WebFetch" || len(deny) != fileDenied+1 {
		t.Fatalf("deny %v", deny)
	}
	raw, _ := os.ReadFile(path)
	edited := strings.Replace(string(raw), `"Agent",`, "", 1)
	if edited == string(raw) {
		t.Fatalf("no Agent in %s", raw)
	}
	if err := os.WriteFile(path, []byte(edited), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, wrote, err := workerSettings(dir, "/home/p"); err != nil || wrote {
		t.Fatalf("a current file was written again: %v %v", wrote, err)
	}
	if again, _ := os.ReadFile(path); string(again) != edited {
		t.Fatalf("the file changed:\n%s", again)
	}
}

// Version 3 takes EnterWorktree out of the file, which Claude Code now reads
// as the user's settings, and leaves the rest of deny as the person had it.
func TestEnterWorktreeLeavesTheFile(t *testing.T) {
	dir := settingsFile(t, `{"rewake": {"defaults": 2}, "permissions": {"deny": ["Agent", "EnterWorktree", "ExitWorktree"]}}`)
	path, _, wrote, err := workerSettings(dir, "/home/p")
	if err != nil || !wrote {
		t.Fatalf("%v %v", wrote, err)
	}
	if deny := denied(readBack(t, path)); !slices.Equal(deny, []any{"Agent", "ExitWorktree"}) {
		t.Fatalf("deny %v", deny)
	}
}

// An upgrade runs only the steps after the file's version: what the person
// took out of an older one stays out, a new default is added, a retired one
// leaves only where it still holds rewake's value.
func TestSettingsMigrate(t *testing.T) {
	defaults := []setting{
		{path: []string{"permissions", "deny"}, value: "Agent", item: true, since: 1},
		{path: []string{"permissions", "deny"}, value: "Old", item: true, since: 1, retired: 3},
		{path: []string{"spinnerTipsEnabled"}, value: false, since: 1, retired: 2},
		{path: []string{"awaySummaryEnabled"}, value: false, since: 1, retired: 2},
		{path: []string{"permissions", "deny"}, value: "New", item: true, since: 2},
		{path: []string{"newKey"}, value: true, since: 3},
		{path: []string{"taken"}, value: true, since: 3},
	}
	settings, err := parseSettings([]byte(`{"rewake": {"defaults": 1}, "permissions": {"deny": ["Old", "Mine"]}, "spinnerTipsEnabled": true, "awaySummaryEnabled": false, "taken": false}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := migrate(settings, 1, 3, defaults); err != nil {
		t.Fatal(err)
	}
	if deny := denied(settings); !slices.Equal(deny, []any{"Mine", "New"}) {
		t.Fatalf("deny %v", deny)
	}
	_, away := settings["awaySummaryEnabled"]
	if settings["spinnerTipsEnabled"] != true || away || settings["newKey"] != true || settings["taken"] != false || settings[rewakeKey].(map[string]any)["defaults"] != 3 {
		t.Fatalf("%v", settings)
	}
	if err := migrate(map[string]any{"permissions": "x"}, 0, 1, defaults); err == nil {
		t.Fatal("a permissions that is not an object was accepted")
	}
}

// A file rewake cannot read refuses the launch naming the line and stays
// as it is; so does a newer build's, which is passed untouched.
func TestSettingsRefusedOrLeft(t *testing.T) {
	for text, want := range map[string]string{
		"{\n  \"a\": 1,\n}\n":                     "line 3",
		"[1]":                                     "line 1: not a JSON object",
		"{} {}":                                   "more after the object",
		`{"rewake": {"keep": ["Agent"]}}`:         "rewake.keep is gone",
		`{"rewake": {"defaults": -1}}`:            "not a version",
		`{"permissions": {"deny": "Agent"}}`:      "permissions.deny is a string, not a list",
		`{"autoMemoryEnabled": "not-a-boolean"}`:  "autoMemoryEnabled is a string, not a boolean",
		`{"permissions": {"deny": [{"a": 1}]}}`:   "permissions.deny holds an object, where each is a string",
		`{"env": {"DISABLE_AUTOUPDATER": ["1"]}}`: "env.DISABLE_AUTOUPDATER is a list, not a string",
		`{"env": "x"}`:                            "env is not an object",
		`{"rewake": {"defaults": 1}, "permissions": {"deny": "Agent"}}`: "permissions.deny is a string",
		"{\n\"rewake\": {\"defaults\": 1.5}}":                           "not a version",
	} {
		dir := settingsFile(t, text)
		_, _, _, err := workerSettings(dir, "/home/p")
		if err == nil || !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), filepath.Join(dir, "settings.json")) {
			t.Errorf("%q: %v", text, err)
		}
		if raw, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(raw) != text {
			t.Errorf("%q was replaced by %q", text, raw)
		}
	}
	newer := `{"rewake": {"defaults": 99}}`
	dir := settingsFile(t, newer)
	if _, _, wrote, err := workerSettings(dir, "/home/p"); err != nil || wrote {
		t.Fatalf("a newer file: %v %v", wrote, err)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "settings.json")); string(raw) != newer {
		t.Fatalf("a newer file became %q", raw)
	}
}

// A settings.json linked into the person's own files stays a link: the file
// it names is written.
func TestSettingsThroughALink(t *testing.T) {
	own := filepath.Join(t.TempDir(), "worker.json")
	if err := os.WriteFile(own, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(own, filepath.Join(dir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := workerSettings(dir, "/home/p"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(dir, "settings.json")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the link was replaced: %v", err)
	}
	if len(denied(readBack(t, own))) != fileDenied {
		t.Fatal("the linked file did not get the defaults")
	}
}

// A settings.json linking to a file not there yet is refused, the link left
// as it is, rather than replaced by a file of its own.
func TestSettingsThroughADanglingLink(t *testing.T) {
	own := filepath.Join(t.TempDir(), "worker.json")
	dir := t.TempDir()
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(own, link); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := workerSettings(dir, "/home/p"); err == nil || !strings.Contains(err.Error(), own) {
		t.Fatalf("a dangling link was accepted: %v", err)
	}
	if target, err := os.Readlink(link); err != nil || target != own {
		t.Fatalf("the link became %q: %v", target, err)
	}
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatalf("the link's target was made: %v", err)
	}
}

// Keys no default owns are the person's, whatever they hold.
func TestSettingsLeaveOtherKeys(t *testing.T) {
	path, _, _, err := workerSettings(settingsFile(t, `{"env": {"MINE": 1}, "mine": [1, {}], "permissions": {"allow": "x"}}`), "/home/p")
	if err != nil {
		t.Fatal(err)
	}
	settings := readBack(t, path)
	if settings["env"].(map[string]any)["MINE"] != json.Number("1") || len(settings["mine"].([]any)) != 2 || settings["permissions"].(map[string]any)["allow"] != "x" {
		t.Fatalf("%v", settings)
	}
}
