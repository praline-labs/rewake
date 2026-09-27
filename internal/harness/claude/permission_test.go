package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/grant"
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
	for _, c := range []struct {
		name     string
		payload  []byte
		entries  []grant.Entry
		says     []string
		added    []string
		removed  []string
		silent   bool
		notAdded []string
	}{
		{
			name:    "a write in a live grant is allowed and its root added, and a settled one taken out with it",
			payload: hookPayload(permissionRequest, "Write", "acceptEdits", work, write(filepath.Join(granted, "a.txt")), granted),
			says:    []string{`"behavior":"allow"`, `"addDirectories"`, granted, `"removeDirectories"`, old, `"destination":"session"`},
			added:   []string{granted}, removed: []string{old},
		},
		{
			name:    "a command whose suggestions all lie in a grant is allowed",
			payload: hookPayload(permissionRequest, "Bash", "default", work, map[string]string{"command": "touch " + granted + "/c.txt"}, granted),
			entries: journal[:1], says: []string{`"behavior":"allow"`, granted}, added: []string{granted},
		},
		{
			name:    "a command that needs anything else is left to the person",
			payload: hookPayload(permissionRequest, "Bash", "default", work, map[string]string{"command": "touch " + other + "/c.txt"}, other),
			entries: journal[:1], silent: true,
		},
		{
			name:    "the Git metadata inside a grant is not given",
			payload: hookPayload(permissionRequest, "Write", "default", work, write(filepath.Join(granted, ".git", "config")), granted),
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
			decided := DecideGrant(c.payload, entries)
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
	payload := hookPayload(permissionRequest, "Write", "default", "/w", map[string]string{"file_path": "/g/a.txt", "content": "secret body"}, "/g")
	call, ok := claudeHarness{}.GrantCall(payload)
	if !ok || strings.Contains(string(call), "secret body") || !strings.Contains(string(call), "/g/a.txt") || !strings.Contains(string(call), "addDirectories") {
		t.Fatalf("call %s, %v", call, ok)
	}
	if _, ok := (claudeHarness{}).GrantCall([]byte("not json")); ok {
		t.Fatal("a payload that does not parse made a call")
	}
}
