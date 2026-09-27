# Worktrees on a branch, landed and finished — September 27, 2026

The checkout `rewake codex --worktree` made on September 26
([the launch](2026-09-26-codex-worktree-launch.md)) was detached: the commits an agent
made there hung off no branch, and taking the work back was done by hand. The owner
decided on September 27, 2026 that a worktree is on a branch of its own, that rewake
lands and finishes it, and that a Claude Code launch gets the same worktree.

## What was built

- **A branch per worktree.** `--worktree=<name>` makes the checkout with
  `git worktree add -b <name>` from HEAD; without a name the generated one is `wt-` and
  six hex digits. A branch of that name already in the repository refuses the launch
  with exit 2 and the next step: another name, or `rewake worktree ls` when the worktree
  is already there. `HEAD` and forty hex digits are no longer names. The record keeps
  the branch; `ls` shows it, and says when the checkout has left it.
- **`rewake worktree land <name> [--into <branch>]`.** A fast-forward of the branch
  checked out in the source, or of `--into`, to the worktree's branch: hashes kept, the
  worktree and its session untouched, any number of times. Checked out somewhere, the
  target moves with `git merge --ff-only` there, and git's refusal over a dirty source is
  passed on with what to do; checked out nowhere, with `git update-ref` against the value
  read. A target that moved on is refused with exit 1 and the rebase to run in the
  worktree. The answer names the commits moved and the old and new commit, also as
  `--json`.
- **`rewake worktree finish <name>`.** It asks everything first — a running session,
  changes, ignored files, a checkout off its branch, a fast-forward that is not possible
  — and refuses with exit 1 moving nothing; otherwise it lands, removes the checkout
  and deletes the branch. No `--force`: that is `rm --force`.
- **rm and the branch.** rm deletes the branch only when another branch, tag or
  remote-tracking ref holds its commits and no checkout has it out; a branch with
  commits only it holds stays, `--force` or not, and rm says how to take or drop them.
  A failed launch that takes its checkout back drops the branch the same way. The
  reachability refusal is unchanged: the branch now holds a checkout's commits, and a
  checkout switched to a detached HEAD, or whose branch was deleted, is the case it
  was.
- **`rewake claude --worktree[=<name>]`.** The long spelling of Claude Code's flag is
  taken for rewake's worktree; `-w` stays Claude Code's own, and beside `--worktree` it
  is refused, as is `--tmux`. Claude Code has no directory flag, so the launch directory
  is the current one. A continuation — `--continue`, `-c`, `--resume`, `-r`,
  `--from-pr`, `--teleport`, `--fork-session`, or a short cluster holding `c` or `r` —
  is refused, since the conversation stays with its directory
  ([research-worktree.md](../research-worktree.md#continuing-a-conversation-and-trust), read in the
  reference source, not run).
- **The refusal of a continuation spells the path out**, for both harnesses:
  `rewake worktree ls` names the checkout, and the continuation runs there without
  `--worktree`.
- **`.worktreeinclude`.** After making the checkout rewake copies from the source the
  files git ignores that the file at the repository's top names, with Claude Code's
  semantics: `.gitignore` syntax, only ignored files, symbolic links skipped, a copy and
  not a link, a wholly ignored directory walked only when a pattern opens it. One file
  serves both harnesses. The copies are in the record, and rm and finish do not count
  them as work while they equal the source's. The semantics come from the owner's check
  of the 2.1.280 binary and from the reference source, which follows links where the
  binary skips them; rewake follows the binary
  ([research-worktree.md](../research-worktree.md#worktreeinclude)).
- **Owner decision, September 27, 2026:** rewake neither carries heavy dependencies into
  a worktree nor installs them. The agent installs them, or the preparation the
  repository describes, best with a package manager that keeps a shared cache. No
  preparation command, no link to the source's dependencies, no shared directory keyed
  by a lock file's hash. `.worktreeinclude` carries only what it lists; without it
  nothing is copied.
- **What was taken from the harnesses' own worktrees**, after review-codex read Codex
  at `67a709665` and 0.157.1 and the reference source of Claude Code:
  - every git call rewake makes clears the inherited `GIT_*` variables but those naming
    the configuration files and the identity, and runs with the filesystem monitor
    off and without the repository's hooks — both set for the one call through
    `GIT_CONFIG_COUNT`, nothing written to the shared configuration; `land` alone keeps
    the hooks, as the person's own merge would run them. Filters stay on, so a file a
    large-file filter keeps is not checked out as its pointer;
  - the name is claimed with an exclusive record before git runs, and a failed
    `git worktree add` takes back the branch its `-b` made, the empty directory and the
    record;
  - rm keeps asking whether a detached HEAD's commits are held, which Codex's removal
    does not;
  - `.worktreeinclude` takes its list from git with NUL separators, skips links and
    makes no directory through one;
  - a name whose branch exists is refused, where Claude Code's `-B` would reset it;
  - a failed launch that cannot take its checkout back says why, where it is, and that
    the launch command runs there without `--worktree`;
  - no shared `core.hooksPath`, and no cleanup that takes untracked files for rubbish.
- The main session's way of working is written into
  [launch.md](../launch.md#a-worktree-for-a-launch): each writer in a worktree of its
  own, the work taken with land and the worktree closed with finish.
- The worktree section of launch.md passed the 400-line limit and moved to
  [worktree.md](../worktree.md); Claude Code's worktree facts went from
  research-launch.md to [research-worktree.md](../research-worktree.md).
- Codex's own worktree, read in the 0.155.1 and 0.157.1 sources, carries nothing
  beyond the commit and runs no preparation; setup at creation is the desktop
  application's ([research-codex.md](../research-codex.md#--worktree-with-a-remote-terminal)).

Records written before this day name no branch; land refuses them, and the code that
tells them apart carries a legacy mark ([legacy.md](../legacy.md)).

## Tests

Unit tests in `internal/worktree` for the branch at creation, a taken branch, land
(fast-forward, a target that moved on, a target checked out nowhere, `--into`, a
repeated land, git's refusal in a dirty source, a detached source), dropping a branch,
the include rules against git as the judge, the copy and its containment, hooks that
run only on land with an inherited `GIT_CONFIG_PARAMETERS` in the way, and a failed
add that leaves nothing. In
`internal/cli`, land and finish as commands, rm with the branch, the Claude Code
refusals, and what a failed launch with a touched checkout says. The workflow case `codex-worktree` gained a land in mid-work, a refused
finish while the worker runs and a final finish, with four more mutants; the new case
`claude-worktree` runs the Claude Code launch with one mutant
([testing-cases.md](../testing-cases.md#a-worktree-for-a-codex-launch),
[its Claude Code case](../testing-cases.md#a-worktree-for-a-claude-code-launch)).

## What stays open

- Review on the same harness as the orchestrator, then acceptance on the Codex side, as
  for every change to the Codex launch path.
- The live check of continuing a conversation inside a rewake worktree
  ([work-queue.md](../work-queue.md)).
- rm refuses with exit 2, finish with exit 1 for the same reasons: rm's code was fixed
  before finish existed.
- The Claude Code facts behind the continuation refusal and trust were read in the
  reference source, not run.
