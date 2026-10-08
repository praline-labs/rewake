package workflow

import "testing"

// The controls of codex-grant-resume, each the product with one part of the
// restore undone.

// The adapter never looks for the grants a resumed conversation had.
var mutantCodexResumeNotRestored = mutation{
	name:  "codex-resume-not-restored",
	file:  "internal/harness/codex/server_dirgrant.go",
	edits: []edit{{"	restoring := s.resumedHints(thread, entries)\n", "	restoring, _ := []grant.Hint(nil), s.resumedHints\n"}},
}

// A new run forgets what the run before it owed, whatever conversation it
// continues: the task is closed by the resume, and main confirms nothing.
var mutantResumeWaitsDropped = mutation{
	name:  "resume-waits-dropped",
	file:  "internal/inbox/adopt.go",
	edits: []edit{{"	if thread == \"\" {\n", "	if true {\n"}},
}

// Main's wrapper takes every task for open, and confirms a grant again after
// its report.
var mutantCodexResumeClosedHeld = mutation{
	name:  "codex-resume-closed-held",
	file:  "internal/wrap/grants.go",
	edits: []edit{{"return inbox.TaskOpen(dir, held.To, held.ID)", "return true, held.ID != \"\""}},
}

func TestACodexResumeNotRestoringFails(t *testing.T) {
	runCodexResumeControl(t, mutantCodexResumeNotRestored, obsCodexResumeGiven, obsCodexResumeTakenBack)
}

func TestACodexResumeForgettingTheWaitsFails(t *testing.T) {
	runCodexResumeControl(t, mutantResumeWaitsDropped, obsCodexResumeGiven, obsCodexResumeTakenBack)
}

func TestACodexResumeAfterTheReportRestoringFails(t *testing.T) {
	runCodexResumeControl(t, mutantCodexResumeClosedHeld, obsCodexResumeClosed)
}

func runCodexResumeControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, codexColumn.harness, "codex-grant-resume", playCodexGrantResume, mutant, breaks...)
}
