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

## Re-acceptance of 858f57d

The Codex side accepted findings 1–3 and 5–12. Finding 4 was open still: the refusal
asked the launch's continuation parser, which disagreed with the terminal. A joined image
value (`--image=foo`, `-ifoo`, `-i=foo`) was read as the start of a variadic list and
took the `resume` after it; the first positional was taken for the subcommand, though a
prompt may come before it; and `--remote-auth-token-env`'s value was not skipped. On
0.155.1 `--worktree=x --image=foo resume --help`, `-ifoo fork --help` and
`"caller prompt" resume --help` all printed the continuation's help with exit 0 and left
a checkout: six forms of twenty-five went through.

Main decided not to follow Codex's grammar: `resume` or `fork` as a whole argument
before `--` refuses, whatever it would have been
([launch.md](../launch.md#a-worktree-for-a-launch)). A prompt that is the bare word is
refused as well, safely, with a hint to put it after `--`. The launch's parser stays for
what it decides — permission overrides on a continuation, and whether the server starts
a fork — and the refusal no longer asks it. `TestAWorktreeRefusesEveryContinuationForm`
holds the six forms, red on 858f57d on exactly those six;
`TestAWorktreeTakesAPromptAfterTheTerminator` keeps `-- resume` a prompt.

## Second review on the Claude Code side, of 858f57d and d4c5e45

No way was found for rm without `--force` to lose work, and the earlier findings were
closed. What it found, fixed in the same change as the refusal by words:

- **The sweep test still failed**, about once in three hundred runs under a parallel
  `-race` load: the neighbour's own sweep left its descendant running. A process that
  has released its memory but is not yet a zombie reads an empty `environ`, so it
  carries no label and is not found; the sweep counted its pause from the end of the
  case's own group and left on that first empty pass. It counts it from the last find
  as well now ([testing-pool.md](../testing-pool.md#owner-labels)), a fix in the sweep,
  not in the test: of 480 runs of both sweep tests, eight at a time beside
  `go test -race ./internal/...`, the old sweep failed 2 and the new one none.
- **A record whose repository is gone could not be removed**, even with `--force`, which
  the refusal promised: every look asked git in a Git directory that no longer existed.
  A regression against `c27fce7`, whose prune skipped a missing repository. Such a
  record now counts as forgotten: with its directory gone rm removes the record, with it
  there rm refuses and `--force` removes both (`TestARecordWhoseRepositoryIsGone`,
  `TestWorktreeRmOfACheckoutWhoseRepositoryIsGone`, both red before).
- **A session started with `-C <checkout>` from elsewhere is not seen by rm**: it
  registers the wrapper's directory. Not fixed; written down in
  [launch.md](../launch.md#a-worktree-for-a-launch) and queued with `claude -w`, which
  has the same gap ([work-queue.md](../work-queue.md#also-queued-not-scheduled)).
- **A dangling link as the worktree root**, pointing into the repository, failed at
  `mkdir` with exit 1 instead of being refused with 2. The root's links are followed
  now even when their target does not exist yet.

Main kept git in a session of its own; the cost, that Ctrl-C does not stop a long
`git worktree add`, is in [launch.md](../launch.md#a-worktree-for-a-launch).

## Final acceptance of c7fdfdf

Accepted on both sides on September 27, 2026. On the Codex side all 25 continuation
forms, on 0.155.1 and 0.157.1, were refused with exit 2 before a checkout, the nested
`exec` and `e` forms too, and no other spelling of a continuation was found. On the
Claude Code side the sweep tests ran 600 times under parallel `-race` load with no
failure, and the full workflow suite passed.

## What stays open

- A relative dangling root link reached through a symlinked directory is resolved
  lexically, so the inside-the-repository check misses it; the launch then fails at
  `MkdirAll` with exit 1 rather than refusing with exit 2. No work is at risk.
- A checkout whose repository was moved, not deleted, is refused as "gone"; the refusal
  could point at `git worktree repair` from the new place, as the missing-directory one
  does.
- The launch's continuation parser still reads a joined image value and a prompt before
  the subcommand as the terminal does not. Without `--worktree` that costs two smaller
  things: `--image=foo fork` is not marked as a fork at startup for the gateway, and a
  continuation spelled so with a permission flag goes without the note that the remote
  terminal rejects it.
- A continued conversation in a new checkout, should the owner want one: hand the
  terminal the checkout as `-C` and check the permissions of the continuation first.
- A process rewake did not start in a checkout is not seen by rm, nor a rewake session
  started in it with `-C` from another directory.
