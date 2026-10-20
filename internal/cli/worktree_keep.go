package cli

import "github.com/praline-labs/rewake/internal/host/worktree"

// fateOfBranch deletes a removed checkout's branch when nothing goes with it,
// and says why it stays, or why there was none to delete: each case has its
// own next step, and advice to merge a branch that is gone, or held by another
// checkout, would send the reader the wrong way.
func (r *worktreeRemoval) fateOfBranch(record worktree.Record) {
	if record.Branch == "" {
		return
	}
	dropping, err := worktree.DropBranch(record)
	switch {
	case err != nil:
		r.BranchKept = err.Error()
	case dropping.Dropped:
		r.BranchDropped = true
	case dropping.Gone != "":
		r.BranchGone = dropping.Gone
	case dropping.CheckedOut != "":
		r.BranchKept = "it is checked out in " + dropping.CheckedOut + "; git branch -D " + record.Branch + " drops it once that checkout has left it"
	default:
		r.BranchKept = "it holds commits no other branch, tag or remote-tracking ref has; git -C " + record.Source + " merge --ff-only " + record.Branch + " takes them, git -C " + record.Source + " branch -D " + record.Branch + " drops them"
	}
}
