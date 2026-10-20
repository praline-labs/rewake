package worktree

import (
	"errors"
	"strings"
)

// Release removes a checkout whose session is over, and its branch, when
// nothing would be lost: nothing rm would keep it for (KeepReasons), and no
// commit of it that a local branch other than its own does not hold. A local
// branch, not the branch it was made from: main may land into another with
// --into, or the checkout it came from may have switched since, and any local
// branch keeps the commits; a tag or a remote-tracking ref may be moved or
// pruned with no work of this checkout in mind. It looks and removes under one
// hold of the repository's lock, as rm does, and says why the checkout stays,
// or "" once it is gone.
func Release(record Record) string {
	why := ""
	err := RemoveChecked(record, false, func(current Record) error {
		reasons, check, err := KeepReasonsAndCheck(current)
		if err == nil && len(reasons) == 0 {
			reasons, err = unmerged(current, check)
		}
		switch {
		case err != nil:
			why = "rewake could not tell whether removing it would lose work: " + err.Error()
		case len(reasons) > 0:
			why = strings.Join(reasons, "; ")
		default:
			return nil
		}
		return errKept
	})
	switch {
	case why != "":
		return why
	case err != nil:
		return "it could not be removed: " + err.Error()
	}
	_, _ = DropBranch(record)
	return ""
}

// errKept refuses a release inside the lock; the reason is kept apart.
var errKept = errors.New("kept")

// unmerged says which of a checkout's commits no other local branch holds:
// its HEAD, and its branch's tip when the checkout has left the branch.
func unmerged(record Record, check Check) ([]string, error) {
	if repositoryGone(record) {
		return nil, nil
	}
	own := branchRef(record.Branch)
	var reasons []string
	if check.Head != "" {
		held, err := heldByAnotherBranch(record.CommonDir, check.Head, own)
		if err != nil {
			return nil, err
		}
		if !held {
			reasons = append(reasons, "its commits up to "+short(check.Head)+" are on no local branch but its own")
		}
	}
	if record.Branch == "" || check.Branch == record.Branch {
		return reasons, nil
	}
	tip, err := refTip(record.CommonDir, own)
	if err != nil || tip == "" || tip == check.Head {
		return reasons, err
	}
	held, err := heldByAnotherBranch(record.CommonDir, tip, own)
	if err != nil {
		return nil, err
	}
	if !held {
		reasons = append(reasons, "its branch "+record.Branch+" holds commits no other local branch has")
	}
	return reasons, nil
}

// heldByAnotherBranch says whether a local branch other than own holds
// commit.
func heldByAnotherBranch(commonDir, commit, own string) (bool, error) {
	holders, err := gitDirOutput(commonDir, "for-each-ref", "--contains", commit, "--format=%(refname)", "refs/heads")
	if err != nil {
		return false, err
	}
	for _, holder := range strings.Split(holders, "\n") {
		if holder != "" && holder != own {
			return true, nil
		}
	}
	return false, nil
}
