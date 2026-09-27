package workflow

import "testing"

// The controls of claude-grant-dir, each the product with one part of the
// grant undone.

// The hook never allows a write in a grant. Nothing is added, so the
// grant ends at the report with nothing to take out, and the observation of
// taking it back breaks too.
var mutantClaudeGrantSilent = mutation{
	name:  "claude-grant-silent",
	file:  "internal/harness/claude/permission.go",
	edits: []edit{{"	roots := in.grantedRoots(live)\n", "	roots := []string(nil)\n"}},
}

// A grant reaches the Git metadata inside it.
var mutantClaudeGrantUnshielded = mutation{
	name:  "claude-grant-unshielded",
	file:  "internal/grant/rules.go",
	edits: []edit{{`var shielded = []string{".git", ".claude", ".codex", ".agents"}`, `var shielded = []string{".claude", ".codex", ".agents"}`}},
}

// The keeper never learns that a task was reported on.
var mutantClaudeGrantKept = mutation{
	name:  "claude-grant-kept",
	file:  "internal/grantauth/keeper.go",
	edits: []edit{{"			settled = k.Settled(entry.Message)\n", "			settled = false && k.Settled(entry.Message)\n"}},
}

func TestAClaudeGrantHookThatNeverAllowsFails(t *testing.T) {
	runClaudeGrantControl(t, mutantClaudeGrantSilent, obsClaudeGrantGiven, obsClaudeGrantTakenBack)
}

func TestAClaudeGrantReachingGitMetadataFails(t *testing.T) {
	runClaudeGrantControl(t, mutantClaudeGrantUnshielded, obsClaudeGrantShielded)
}

func TestAClaudeGrantKeptAfterItsReportFails(t *testing.T) {
	runClaudeGrantControl(t, mutantClaudeGrantKept, obsClaudeGrantTakenBack)
}

func runClaudeGrantControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, claudeColumn.harness, "claude-grant-dir", playClaudeGrantDir, mutant, breaks...)
}
