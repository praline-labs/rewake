# Milestone 4. Codex: launch and delivery — done, September 16, 2026

- `internal/harness/codex`: finding the current thread from open lock files,
  chosen by mtime; delivery via `codex queue`; parsing its errors (`no rollout
  found` → `pending`, `No active session` → `failed`); `CODEX_HOME` in the
  session record.
- The Codex wrapper: the intro via `-c developer_instructions`, concatenated
  with the user's value; the state directory added to `writable_roots` under
  `exclude_slash_tmp`.

Tests: thread lookup on a fixture of a process tree and a `/proc` directory;
picking the most recent lock; classifying `codex queue` errors by their text.

**Live criterion:** `rewake codex`, then `rewake send` before the first message
— exit code 3 and `pending`; after the session's first turn, the same message
gets delivered and Codex starts a turn on its own. Separately: `codex sandbox -P
:workspace -- rewake send …` delivers to Claude Code — verifying the inbox path,
without calling the model.

Met on September 16, 2026, in full: the message sent before the first turn came
back as pending with its reason, the thread id was read from the open lock file,
and after one turn of the session the waiting message was delivered on its own,
Codex started a turn and answered through `rewake send` from inside its sandbox.

The run found a defect no test had. A Codex agent runs its commands in a sandbox
with its own pid namespace, where every process but its own is missing — so
`rewake list`, run from there, judged every session dead and deleted the record
of the session that was running it. Records now carry the namespace their pids
belong to, and a reader in a different one neither reports a session gone nor
removes anything.
