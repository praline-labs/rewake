# Worktree removal, finish and land made whole after the reconnaissance of September 28, 2026

The read-only reconnaissance of the code that landed September 26–28 found the worktree
commands acting on a look taken outside the lock, promising a finish that lands nothing
when git would refuse the removal, and giving advice that did not fit the state. Main set
this package the same day: those findings, a suspected race in land, and a bound on the
repository hooks land runs.

## What was found and done

- **rm looked outside the lock and removed after it.** A checkout a launch had claimed
  and not yet checked out looked missing and forgotten, nothing to keep; the removal,
  once the launch let go of the lock, took the checkout just made, and the launch went on
  in a directory that was gone. rm's checks and its removal now hold the repository's
  lock together, with the record read again under it (`worktree.RemoveChecked`), and
  finish holds it from its checks through its landing to the removal (`worktree.Finish`);
  a failed launch taking its checkout back does the same.
- **The window between the checkout and the session.** Until the session is
  registered, nothing said the checkout was in use. The record names the rewake process
  that made it, and while that process runs and no session is claimed, rm and finish
  keep the checkout and `ls` shows it as launching.
- **`.worktreeinclude` copies were recorded only at the claim.** A launch killed before
  the claim left them looking like ignored work. They are written into the record as
  soon as they are made, under the same lock.
- **finish could land and then fail to remove.** `git worktree remove` refuses a
  checkout locked with `git worktree lock` and one holding a submodule checked out
  (checked on git 2.43), and finish asked neither before landing. Both are refusals of
  finish and rm now, before anything moves; `rm --force` passes `--force` twice, which
  a locked checkout needs.
- **Plain `git worktree remove` deletes ignored files too**, not only a forced one, as
  worktree.md said; the document now says so. rm refused them already.
- **Advice on the branch after rm or finish.** A branch another checkout has out, one
  deleted already, one gone with its repository each got the advice to merge or delete
  commits only it held. Each case now has its own line.
- **A root inside the main checkout passed from a linked one.** The check compared the
  root with the linked checkout only; the main checkout is asked too.
- **land exit codes.** A source that has the worktree's own branch checked out is a state
  and exits 1; `--into` naming that branch stays a wrong call, exit 2. `--into=` with no
  branch landed into the default target; it is a wrong call now.
- **land and a source switching branches, confirmed.** `git merge` moves whatever branch
  its checkout has out, so a switch between land's look and its merge fast-forwarded the
  other branch, and land reported success. The source is asked again right before the
  merge, and after it the target must hold the tip; otherwise land is refused, naming the
  branch git moved.
- **Hooks without a time limit.** land waits for a git call running the repository's
  hooks at most a minute, then is refused naming the hook still running. git is not
  stopped — a merge cut between the files and the branch would leave the checkout changed
  under a branch that did not move — and writes its output to files, so it can outlive
  rewake without meeting a closed pipe. The other git calls of land, finish and rm run no
  hooks.
- **Tests** for each, each checked against the fix taken out.

## What stays open

- land, finish and the checks of ls and rm are still not serialized with a
  `git worktree add` rewake does not run ([work-queue.md](../work-queue.md)).
- A hook land stopped waiting for may still land what it was merging; land run again
  says what did.
- The git calls without hooks — `git status` with a clean or smudge filter, say — have no
  time limit.
