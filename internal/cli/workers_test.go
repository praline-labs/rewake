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

const workerAliasesExample = `[alias.agent-main]
harness = "agent"
rewake = ["--main", "--name", "main"]
args = []

[alias.agent-write]
harness = "agent"
rewake = ["--write", "--name", "write"]
args = []
command = "agent-worker"
`

func TestWorkersOldFileAndReadOnlyModel(t *testing.T) {
	workerAliases(t, workerAliasesExample)
	base := filepath.Join(t.TempDir(), "never-created")
	t.Setenv(state.DirEnv, base)
	t.Setenv(state.SessionEnv, "")
	code, out, errOut := run("workers", "--json")
	var view workersListing
	if code != ExitOK || json.Unmarshal([]byte(out), &view) != nil || len(view.Workers) != 1 {
		t.Fatalf("workers: %d %s %s", code, out, errOut)
	}
	worker := view.Workers[0]
	if worker.Name != "agent-write" || worker.Role != "write" || worker.Command != "agent-worker" || worker.Main {
		t.Fatalf("old worker changed: %+v", worker)
	}
	code, out, errOut = run("workers", "--all", "--json")
	if code != ExitOK || json.Unmarshal([]byte(out), &view) != nil || len(view.Workers) != 2 || !view.Workers[0].Main {
		t.Fatalf("all workers: %d %s %s", code, out, errOut)
	}
	if _, err := os.Stat(base); !os.IsNotExist(err) {
		t.Fatalf("workers touched state: %v", err)
	}
}

type workerSettingsHarness struct{ agentHarness }

func (workerSettingsHarness) LaunchSettings(model, effort string) []string {
	return []string{"--model", model, "--effort", effort}
}

func (workerSettingsHarness) SingleUseFlags() []adapter.Flag {
	return append(agentHarness{}.SingleUseFlags(), adapter.Flag{Spellings: []string{"--effort"}, TakesValue: true})
}

func (workerSettingsHarness) Offers() adapter.Offer {
	return adapter.Offer{Conversation: adapter.ConversationOffer{Flag: "--session-id"}}
}

func TestWorkersStructuredModel(t *testing.T) {
	workerAliases(t, workerAliasesExample+`
description = "Small patch worker"
[alias.agent-write.launch]
model = "chosen"
effort = "high"
context = "large"
[alias.marked]
harness = "agent"
main = true
[alias.wrapper]
harness = "agent"
[alias.wrapper.launch]
model = "wrapped"
effort = "low"
context = "small"
apply = false
`)
	catalog := adapter.NewCatalog(workerSettingsHarness{})
	code, out, errOut := runOn(catalog, "workers", "--json")
	var view workersListing
	if code != ExitOK || json.Unmarshal([]byte(out), &view) != nil || len(view.Workers) != 2 {
		t.Fatalf("workers: %d %s %s", code, out, errOut)
	}
	worker, wrapper := view.Workers[0], view.Workers[1]
	if worker.Description != "Small patch worker" || !worker.Model.Applied || !worker.Effort.Applied || worker.Context.Applied || worker.Context.Value != "large" {
		t.Fatalf("structured worker: %+v", worker)
	}
	if wrapper.Model.Value != "wrapped" || wrapper.Model.Applied || wrapper.Effort.Applied || wrapper.Context.Applied {
		t.Fatalf("wrapper settings: %+v", wrapper)
	}
	code, out, errOut = runOn(catalog, "workers", "--all")
	if code != ExitOK || !strings.Contains(out, "[main]") || !strings.Contains(out, "(applied)") || !strings.Contains(out, "(shown only)") {
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

func TestSpaceAliasUsesLaunchAndRejectsMain(t *testing.T) {
	workerAliases(t, workerAliasesExample+`
[alias.agent-write.launch]
model = "chosen"
effort = "high"
context = "large"
[alias.marked]
harness = "agent"
main = true
`)
	ctx := &Context{table: newTable(adapter.NewCatalog(workerSettingsHarness{}))}
	for _, argv := range [][]string{
		{"space", "open", "api", "--alias", "agent-main"},
		{"space", "open", "api", "--alias", "marked", "--role", "write"},
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
