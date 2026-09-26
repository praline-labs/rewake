# Review of the Codex launch worktree — September 26, 2026

Two reviews of [the worktree for a Codex launch](2026-09-26-codex-worktree-launch.md)
at `c27fce7` — one on the Claude Code side, one on the Codex side, with probes in a
container running the real 0.155.1 terminal and server — did not accept it. The rule main
set for the fixes: `rewake worktree rm` without `--force` never loses work, and where it
cannot tell, it refuses with a hint that names the next step.

## What was found and done

Blocking:

1. **Reachability was asked only after the HEAD moved.** A checkout still at the commit
   it was made at was taken as safe, and after the branch holding that commit was
   deleted, rm without `--force` left it to garbage collection. Now asked every time,
   against branches, tags and remote-tracking refs (`internal/worktree/git.go`).
2. **Only the session a checkout was made for held it.** After it ended, a session
   started in the checkout by hand — the next step the taken-name refusal suggests —
   lost its directory to a plain rm. Now every live rewake session whose working
   directory is in the checkout holds it, in every room of the current state directory
   and of the owner's; `ls` names them. A process rewake did not start is not seen, and
   that is written down.
3. **A missing directory was pruned.** `git worktree prune` takes every missing checkout
   of the repository, and a directory moved by hand is missing too. Now the HEAD of a
   missing checkout is read from `git worktree list --porcelain` and checked for
   reachability; rm without `--force` refuses and points at `git worktree repair`, and
   with `--force` removes its own entry with `git worktree remove`, which touches no
   other. A checkout the repository no longer lists leaves only the record.
4. **`--worktree` with `resume` or `fork`.** The conversation kept its saved cwd outside
   the checkout while its workspace roots were the checkout's (verified on 0.155.1).
   Refused now with exit 2, before anything is made, naming the two ways on. Chosen over
   moving the conversation, which would hand the terminal the checkout as its `-C`: what
   that does to a continued conversation's permissions was not checked, and fork was
   only read in the source. The refusal is the smaller claim until then.
5. **A harness that exited with an error kept its checkout.** A `resume` of a
   conversation that does not exist exits 1 with the checkout made. The launch now takes
   an untouched checkout back on a nonzero exit as on a failure — at its commit, and
   with nothing rm without `--force` would keep.

Medium and low:

6. **Ignored files** — a `.env`, a local build — are deleted by `git worktree remove`
   without a word; `git status --ignored=matching` finds them now, and rm refuses.
7. **A worktree directory inside the repository** is refused, symbolic links resolved.
8. **`GIT_OPTIONAL_LOCKS=0`** on every git call: `git status` no longer takes
   `index.lock` from under a commit the session is making.
9. **"Refusals come before anything is made"** was not true. The early checks — the
   flag, `resume`/`fork`, `--remote` and profiles — now come before the directory is
   resolved; [launch.md](../launch.md#a-worktree-for-a-launch) says which refusals follow
   the creation of the worktree directory or of the checkout, and that the checkout of a
   refused registration is taken back. The line saying where the session works is
   printed only once its name is claimed.
10. **No prompt, no terminal**: `GIT_TERMINAL_PROMPT=0` and a session of its own for
    every git call.
11. **Codex's bucket id** was "four hex" in one document and "four-character id" in
    another: it is the first four characters of a random UUID in its simple form, four
    lowercase hex digits (`worktree/src/paths.rs`).
12. **A record naming another directory** — whose path is not the directory of its own
    name, or not clean — is not listed, so rm cannot be led to remove it.

The harness interface gained `WorktreeRefusal(args)` ([code.md](../code.md)).

## Evidence

- Unit tests, each red on a mutation that undoes its fix (run by hand for this review):
  `TestACommitNoBranchHoldsAnyMoreIsUnreachable` (1),
  `TestASessionStartedInACheckoutHoldsIt` (2), `TestAMissingCheckoutIsRemovedAlone` and
  `TestWorktreeRmKeepsAMissingCheckoutUnlessForced` (3, the second also red when the
  refusal is lifted), the `resume`/`fork` rows of `TestWorktreeLaunchRefusals` (4),
  `TestAnErrorExitTakesAnUntouchedCheckoutBack` (5), `TestIgnoredFilesAreWork` and
  `TestWorktreeRmKeepsIgnoredFiles` (6), `TestARootInsideTheRepositoryIsRefused` (7),
  `TestGitRunsDetachedAndWithoutOptionalLocks` (8, 10),
  `TestATakenSessionNameLeavesNoCheckout` (9) and
  `TestListSkipsARecordNamingAnotherDirectory` (12).
- The workflow case `codex-worktree` grew four observations — a visitor session holds
  the ended session's checkout, an ignored file, a commit whose branch was deleted, a
  moved directory — each on a checkout of its own, and four mutants:
  `worktree-visitor-ignored`, `worktree-ignored-unseen`, `worktree-branch-trusted` and
  `worktree-moved-unrefused`. `worktree-running-ignored` was redefined on the new
  lookup, and `worktree-git-kept` breaks the moved checkout's observation as well
  ([testing-cases.md](../testing-cases.md#a-worktree-for-a-codex-launch)).
- Two waits in the scenario were too early, both seen under the full suite's load: the
  first read a directory file the fixture had created and not yet written, and took the
  empty text for the terminal's directory; the visitor's was satisfied by the
  registration, before the harness had started, so a mutant's rm took the directory and
  the visitor failed to start. Both now wait for the directory files of both halves.
- After the branch was moved onto main, `TestACaseSweepLeavesItsNeighbourAlone` went red
  there. Not the branch: the test raced the kernel on main too, 2 of 30 runs, since its
  escaped descendant, a bare `sleep`, could die between the sweep's signal and its look.
  The descendant now takes a fifth of a second to end
  ([testing-pool.md](../testing-pool.md#owner-labels)); 0 of 180 runs, and 0 of 60
  under `-race` with every core busy.

## What stays open

- Acceptance on the Codex side of these fixes.
- A continued conversation in a new checkout, should the owner want one: hand the
  terminal the checkout as `-C` and check the permissions of the continuation first.
- A process rewake did not start in a checkout is not seen by rm.
