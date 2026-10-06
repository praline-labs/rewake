# The session record

What `sessions/<name>.json` holds, field by field, and how it is published, updated and
pruned. Split out of [design.md](design.md#session-record) by subject on September 30,
2026, when the record gained the boot; the state directory it lives
in is described [there](design.md#state-directory).

```json
{
  "name": "general-claude-2",
  "room": "default",
  "role": "general",
  "roleReason": "selected explicitly with --general",
  "harness": "claude",
  "servicePid": 12345,
  "serviceStart": 1671399,
  "harnessPid": 12346,
  "harnessStart": 1671402,
  "boot": "0f0e0d0c-0b0a-4908-8706-050403020100",
  "cwd": "/home/u/code/x",
  "startedAt": "2026-09-16T00:08:03Z",
  "messagingReadyAt": "2026-09-16T00:08:05Z",
  "socket": "/tmp/rewake-1000/rooms/default/sock/3f5a9c0e7d21b84c6a0f93e2.sock",
  "ownsSocket": true,
  "pidNamespace": "pid:[4026531836]"
}
```

- `serviceStart`, `harnessStart` — field 22 of `/proc/<pid>/stat` (start time in
  ticks). Liveness = the process exists and the start time matches: pids get reused.
- `boot` — the machine's boot id, part of the epoch `<pid>.<ticks>.<boot>`: a pid and its
  start recur after a restart of the machine, the boot id does not.
- `messagingReadyAt` — when the session first became ready to take messages; it marks
  that the start succeeded, not that delivery works now. Absent until then.
- `ownsSocket` — this session created the socket path, so it removes it when it ends;
  absent for a harness without a socket, and for one whose socket path the caller named.
- `cwd` — the directory rewake was launched from, taken when the name is claimed. A
  harness that moves itself afterwards is not followed: Claude Code launched with `-w
  <name>` runs in `.claude/worktrees/<name>`, while the record, `rewake list` and the
  availability notices show the launch directory, a known discrepancy (live, 2.1.280,
  September 26, 2026, [research-launch.md](research-launch.md#a-worktree-at-launch)).
- `codexHome` — the `CODEX_HOME` a Codex session runs with; absent for other harnesses.
- `pidNamespace` — the pid namespace the two pids belong to: a reader in another one
  cannot judge whether they are alive, and does not try.
- A session is alive as long as both the servicing process and the harness are
  alive. A listing or a lookup deletes a dead record it reads, but only when the
  record's name lock is free at that moment (`LOCK_NB`): both are reads and never wait
  on a lock — a lookup runs inside `turn-ended`, the foreground Stop hook, where a wait
  would stall the end of a turn. Whoever holds the lock is the run leaving or another
  reader cleaning up, so a record left now goes with the next read, and a dead record
  is reported as no session either way. Publishing still takes the lock and waits,
  replacing a dead record under it. Decided September 23, 2026 over a bounded wait,
  which would still make a read wait on something it cannot see.
- Publishing a record is atomic and exclusive: write a temp file, then `link()`
  it to the final name — `link` fails if the name is taken. If the existing
  record belongs to a dead session, it's removed and the attempt retried; for a
  live one, the name stays taken.
- Updating one's own record (for example, the harness pid after launch) uses a
  temp file and `rename()`.
