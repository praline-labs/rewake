package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/grant"
)

// hookPayload is a hook's payload as Claude Code 2.1.280 sent it in the
// probe of September 27, 2026 (docs/research-claude-actions.md), with the
// paths moved into the test's directory.
func hookPayload(event, tool, mode, cwd string, input map[string]string, suggested ...string) []byte {
	payload := map[string]any{
		"cwd": cwd, "permission_mode": mode, "hook_event_name": event, "tool_name": tool,
		"tool_input": input, "effort": map[string]string{"level": "medium"},
	}
	if len(suggested) > 0 {
		payload["permission_suggestions"] = []map[string]any{{"type": "addDirectories", "directories": suggested, "destination": "session"}}
	}
	encoded, _ := json.Marshal(payload)
	return encoded
}

func TestTheGrantHookAnswersFromTheJournal(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	work, granted, old, other := filepath.Join(root, "work"), filepath.Join(root, "grant"), filepath.Join(root, "old"), filepath.Join(root, "other")
	for _, dir := range []string{work, granted, old, other, filepath.Join(granted, ".git")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	journal := []grant.Entry{
		{Path: granted, Message: "m1", Outcome: grant.Granted},
		{Path: old, Message: "m0", Outcome: grant.Revoking},
	}
	write := func(path string) map[string]string { return map[string]string{"file_path": path, "content": "probe\n"} }
	read := map[string]string{"file_path": filepath.Join(work, "README")}
	command := func(line string) map[string]string { return map[string]string{"command": line} }
	for _, c := range []struct {
		name     string
		payload  []byte
		entries  []grant.Entry
		says     []string
		added    []string
		removed  []string
		silent   bool
		notAdded []string
		// ownTools is a launch with the person's own allowed tools, where
		// rewake added no rule of its own.
		ownTools bool
	}{
		{
			name:    "a write in a live grant is allowed and its root added, and a settled one taken out with it",
			payload: hookPayload(permissionRequest, "Write", "acceptEdits", work, write(filepath.Join(granted, "a.txt")), granted),
			says:    []string{`"behavior":"allow"`, `"addDirectories"`, granted, `"removeDirectories"`, old, `"destination":"session"`},
			added:   []string{granted}, removed: []string{old},
		},
		{
			name:    "a command is never allowed, even one whose suggestions all lie in a grant",
			payload: hookPayload(permissionRequest, "Bash", "default", work, command("touch "+granted+"/c.txt && touch ~/.bashrc"), granted),
			entries: journal[:1], silent: true,
		},
		{
			name:    "nor one that needs anything else",
			payload: hookPayload(permissionRequest, "Bash", "default", work, command("touch "+other+"/c.txt"), other),
			entries: journal[:1], silent: true,
		},
		{
			name:    "a command is not asked about while nothing is taken back",
			payload: hookPayload(preToolUse, "Bash", "acceptEdits", work, command("touch "+granted+"/c.txt")),
			entries: journal[:1], silent: true,
		},
		{
			name:    "a command while a grant is taken back is asked about",
			payload: hookPayload(preToolUse, "Bash", "acceptEdits", work, command("touch "+old+"/c.txt")),
			says:    []string{`"permissionDecision":"ask"`, old},
		},
		{
			name:    "and its question goes to the person",
			payload: hookPayload(permissionRequest, "Bash", "acceptEdits", work, command("touch "+old+"/c.txt")),
			silent:  true,
		},
		{
			name:    "unless it is a plain rewake command, answered with the removal",
			payload: hookPayload(permissionRequest, "Bash", "acceptEdits", work, command("rewake inbox --owed")),
			says:    []string{`"behavior":"allow"`, `"removeDirectories"`, old}, removed: []string{old}, notAdded: []string{granted},
		},
		{
			name:     "but not where the person gave their own allowed tools, which may leave rewake to be asked",
			payload:  hookPayload(permissionRequest, "Bash", "acceptEdits", work, command("rewake inbox --owed")),
			ownTools: true, silent: true,
		},
		{
			name:    "a rewake command with anything more is no plain one",
			payload: hookPayload(permissionRequest, "Bash", "acceptEdits", work, command("rewake inbox; touch "+old+"/x")),
			silent:  true,
		},
		{
			name:    "a command in a mode that asks nothing is not asked",
			payload: hookPayload(preToolUse, "Bash", "bypassPermissions", work, command("touch "+old+"/c.txt")),
			silent:  true,
		},
		{
			name:    "the Git metadata inside a grant is not given",
			payload: hookPayload(permissionRequest, "Write", "default", work, write(filepath.Join(granted, ".git", "config")), granted),
			entries: journal[:1], silent: true,
		},
		{
			name:    "a file tool writing into the Git metadata of a live grant is sent to the person before it runs",
			payload: hookPayload(preToolUse, "Write", "acceptEdits", work, write(filepath.Join(granted, ".git", "config"))),
			entries: journal[:1], says: []string{`"permissionDecision":"ask"`, "left to the person"},
		},
		{
			name:    "and one writing elsewhere in it is not",
			payload: hookPayload(preToolUse, "Write", "acceptEdits", work, write(filepath.Join(granted, "a.txt"))),
			entries: journal[:1], silent: true,
		},
		{
			name:    "a file tool writing where a grant is taken back is denied, and nothing taken out yet",
			payload: hookPayload(preToolUse, "Write", "acceptEdits", work, write(filepath.Join(old, "a.txt"))),
			says:    []string{`"permissionDecision":"deny"`, "taken back"},
		},
		{
			name:    "a read in the working directory is turned into the question that takes a grant back",
			payload: hookPayload(preToolUse, "Read", "acceptEdits", work, read),
			says:    []string{`"permissionDecision":"ask"`, old},
		},
		{
			name:    "and its question is allowed with the removal",
			payload: hookPayload(permissionRequest, "Read", "acceptEdits", work, read),
			says:    []string{`"behavior":"allow"`, `"removeDirectories"`, old}, removed: []string{old}, notAdded: []string{granted},
		},
		{
			name:    "a mode that asks nothing is not asked",
			payload: hookPayload(preToolUse, "Read", "bypassPermissions", work, read),
			silent:  true,
		},
		{
			name:    "a directory another live task holds is not taken back",
			payload: hookPayload(preToolUse, "Read", "acceptEdits", work, read),
			entries: []grant.Entry{{Path: old, Message: "m0", Outcome: grant.Revoking}, {Path: old, Message: "m2", Outcome: grant.Granted}},
			silent:  true,
		},
		{
			name:    "a session with nothing granted hears nothing",
			payload: hookPayload(permissionRequest, "Write", "default", work, write(filepath.Join(granted, "a.txt")), granted),
			entries: []grant.Entry{}, silent: true,
		},
		{
			name:    "a payload that does not parse is silence",
			payload: []byte("{"), silent: true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			entries := c.entries
			if entries == nil {
				entries = journal
			}
			// Through GrantCall, as the hook hands it to the wrapper.
			call, _ := GrantCall(c.payload, !c.ownTools)
			decided := DecideGrant(call, entries)
			if c.silent {
				if len(decided.Output) != 0 {
					t.Fatalf("answered %s", decided.Output)
				}
				return
			}
			for _, want := range c.says {
				if !strings.Contains(string(decided.Output), want) {
					t.Fatalf("answered %s, want %q in it", decided.Output, want)
				}
			}
			for _, unwanted := range c.notAdded {
				if strings.Contains(string(decided.Output), `"addDirectories"`) || slices.Contains(decided.Added, unwanted) {
					t.Fatalf("added %v: %s", decided.Added, decided.Output)
				}
			}
			if !slices.Equal(decided.Added, c.added) || !slices.Equal(decided.Removed, c.removed) {
				t.Fatalf("added %v removed %v, want %v and %v", decided.Added, decided.Removed, c.added, c.removed)
			}
		})
	}
}

