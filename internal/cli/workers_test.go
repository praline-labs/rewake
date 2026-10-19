package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/state"
)

func workerAliases(t *testing.T, text string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
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
	code, out, errOut = runOn(catalog, "workers")
	if code != ExitOK || !strings.Contains(out, `model="chosen" (applied)`) || !strings.Contains(out, `context="large" (shown only)`) {
		t.Fatalf("workers lines: %d %s %s", code, out, errOut)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("workers touched state: %v", err)
	}
}

func TestWorkersUnreadableFileRefuses(t *testing.T) {
	path := workerAliases(t, "[alias.bad\n")
	code, _, errOut := run("workers")
	if code != ExitUsage || !strings.Contains(errOut, path+":1:") {
		t.Fatalf("broken catalog: %d %s", code, errOut)
	}
}

func TestSpaceAliasUsesTheTemplate(t *testing.T) {
	workerAliases(t, workerAliasesExample+`
[alias.agent-write.launch]
model = "chosen"
effort = "high"
context = "large"
`)
	ctx := &Context{table: newTable(adapter.NewCatalog(workerSettingsHarness{}))}
	for _, argv := range [][]string{
		{"space", "open", "api", "--alias", "nothing"},
		{"space", "open", "api", "--alias", "agent-write", "--role", "main"},
		{"space", "open", "api", "--alias", "agent-write", "--harness", "agent"},
		{"space", "open", "api", "--alias", "agent-write", "--worktree="},
	} {
		parsed, err := ctx.table.parse(argv)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ctx.spaceTarget(parsed.Call); err == nil {
			t.Fatalf("accepted %v", argv)
		}
	}
	parsed, err := ctx.table.parse([]string{"space", "open", "api", "--alias", "agent-write", "--role", "general", "--", "--effort", "low"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := ctx.spaceTarget(parsed.Call)
	if err != nil {
		t.Fatal(err)
	}
	words, err := workerLaunch(target, parsed.Call, "api", "chosen-room", "fresh-id")
	want := []string{"--name", "api", "--room", "chosen-room", "--general", "--command", "agent-worker", "agent", "--model", "chosen", "--effort", "low", "--session-id", "fresh-id"}
	if err != nil || !reflect.DeepEqual(words, want) {
		t.Fatalf("launch: %v, %v; want %v", words, err, want)
	}

	// A literal prompt tail cannot swallow the space's conversation id or
	// checkout flag; an explicit checkout replaces the alias's older one.
	target.args = []string{"--worktree=old", "--", "--session-id=literal"}
	parsed.Call.Flags["worktree"] = "patch"
	words, err = workerLaunch(target, parsed.Call, "api", "chosen-room", "fresh-id")
	want = []string{"--name", "api", "--room", "chosen-room", "--general", "--command", "agent-worker", "agent", "--worktree=patch", "--session-id", "fresh-id", "--", "--session-id=literal"}
	if err != nil || !reflect.DeepEqual(words, want) {
		t.Fatalf("literal tail: %v, %v; want %v", words, err, want)
	}
}
