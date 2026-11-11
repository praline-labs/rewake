package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/workmode"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func workerAliases(t *testing.T, text string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	// The catalog now answers from a live main when the room has one, so a
	// test must name its own state: the default would be the person's.
	t.Setenv(state.DirEnv, filepath.Join(home, "state"))
	t.Setenv(state.SessionEnv, "")
	t.Setenv(state.EpochEnv, "")
	t.Setenv(state.RoomEnv, "")
	t.Chdir(t.TempDir())
	path := filepath.Join(home, ".config", "rewake", "aliases.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const workerAliasesExample = `[alias.agent-general]
harness = "agent"

[alias.agent-write]
harness = "agent"
role = "write"
args = []
command = "agent-worker"
`

// An alias is no launch of its own: its name on the line is refused with how
// main opens it.
func TestAnAliasIsNoLaunch(t *testing.T) {
	workerAliases(t, workerAliasesExample)
	code, _, errOut := run("agent-write")
	if code != ExitUsage || !strings.Contains(errOut, "rewake space open <space> --alias agent-write") {
		t.Fatalf("a direct alias launch: %d %s", code, errOut)
	}
}

func TestWorkersRefusesAnOldFile(t *testing.T) {
	workerAliases(t, "[alias.lead]\nharness = \"agent\"\nmain = true\nrewake = [\"--main\"]\n")
	code, _, errOut := run("workers")
	if code != ExitUsage || !strings.Contains(errOut, "alias.lead.main (line 3)") || !strings.Contains(errOut, "alias.lead.rewake (line 4)") {
		t.Fatalf("an old file: %d %s", code, errOut)
	}
}

type workerSettingsHarness struct{ agentHarness }

func (workerSettingsHarness) LaunchSettings(model, effort, _ string) ([]string, error) {
	return []string{"--model", model, "--effort", effort}, nil
}

func (workerSettingsHarness) AppliesContext() bool { return false }

func (workerSettingsHarness) SingleUseFlags() []adapter.Flag {
	return append(agentHarness{}.SingleUseFlags(), adapter.Flag{Spellings: []string{"--effort"}, TakesValue: true})
}

func (workerSettingsHarness) Offers() adapter.Offer {
	return adapter.Offer{Conversation: adapter.ConversationOffer{Flag: "--session-id"}}
}

func TestWorkersListsTheCatalogue(t *testing.T) {
	workerAliases(t, workerAliasesExample+`
description = "Small patch worker"
[alias.agent-write.launch]
model = "chosen"
effort = "high"
context = "large"
`)
	base := filepath.Join(t.TempDir(), "never-created")
	t.Setenv(state.DirEnv, base)
	t.Setenv(state.SessionEnv, "")
	catalog := adapter.NewCatalog(workerSettingsHarness{})
	code, out, errOut := runOn(catalog, "workers", "--json")
	var view workersListing
	if code != ExitOK || json.Unmarshal([]byte(out), &view) != nil || len(view.Workers) != 2 {
		t.Fatalf("workers: %d %s %s", code, out, errOut)
	}
	general, writer := view.Workers[0], view.Workers[1]
	if general.Name != "agent-general" || general.Role != "general" || general.Command != "agent" || general.Model.Value != "" {
		t.Fatalf("an alias with defaults: %+v", general)
	}
	if writer.Role != "write" || writer.Command != "agent-worker" || writer.Description != "Small patch worker" ||
		!writer.Model.Applied || !writer.Effort.Applied || writer.Context.Applied || writer.Context.Value != "large" {
		t.Fatalf("a structured worker: %+v", writer)
	}
	// The mode catalog travels whole enough to be chosen from: name, source,
	// path and description, never the text a session reads.
	if len(view.Modes) != 4 || view.Modes[0].Name != workmode.Review || view.Modes[0].Text != "" || view.Modes[0].Description == "" {
		t.Fatalf("the catalog in --json: %+v", view.Modes)
	}
	code, out, errOut = runOn(catalog, "workers")
	if code != ExitOK || !strings.Contains(out, `model="chosen" (applied)`) || !strings.Contains(out, `context="large" (shown only)`) {
		t.Fatalf("workers lines: %d %s %s", code, out, errOut)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("workers touched state: %v", err)
	}
}

// No writer is inventory, not an authority decision: the listing says the
// templates are missing and, apart from that, what main's policy is.
func TestWorkersSaysWhenNoTemplateWrites(t *testing.T) {
	workerAliases(t, "[alias.agent-general]\nharness = \"agent\"\n")
	code, out, errOut := run("workers")
	if code != ExitOK || strings.Count(out, "No launchable write-role templates.") != 1 ||
		!strings.Contains(out, "Implementation must be delegated: when implementation is requested") ||
		strings.Contains(out, "main remains the orchestrator") {
		t.Fatalf("no writer: %d %s %s", code, out, errOut)
	}
	workerAliases(t, "")
	code, out, errOut = run("workers")
	if code != ExitOK || strings.Count(out, "No launchable write-role templates.") != 0 ||
		!strings.Contains(out, "No launchable worker templates. To delegate, define") || !strings.Contains(out, "Implementation must be delegated") {
		t.Fatalf("an empty catalog: %d %s %s", code, out, errOut)
	}
	workerAliases(t, workerAliasesExample)
	code, out, errOut = run("workers")
	if code != ExitOK || strings.Contains(out, "No launchable write-role templates") || strings.Contains(out, "Implementation must be delegated") {
		t.Fatalf("a writer: %d %s %s", code, out, errOut)
	}
}

// The starting mode's own model and effort are shown, with their source, and
// do not change the template's launch settings' JSON meaning.
func TestWorkersShowsTheStartingModesSelection(t *testing.T) {
	workerAliases(t, workerAliasesExample+`
[alias.agent-write.launch]
model = "template/m"
[alias.agent-write.pi]
start_mode = "build"
[alias.agent-write.pi.modes.build]
model = "mode/m"
effort = "high"
`)
	catalog := adapter.NewCatalog(workerSettingsHarness{})
	code, out, errOut := runOn(catalog, "workers", "--json")
	var view workersListing
	if code != ExitOK || json.Unmarshal([]byte(out), &view) != nil {
		t.Fatalf("workers: %d %s %s", code, out, errOut)
	}
	writer := view.Workers[1]
	if writer.StartMode != "build" || writer.StartModeSettings == nil || writer.StartModeSettings.Model != "mode/m" || writer.StartModeSettings.Effort != "high" {
		t.Fatalf("the starting mode's settings: %+v", writer)
	}
	if writer.Model.Value != "template/m" || !writer.Model.Applied {
		t.Fatalf("the template's launch setting changed meaning: %+v", writer.Model)
	}
	code, out, errOut = runOn(catalog, "workers")
	if code != ExitOK || !strings.Contains(out, `start_model="mode/m" (starting mode; overrides template model)`) ||
		!strings.Contains(out, `start_effort="high" (starting mode; overrides template effort)`) ||
		!strings.Contains(out, `model="template/m"`) {
		t.Fatalf("workers lines: %d %s %s", code, out, errOut)
	}
}

func TestWorkersUnreadableFileRefuses(t *testing.T) {
	path := workerAliases(t, "[alias.bad\n")
	code, _, errOut := run("workers")
	if code != ExitUsage || !strings.Contains(errOut, path+":1:") {
		t.Fatalf("broken catalog: %d %s", code, errOut)
	}
}

// A worker comes only from a template, which names its harness and role:
// space open takes neither as a flag.
func TestSpaceOpenTakesNoHarnessOrRole(t *testing.T) {
	ctx := &Context{table: newTable(adapter.NewCatalog(workerSettingsHarness{}))}
	for _, flag := range []string{"--harness", "--role"} {
		if _, err := ctx.table.parse([]string{"space", "open", "api", "--alias", "agent-write", flag, "agent"}); err == nil {
			t.Fatalf("%s parsed", flag)
		}
	}
}

// space open hands launch the request as typed: a bare --worktree asks for a
// generated name, the words after -- are the harness's.
func TestSpaceOpenAsksWhatWasTyped(t *testing.T) {
	ctx := &Context{table: newTable(adapter.NewCatalog(workerSettingsHarness{}))}
	for _, c := range []struct {
		argv []string
		want string
	}{
		{[]string{"--alias", "w"}, `{"Alias":"w","Worktree":null,"NoWorktree":false,"Mode":null,"Args":[]}`},
		{[]string{"--alias", "w", "--worktree", "--work-mode", "build", "--", "--model", "m"}, `{"Alias":"w","Worktree":"true","NoWorktree":false,"Mode":"build","Args":["--model","m"]}`},
		{[]string{"--alias", "w", "--worktree=fix", "--no-worktree"}, `{"Alias":"w","Worktree":"fix","NoWorktree":true,"Mode":null,"Args":[]}`},
		{[]string{"--worktree=", "--work-mode="}, `{"Alias":"","Worktree":"","NoWorktree":false,"Mode":"","Args":[]}`},
	} {
		parsed, err := ctx.table.parse(append([]string{"space", "open", "api"}, c.argv...))
		if err != nil {
			t.Fatalf("%v: %v", c.argv, err)
		}
		got, _ := json.Marshal(spaceRequest(parsed.Call))
		if string(got) != c.want {
			t.Errorf("%v: %s, want %s", c.argv, got, c.want)
		}
	}
}
