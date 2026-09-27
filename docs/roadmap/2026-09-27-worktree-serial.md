# Worktrees of one repository made one at a time

September 27, 2026. Found while chasing a control of `codex-worktree` that failed once
under load, after the review of the nested-launch refusal
([entry](2026-09-27-no-nested-launch.md)); fixed the same day.

## What was found

- **Three launches at once, one exits 1.** `codex-worktree` starts three
  `rewake codex --worktree` in one repository together. One of 48 runs of the worktree
  cases under load failed a launch, and the suite said only that the session exited 1.
  Rerun with the sessions' standard error captured by hand, the launch said
  `git worktree add failed: Preparing worktree (checking out 'moved') / fatal: failed to
  read .git/worktrees/unreached/commondir: Success`: the add of one launch had read the
  entry another was writing, with its `commondir` still empty. With git 2.43 that is
  fatal to the add, and to `git worktree remove`, `list` and `branch -d` as well
  ([research-worktree.md](../research-worktree.md#git-worktree-add-beside-another)).
- **A red case said nothing of why.** The sessions' standard error went nowhere, so a
  launch that refused, or a fixture that complained, showed only as an exit code.

## What was done

- **A lock per repository**, by main's decision of September 27, 2026: an `flock` on
  `<repository>-<hash>.lock` beside the repository's directory under the worktree root,
  never in the repository's Git directory. A launch holds it from before its branch is
  made until its checkout, copies and record are in place; `rewake worktree rm` and
  finish's removal hold it too. The wait ends after a minute with exit 1, naming the lock
  and its last holder ([worktree.md](../worktree.md#launches-at-once)).
- **One retry** of an add that meets a half-written entry, matched by the path in git's
  message, after a fifth of a second. The lock keeps rewake's adds apart, but not the
  person's own `git worktree add` or Claude Code's `-w`; git has already removed what it
  began of the failed checkout, so the retry starts clean. Any other failure is not
  retried, and a second one of this kind is reported.
- **Tests.** Three `Create` at once in one repository, each add standing in for one in
  the middle of writing its entry, run one at a time — red without the lock, three at
  once. An empty `commondir` left by hand fails the add; filled before the retry, the
  checkout is made; left empty, the failure is reported after exactly one retry with no
  branch or record behind; another failure is not retried — the first two red without
  the retry. A lock held elsewhere refuses a checkout and a removal in bounded time,
  naming the holder, and a checkout succeeds once it is let go. The lock file stays out
  of the Git directory and the repository's directory still goes with its last checkout.
- **Standard error kept in the suite.** Every session a case starts writes its standard
  error to `<name>.stderr` in the case's HOME, the file itself rather than a pipe the
  test drains. A red case keeps the files cut to 64 KiB, first and last halves, and names
  the last line of each, at most four, in its failure, its record and the summary's
  excerpt ([testing.md](../testing.md#reading-a-result)); a green case deletes them with
  its directory.

## After review

- A holder can keep the lock past the minute without hanging: the copies
  `.worktreeinclude` asks for stay under it, so a parallel rm cannot take a checkout
  still being filled (main's decision), and large ignored directories take time. The
  refusal and [worktree.md](../worktree.md#launches-at-once) now say so and say to wait
  and run again; the minute stays.
- A removal no longer makes the worktree root again for its lock: when the root is gone
  it goes without the lock, and only Create makes the root. Before, rm of a checkout
  whose root the person had deleted made an empty root and a lock file again.

## What stays open

- land, finish's landing and the checks before ls and rm are not serialized with an
  add rewake does not run, and fail with git's message while one is writing its entry;
  they are run again.
