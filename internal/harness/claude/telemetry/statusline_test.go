package telemetry

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func line(command string) string {
	encoded, _ := json.Marshal(command)
	return `{"statusLine":{"type":"command","command":` + string(encoded) + `}}`
}

// The layers merge field by field, lowest first: user, project, local, then
// the caller's --settings. A layer naming only another field changes nothing
// about the command.
func TestTheEffectiveStatusLineFollowsTheHarnessOrder(t *testing.T) {
	dir := t.TempDir()
	home, project := filepath.Join(dir, "home"), filepath.Join(dir, "project")
	env := []string{"HOME=" + home}
	resolve := func(sources string, caller string) string {
		return ResolveStatusCommand(env, project, sources, json.RawMessage(caller))
	}
	if got := resolve(AllSources, ""); got != "" {
		t.Errorf("nothing configured: %q", got)
	}
	writeSettings(t, filepath.Join(home, ".claude", "settings.json"), line("user"))
	if got := resolve(AllSources, ""); got != "user" {
		t.Errorf("user only: %q", got)
	}
	writeSettings(t, filepath.Join(project, ".claude", "settings.json"), line("project"))
	if got := resolve(AllSources, ""); got != "project" {
		t.Errorf("project over user: %q", got)
	}
	writeSettings(t, filepath.Join(project, ".claude", "settings.local.json"), `{"statusLine":{"padding":2}}`)
	if got := resolve(AllSources, ""); got != "project" {
		t.Errorf("a local layer without a command: %q", got)
	}
	writeSettings(t, filepath.Join(project, ".claude", "settings.local.json"), line("local"))
	if got := resolve(AllSources, ""); got != "local" {
		t.Errorf("local over project: %q", got)
	}
	if got := resolve(AllSources, `{"type":"command","command":"caller"}`); got != "caller" {
		t.Errorf("caller over local: %q", got)
	}
	if got := resolve("user", ""); got != "user" {
		t.Errorf("user source only: %q", got)
	}
	if got := resolve(NoSources, ""); got != "" {
		t.Errorf("no sources: %q", got)
	}
	if got := resolve(NoSources, `{"type":"other","command":"x"}`); got != "" {
		t.Errorf("a status line of another kind: %q", got)
	}
	writeSettings(t, filepath.Join(project, ".claude", "settings.json"), `{not json`)
	if got := resolve("project", ""); got != "" {
		t.Errorf("an unreadable file contributed: %q", got)
	}
}

// The user layer is the configuration directory when one is given, and the
// cowork file name under its variable.
func TestTheUserLayerFollowsTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config")
	writeSettings(t, filepath.Join(config, "settings.json"), line("config"))
	writeSettings(t, filepath.Join(config, "cowork_settings.json"), line("cowork"))
	env := []string{"HOME=" + filepath.Join(dir, "home"), "CLAUDE_CONFIG_DIR=" + config}
	if got := ResolveStatusCommand(env, dir, "user", nil); got != "config" {
		t.Errorf("config dir: %q", got)
	}
	if got := ResolveStatusCommand(append(env, "CLAUDE_CODE_USE_COWORK_PLUGINS=1"), dir, "user", nil); got != "cowork" {
		t.Errorf("cowork: %q", got)
	}
	if got := ProjectDir([]string{"CLAUDE_PROJECT_DIR=/p"}); got != "/p" {
		t.Errorf("project dir = %q", got)
	}
}
