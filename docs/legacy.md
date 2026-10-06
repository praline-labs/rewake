# Support for old versions: the legacy mark

Code that exists only so an older harness version, or records an older rewake left on
disk, keep working carries one mark. The mark makes such code findable by one search
and tells whoever finds it when it may go. Decided by the owner on September 26, 2026.

## The mark

One comment line, in the code it keeps:

```go
// legacy(<subject> <<bound>): <why it is kept>; remove when <condition>
```

- **subject** — whose old form: `codex` or `claude` for a harness, `rewake` for what
  an earlier build of rewake itself wrote: records in the state directory, the
  environment it passed to a harness.
- **bound** — below which the old form is kept, and only one kind per subject, so the
  mark cannot be read two ways. For a harness, a version `x.y.z`: the first version
  that no longer needs the code. For `rewake`, a date `YYYY-MM-DD`: the day of the
  commit that replaced the old form; builds before it wrote the old one.
- **why** — what the old form is, in a clause: what the old version sends, or what
  the old records lack.
- **condition** — when it goes, in words a reader can check. For a harness it is
  that version dropping out of support; for `rewake`, that no session started by an
  earlier build is registered.

The mark stands on its own line, never at the end of a statement, so a mark is one
line of the search output. An explanation longer than a clause stays in the ordinary
comment beside it. A block of code kept for one reason takes one mark above it; two
separate places take two marks, so each is found where it is. When the code under a
mark also serves a present case — a wall clock kept as well for a host without a boot
clock — the condition says what stays, and the clean-up removes only the old form's use.

A test or a fixture that exists only for the old form carries the mark too, on the
helper, the constant or the fixture list that holds it, so it goes with the code.
Unit tests that merely spell the old form inline, taking it for any terminal's, are
not marked one by one: once the code under the mark is gone they fail, and are
rewritten in the current form as part of the same clean-up.

## Finding them

```bash
grep -rn '// legacy(' --include=*.go .
```

`docs/legacy_test.go` runs with the five checks and holds every mark to the form
above: a comment that holds the mark's word but not in this form fails it, so a
misspelled mark cannot drop out of the search. It also keeps the table below equal to
the code, and fails once a harness mark is due.

## When a mark is due

**A harness mark** is due when the oldest supported version of that harness reaches
its bound. The oldest supported version is recorded below and raised only by an owner
decision, written in the same row with its date. It is not the version pin in
`internal/harness/codex/server_version.go` (`lastObservedServerVersion`): the pin names the
version last observed working and only chooses whether a launch prints a note; an older
installed version may still be supported. When the row is raised, the test turns every
mark below the new floor red, and those marks are removed in the same change.

**A `rewake` mark** is due when nothing written by a build before its date can still
be read. The state directory is under `/tmp` unless `REWAKE_DIR` moves it, and what is
in it lives:

- a session record, as long as its session runs; it is removed when the session ends
  and pruned when found dead;
- finished mail and statuses, a day after they were last written (`keepFinished` in
  `internal/inbox/serve.go`);
- a task still owed, for as long as it is owed — which ends at its reader's next report,
  or with the reader's run.

So the check is: every session in `rewake list` started after the installed rewake
included the change of the mark's date, and a day has passed since the last session
started before it ended. A restart that clears `/tmp` settles it at once.

## Clearing a mark

In one change, in this order:

1. The code under the mark, and whatever only it used.
2. The tests and fixtures of the old form: the marked helpers and fixture lists, the
   recordings they load (`testdata/`), and the inline uses that now fail.
3. The documents that describe the old form as current — listed per bound below —
   rewritten to describe what remains. A verbatim record of an observation stays as it
   was ([AGENTS.md](../AGENTS.md#keeping-the-documentation-true)).
4. The row in the table below.

### Documents per bound

- **`codex <0.157.1`** — the roots and permissions that marked the terminal's requests
  until 0.155.1: [gateway.md](gateway.md#compatibility-and-limits) (the terminal's
  recognition and the reconnect), [gateway-native-evidence.md](gateway-native-evidence.md)
  (the construction of the primary start and resume),
  [thread-ownership-investigation.md](thread-ownership-investigation.md) (the ordinary
  resume's fields), [traps.md](traps.md) (a wrapper built before September 26, 2026 on
  0.157.1), [research-codex-live-checks.md](research-codex-live-checks.md#the-terminals-selection-on-01571)
  (0.155.1 as the control of the probe).

## Oldest supported versions

| Harness | Oldest supported | Decided |
|---|---|---|
| codex | 0.155.1 | The version installed and pinned since September 21, 2026, owner decision; 0.157.1 is supported beside it since September 26, 2026. |
| claude | not set | No mark depends on it yet; set it with the first `claude` mark. |

## Current marks

| File | Mark | Count |
|---|---|---|
| `internal/harness/codex/gateway/connections.go` | `codex <0.157.1` | 1 |
| `internal/harness/codex/gateway/fork.go` | `codex <0.157.1` | 1 |
| `internal/harness/codex/gateway/metadata.go` | `codex <0.157.1` | 1 |
| `internal/harness/codex/gateway/metadata_validation.go` | `codex <0.157.1` | 1 |
| `internal/harness/codex/gateway/state.go` | `codex <0.157.1` | 2 |
| `internal/harness/codex/gateway/state_test.go` | `codex <0.157.1` | 1 |
| `internal/harness/codex/gateway/tui_paths_test.go` | `codex <0.157.1` | 2 |
| `internal/inbox/answer_mark.go` | `rewake <2026-09-26` | 1 |
| `internal/inbox/waiters.go` | `rewake <2026-09-28` | 1 |
| `internal/inbox/window.go` | `rewake <2026-09-26` | 1 |
| `internal/registry/liveness.go` | `rewake <2026-09-16` | 1 |
| `internal/worktree/land.go` | `rewake <2026-09-27` | 1 |
| `internal/role/role.go` | `rewake <2026-09-17` | 1 |
| `internal/sessionstate/store.go` | `rewake <2026-09-26` | 1 |
| `test/workflow/codexshim_client_test.go` | `codex <0.157.1` | 1 |
| `test/workflow/codexshim_params_test.go` | `codex <0.157.1` | 1 |
| `test/workflow/codexshim_served_test.go` | `codex <0.157.1` | 1 |