// The wrapper is handed the call without what the tool writes: a Write
// carries the whole file.
func TestAGrantCallLeavesOutTheContent(t *testing.T) {
	payload := hookPayload(permissionRequest, "Write", "acceptEdits", "/w", map[string]string{"file_path": "/g/a.txt", "content": "secret body"}, "/g")
	call, ok := GrantCall(payload, true)
	if !ok || strings.Contains(string(call), "secret body") || !strings.Contains(string(call), "/g/a.txt") || !strings.Contains(string(call), "acceptEdits") {
		t.Fatalf("call %s, %v", call, ok)
	}
	bash := hookPayload(preToolUse, "Bash", "acceptEdits", "/w", map[string]string{"command": "rewake inbox", "description": "read the mail"})
	if call, ok := GrantCall(bash, true); !ok || !strings.Contains(string(call), "rewake inbox") || strings.Contains(string(call), "read the mail") {
		t.Fatalf("a command's call %s, %v", call, ok)
	}
	// Whether rewake's rule is in force is the hook's to say, never the payload's.
	var claimed map[string]any
	_ = json.Unmarshal(bash, &claimed)
	claimed["rewake_rule"] = true
	forged, _ := json.Marshal(claimed)
	if call, _ := GrantCall(forged, false); strings.Contains(string(call), "rewake_rule") {
		t.Fatalf("a payload set the rule: %s", call)
	}
	if _, ok := GrantCall([]byte("not json"), true); ok {
		t.Fatal("a payload that does not parse made a call")
	}
}
