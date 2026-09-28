# Directory grants seen live on 1.0.2

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

## What stays open

- The hook seen approving a write in a session in the `default` permission mode, and the
  grant taken back there at the first rewake command after the report.
- Cosmetic: on Codex the send prints the grant twice — in the delivery line
  (`write granted: <dir>`) and again as `grants <name> write access to: <dir>`.
- The worktree lifecycle and a grant restored after a cold resume, live.
