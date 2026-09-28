# Directory grants and a worktree seen live on 1.0.2

On September 28, 2026, after the owner restarted the room on rewake 1.0.2 (build
`4f3ea37`; Claude Code 2.1.280, Codex 0.155.1 at the model's high effort), main granted a
directory to a Claude Code write session and a Codex general session, each with a
one-step task.

## What was seen

- **A directory directly in the home directory is broad.** `--grant-dir ~/<dir>` was
  refused with exit 2, naming `--grant-dir-broad` as the next step; a directory one level
  down was granted, and send printed the resolved path it granted.
- **Codex, granted.** The worker ran `printf ... > <dir>/probe.txt` inside its sandbox, with
  no escalation and no approval: exit 0, the file written.
- **Codex, taken back.** The next task, carrying no grant, was delivered with
  `taken back, their tasks reported on: <dir>`; the same command then failed inside the
  sandbox with `Read-only file system`, exit 1.
- **Claude Code, granted.** The Write tool created the file with no refusal and no wait.
  The session ran in auto mode, which may approve a write by itself, and in auto mode a
  grant lives to the session's end by design — so this shows the write was not blocked,
  not that the grant's hook approved it.

## A worktree, live

The owner restarted the write session as `rewake --write claude --worktree=test/probe`;
it came up in `~/.local/share/rewake/worktrees/<repository>/test+probe` on the branch
`test/probe`, and `rewake worktree ls` showed it running and clean. The session made one
real commit there (a line in the work queue). While it still ran, `rewake worktree land
test/probe` fast-forwarded the source's `main` to that commit, hash kept, and left the
session and its checkout in place; `finish` refused with exit 1, naming the running
session, and landed and removed nothing. Once the owner ended the session, `finish` said
there was nothing left to land, removed the checkout, its record and the branch, exit 0.

The same on Codex: `rewake --write codex --worktree=test/codex-probe` came up in its own
checkout; a task sent with `--grant-git` was delivered with "Git metadata roots added", and
the session committed inside its sandbox with no escalation. `land` fast-forwarded `main`
while it ran, `finish` refused while it ran and removed the checkout and the branch after
it ended. One limit showed: from the worktree the sandbox left `~/.cache/go-build`
read-only, so the worker could run `go test ./docs/` from the cache but not the five
checks; main ran them on `main` after landing.

## What stays open

- The hook seen approving a write in a session in the `default` permission mode, and the
  grant taken back there at the first rewake command after the report.
- Cosmetic: on Codex the send prints the grant twice — in the delivery line
  (`write granted: <dir>`) and again as `grants <name> write access to: <dir>`.
- A grant restored after a cold resume, live.
