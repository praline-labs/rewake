package workflow

import "testing"

// The controls of fixture-worktree, each the core's worktree path with one
// part undone; those of what rm keeps after the end are in
// fixture_worktree_keep_test.go.

// The launch that does not enter its checkout: the session and both halves
// work in the source, and nothing else notices.
var mutantWorktreeNotEntered = mutation{
	name:  "worktree-not-entered",
	file:  "internal/cli/launch_worktree.go",
	edits: []edit{{"if err := os.Chdir(record.Workdir()); err != nil {", "if err := os.Chdir(\".\"); err != nil {"}},
}

// rm that takes no running session for one: it removes the checkout from
// under the session, and after the end there is nothing left to visit or to
// remove.
var mutantWorktreeRunningIgnored = mutation{
	name:  "worktree-running-ignored",
	file:  "internal/cli/worktree_keep.go",
	edits: []edit{{"\t\t\t\tif session.Alive() && (made ||", "\t\t\t\tif false && session.Alive() && (made ||"}},
}

// rm that deletes the directory without asking git: the repository keeps an
// entry for a checkout that is gone, the moved one's included.
var mutantWorktreeGitKept = mutation{
	name:  "worktree-git-kept",
	file:  "internal/worktree/remove.go",
	edits: []edit{{"\t\tif err := gitDir(record.CommonDir, append(args, record.Path)...); err != nil {", "\t\tif err := os.RemoveAll(record.Path); err != nil {"}},
}

func TestAWorktreeLaunchThatStaysInTheSourceFails(t *testing.T) {
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-worktree", playFixtureWorktree, mutantWorktreeNotEntered, obsTreeEntered)
}

func TestARemovalThatIgnoresTheSessionFails(t *testing.T) {
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-worktree", playFixtureWorktree, mutantWorktreeRunningIgnored, obsTreeKept, obsTreeLanded, obsTreeFinishKept, obsTreeVisited, obsTreeRemoved)
}

func TestARemovalBehindGitsBackFails(t *testing.T) {
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-worktree", playFixtureWorktree, mutantWorktreeGitKept, obsTreeMoved, obsTreeRemoved)
}

// The checkout made detached, as before worktrees had a branch: the claim's
// branch stays at the commit, so there is nothing to land, and nothing finish
// can take.
var mutantWorktreeDetached = mutation{
	name:  "worktree-detached",
	file:  "internal/worktree/serial.go",
	edits: []edit{{`git(source.Source, "worktree", "add", record.Path, record.Branch)`, `git(source.Source, "worktree", "add", "--detach", record.Path, record.Commit)`}},
}

// land that merges instead of fast-forwarding: main gets a merge commit, not
// the worker's, and the branch can no longer be finished.
var mutantWorktreeLandMerges = mutation{
	name:  "worktree-land-merges",
	file:  "internal/worktree/land.go",
	edits: []edit{{`"merge", "--ff-only", "--quiet", tip`, `"-c", "user.name=Mutant", "-c", "user.email=mutant@example.invalid", "merge", "--no-ff", "--quiet", "-m", "Land", tip`}},
}

// finish that asks nothing first: it lands and removes the checkout from
// under the running session.
var mutantWorktreeFinishUnchecked = mutation{
	name:  "worktree-finish-unchecked",
	file:  "internal/cli/worktree_land.go",
	edits: []edit{{"\t\tif len(reasons) > 0 {\n\t\t\treturn &FailedError{Message: fmt.Sprintf(\"%s is not finished", "\t\tif false {\n\t\t\treturn &FailedError{Message: fmt.Sprintf(\"%s is not finished"}},
}

// finish that leaves the branch behind.
var mutantWorktreeFinishBranchKept = mutation{
	name:  "worktree-finish-branch-kept",
	file:  "internal/cli/worktree_land.go",
	edits: []edit{{"\tresult.fateOfBranch(record)\n\treturn printValue(ctx, result, func() []string {\n\t\tlines := append(landing.lines()", "\treturn printValue(ctx, result, func() []string {\n\t\tlines := append(landing.lines()"}},
}

func TestAWorktreeWithoutABranchFails(t *testing.T) {
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-worktree", playFixtureWorktree, mutantWorktreeDetached, obsTreeEntered, obsTreeLanded, obsTreeFinishKept, obsTreeRemoved)
}

func TestALandThatMergesFails(t *testing.T) {
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-worktree", playFixtureWorktree, mutantWorktreeLandMerges, obsTreeLanded, obsTreeRemoved)
}

func TestAFinishThatAsksNothingFails(t *testing.T) {
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-worktree", playFixtureWorktree, mutantWorktreeFinishUnchecked, obsTreeFinishKept, obsTreeVisited, obsTreeRemoved)
}

func TestAFinishThatKeepsTheBranchFails(t *testing.T) {
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-worktree", playFixtureWorktree, mutantWorktreeFinishBranchKept, obsTreeRemoved)
}
