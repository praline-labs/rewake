# A worktree for a Codex launch — September 26, 2026

Earlier the same day the research on Codex's `--worktree` closed with rewake keeping its
refusal ([the research](2026-09-26-codex-worktree.md)): the terminal refuses the flag
beside the `--remote` rewake always passes, and a managed checkout was to be built only
if the owner asked. The owner asked, and chose between two variants on September 26,
2026:

- **A** — repeat Codex's private scheme: its directory layout, a detached `--no-checkout`
  add, `config.worktree`, the binding of the checkout to the thread. Not built: the
  scheme is no contract, and rewake would drift from a new Codex version silently. It is
  in [work-queue.md](../work-queue.md) as something to study later.
- **B** — only the substance: a detached checkout made with the public
  `git worktree add`, and rewake's own record of it. Built.

## What was built

- **`rewake codex --worktree[=<name>]`.** The launch command takes the flag, which never
  reaches Codex, makes a detached checkout of the launch directory's HEAD and starts the
  session in it, at the same place within the repository; `-C`/`--cd` choose the launch
  directory and are taken out. A name is generated when none is given; a taken name, one
  out of shape, a directory outside a working tree and a repository without a commit are
  refusals with exit 2 and a next step. A launch that fails takes its untouched checkout
  back; one that ran leaves it, with a line saying where
  ([launch.md](../launch.md#a-worktree-for-a-launch)).
- **The place**: `$REWAKE_WORKTREES`, else `$XDG_DATA_HOME/rewake/worktrees`, else
  `~/.local/share/rewake/worktrees` — durable, unlike the state directory in `/tmp`,
  outside every repository, one for all of them — with a directory per repository named
  for it and a hash of its Git directory.
- **The record** beside each checkout: repository, source checkout, commit, subdirectory,
  time, and the session — name, room, run and the room's state directory — written once
  the name is claimed (`wrap.Request.OnClaimed`).
- **`rewake worktree ls`** and **`rewake worktree rm <name>`**, in the command table:
  ls with the owner, whether it still runs and what removing would lose, also as
  `--json`; rm through `git worktree remove`, refusing changes, commits no branch holds
  and a running session unless `--force` — widened by [the review](2026-09-26-codex-worktree-review.md).
- **Git metadata grants** needed no change: `taskGitRoots` reads the conversation's cwd,
  which is in the checkout, and to Git the checkout is an ordinary linked worktree. A test
  in `gitmetadata_test.go` resolves one made by rewake beside one made by hand.
- **Trust**: read in Codex's source, a linked worktree takes its trust from its main
  checkout when its metadata has the layout `git worktree add` writes; a test holds
  rewake's checkouts to that layout
  ([research-codex.md](../research-codex.md#--worktree-with-a-remote-terminal)).
- The harness side is one optional interface, `harness.WorktreeHarness`, implemented by
  Codex only; the new package is `internal/worktree`.

## Tests

Unit tests for the package — creation, the subdirectory, names, refusals, a GIT_DIR
inherited from a hook, listing, what removing would lose, removal with and without
force, a checkout already deleted — and for the launch and the command in
`internal/cli`. The workflow case `codex-worktree` runs the Codex fixture with
`--worktree=probe` end to end, with three mutants
([testing-cases.md](../testing-cases.md#a-worktree-for-a-codex-launch)).

## What stays open

- Acceptance on the Codex side, as for every change to the Codex launch path. The first
  round did not accept it; the findings and fixes are in
  [the review](2026-09-26-codex-worktree-review.md).
- A live run against a real Codex: the fixture plays the terminal and the server; that
  the real terminal's trust dialog reads the same key as the source's resolution was not
  checked.
- Claude Code keeps its own `-w`; whether it needs anything under rewake waits for the
  live probe the owner has not yet cleared.
