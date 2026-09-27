package workflow

import "testing"

// The controls of claude-grant-resume, each the product with one part of the
// restore undone.

// The grants confirmed again are not given to the resumed harness at launch.
var mutantClaudeResumeNotGiven = mutation{
	name:  "claude-resume-not-given",
	file:  "internal/wrap/wrap.go",
	edits: []edit{{"		GrantDirs:         restored.dirs(),\n", "		GrantDirs:         nil,\n"}},
}

// The keeper takes a restored grant for one the session never wrote in, so
// the report ends it without taking the directory out.
var mutantClaudeResumeNotHeld = mutation{
	name:  "claude-resume-not-held",
	file:  "internal/wrap/grant_resume.go",
	edits: []edit{{"!keepResumed(keeper, restored.thread, restored.grants, true)", "!keepResumed(keeper, restored.thread, restored.grants, false)"}},
}

// Main's wrapper takes every task for open, and confirms a grant again after
// its report.
var mutantClaudeResumeClosedHeld = mutation{
	name:  "claude-resume-closed-held",
	file:  "internal/wrap/grants.go",
	edits: []edit{{"return inbox.TaskOpen(dir, held.To, held.ID)", "return true, held.ID != \"\""}},
}

func TestAClaudeResumeWithoutTheDirectoryFails(t *testing.T) {
	runClaudeResumeControl(t, mutantClaudeResumeNotGiven, obsClaudeResumeGiven)
}

func TestAClaudeResumeKeptAsNeverWrittenFails(t *testing.T) {
	runClaudeResumeControl(t, mutantClaudeResumeNotHeld, obsClaudeResumeTakenBack)
}

func TestAClaudeResumeAfterTheReportRestoringFails(t *testing.T) {
	runClaudeResumeControl(t, mutantClaudeResumeClosedHeld, obsClaudeResumeClosed)
}

func runClaudeResumeControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, claudeColumn.harness, "claude-grant-resume", playClaudeGrantResume, mutant, breaks...)
}
