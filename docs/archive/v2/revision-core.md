# Revision: core, infra and host

The packages that know no harness in 2.0, or should: the mail model, the turn-end
records, the registry, the wrapper. The overview and the verdicts are in
[revision.md](revision.md); figures are code / tests in lines.

## Inbox

`internal/inbox`: 7 613 / 9 574, 108 files. Four groups live in it today, mixed: the
mail model, the delivery server the wrapper runs, M1's turn-end records with the
plan/apply seam, and the migration of an earlier build's records.

### The mail model — carry over to `core/mail`

| Files | Lines | Notes |
|---|---:|---|
| `inbox.go` | 313 | `Message`, `Kind`, `Owed`, `Result`, `Put`/`PutOnce`; the `HeldFor` field (64–67) goes with the migration |
| `addenda.go`, `adopt.go` | 56, 173 | task addenda; `AdoptWaits` lets a resumed run take its waits — resume, not cutover |
| `answer.go`, `answer_mark.go`, `answer_reservation.go` | 117, 73, 96 | questions and answers; the legacy mark at `answer_mark.go:64` goes, the mtime fallback for a half-written mark stays |
| `awaited.go`, `claims.go`, `grants.go` | 373, 137, 237 | where an awaited letter is; reading in parts; grants per task |
| `local.go`, `owed.go`, `recall.go`, `sent.go`, `retention.go`, `status.go`, `thread.go` | 558 | owed and sent views, retention, status |
| `notices.go` | 68 | `tellMain` stays; `tellMainHeldMoot` and `TellMainUpgraded` (56–68) go |
| `unread.go`, `waiters.go`, `withdraw.go` | 274, 329, 245 | the legacy fallback of `readAt` to `Since` (`waiters.go:40`) goes |
| `availability.go`, `state_notice.go` | 29 | availability and compaction notices |

### M1 journals, marks and the seam — carry over to `core/mail`

| Files | Lines | Notes |
|---|---:|---|
| `access.go` | 161 | the `fileAccess` seam that every read and write of the barrier passes; the plan reads the same code read-only |
| `journal.go`, `journal_ends.go` | 259, 76 | `TurnJournal`; the fields `Held`, `Successors`, `Notices` (63–75), the held branches of `completeJournal` (172–200) and the `conversionFile` exclusions go |
| `journal_held.go` | 212 | splits, see below |
| `kept.go`, `interim.go`, `marks.go`, `once.go` | 122, 114, 154, 217 | kept answers, interim turn ends, pending marks, `intent`/`published` marks |
| `read_boundary.go` | 255 | the read clock and `ReadBoundary` |
| `records.go` | 264 | the list of record kinds; `turns/*` and `journal/conversion` leave the list |
| `stop.go` | 147 | the stop record, `UnknownRecordError`, `liftStop` |
| `reconcile.go` | 330 | splits, see below |
| `turn_records.go` | 72 | `sweepTurnRecords` stays; `TurnsPath` (13–21) goes |

### The 1.x migration inside inbox

Drop, whole files: `conversion.go` (280), `conversion_decide.go` (216), `held_take.go`
(63). Inside other files about 280 more lines go, named above and below. Tests that go:
`conversion_test.go` (373, which also defines the `conversionLab` fixture),
`conversion_moot_test.go` (51), `held_successor_test.go` (223).

### journal_held.go and reconcile.go, line by line

| File and lines | What | Verdict |
|---|---|---|
| `journal_held.go` 24–35 | `deliverReport` for a run of this build: publish, or moot for an ended recipient | carry over |
| `journal_held.go` 36–88 | `deliverReport` for an earlier build: hold, successor, moot | drop |
| `journal_held.go` 85–143 | `heldSuccessor`, `hold`, `mootHeld`, `tellNotices` | drop |
| `journal_held.go` 145–212 | `publishReport`, `publishMarked` | carry over |
| `reconcile.go` 31–103 | `Reconcile`: plan, stop written or lifted, effects, a second plan to explain | carry over; the `isStopped` branches (60–71, 116–124) simplify once `StoppedError` is gone |
| `reconcile.go` 105–150 | `plan`, `reconcileRecords` | carry over without `convertReceipts`/`finishConversion` (127–136) |
| `reconcile.go` 152–175 | `MailboxStopped`, the gate every changing call asks | carry over without the conversion tail (166–174) |
| `reconcile.go` 177–242 | `mailboxRecords`, `inspectMailbox` | change: no `conversion`, `receipts`, `leftovers`; no earlier receipts (205–219) |
| `reconcile.go` 244–274 | `readEveryWait` | carry over without the check for a wait off the read clock (262–271), which only an earlier writer produced |
| `reconcile.go` 281–330 | `Settle`, `SettleRefusal`, `settledWord` | drop with `rewake settle` |

Callers: `Reconcile` from `cli/turnended.go:132` and `cli/turn_reports.go:112`;
`MailboxStopped` from `cli/inbox.go:140`, `cli/read_ack.go:85,173`, `cli/pending.go:104`,
`inbox/answer.go:80`; `TakeHeld` and `TellMainUpgraded` from `wrap/cutover.go:49,66`.

### The delivery server and held.go

`serve.go` (331), `batch.go`, `announcement.go`, `reservation.go`, `outcome.go`,
`window.go`, `sweep.go`, `watch_linux.go`, `held.go`: 1 593 lines that run inside the
wrapper. They move to `host`; their seams (`Deliverer`, `Reserver`, a channel of
receipts) are interfaces already.

`held.go` (281) receives a receipt for a notice the harness parked, settles or takes it
back, and tells the sender if it never showed (`receive` 142, `takeBack` 180,
`settleHeld` 205). Its only producer is the Claude lane (`harness/claude/lane.go:233`,
`harness/claude/receipt.go:79`); Codex produces no `inbox.Held`. Verdict: change,
decided with the delivery probe. If the mod wakes through `$.prompt.submit`, the
promise that resolves when the turn starts can report the same outcome, and the file
shrinks to that; tests `held_test.go` (223) and `held_late_test.go` (150) follow it.

### Tests

| Group | Lines | Verdict |
|---|---:|---|
| M1 on the journal protocol: `plan_faults`, `plan_pairs`, `plan_writes`, `seam_probe`, `journal`, `journal_unknown`, `pending`, `read_boundary`, `confirm`, `records`, `stop_writes` | 2 091 | carry over |
| M1 on the `conversionLab` fixture: `plan_scenes`, `evidence`, `effect_stop`, `late_unknown`, `reconcile_stop` | 838 | change: keep the cases, rebuild the fixture on journal records; of 11 plan scenes 4 carry over, 7 are about earlier receipts or held reports |
| migration | 647 | drop |
| held notices | 373 | follows `held.go` |
| mail model and delivery, 52 files | 5 625 | carry over |

## Receipt

`internal/receipt`: 876 / 418, imports only `state`. The record of one call operation
keyed by run, conversation, turn and digest, with its lock, binding and sweep. Carry
over to `core/receipt`: every path that changes mail through the tool or the shell
holds one. `shell.go` (132, the shell's evidence for the channel record) stays with the
channel in core.

## Registry

`internal/registry`: 827 / 785. Change, to `core/registry`.

- Drop: `runs.go` (201, `RunRecord`, `BindSuccessor`, `Successor` — "which build a run
  followed", `runs.go:12-19`); from `run.go` `BuildStamp`, `EarlierBuild`,
  `EarlierBuildEpoch`; the field `Session.Build` (`registry.go:67`); `runs_test.go`
  (149). Callers of `EarlierBuild*` all go with it: `cli/self.go:58`,
  `cli/send.go:133`, `cli/turn_reports.go:141`, `wrap/cutover.go:65`,
  `wrap/channel.go:264`, `inbox/notices.go:39`, `inbox/conversion_decide.go:171`,
  `inbox/journal_held.go:25`.
- Keep: `Epoch`, `RunEpoch`, `ParseEpoch`, `ParseRun`, `ObserveRun`, `EpochAlive` and
  `Session.Boot` — a pid is reused after a reboot whatever the build.
- The legacy mark at `liveness.go:40` goes: a record without `pidNamespace` is no longer
  read as local.
- `Session.CodexHome` (`registry.go:55`) moves to adapter data.

## Cutover

`internal/cutover`: 517 / 378. Drop (1.x migration). Its package comment says it proves
that no process of the earlier build can still write a name's mailbox
(`cutover/scan.go:1-4`). One caller in code, `wrap/cutover.go:20,28`; in tests
`wrap/launch_order_test.go`, `wrap/launch_evidence_test.go`, the `/proc` substitute in
`cli/grant_temp_test.go:26` and the ldflag in `test/workflow/timings_test.go:79`.

## Grant, grantauth and worktree

- `internal/grant` (871 / 949): carry over to `core/grant`. Two harness traces become
  adapter answers: the shielded names `.claude`, `.codex`, `.agents`
  (`grant/rules.go:317`) and the `Revoking` outcome, which only Claude Code's hook
  produces (`grant/journal.go:21-24`). `tiers.go` already takes the harness
  directories as a parameter rather than importing them (`tiers.go:39-42`).
- `internal/grantauth` (1 019 + `grantauthtest` 134 / 1 006): the authority, its server
  and `Reconfirm` (750 lines) carry over to `core/grant`; `keeper.go` (269, the Claude
  permission hook's journal) and `state.KeeperAddress` wait for stage 6, where the mod
  answers `classic.PermissionRequest` with `updatedPermissions` itself.
- `internal/worktree` (2 366 / 2 295): carry over to `host/worktree`. It imports no
  adapter; it repeats two Claude Code conventions as its own rules (name characters,
  `name.go:15`; `.worktreeinclude`, `include.go:17-27`). The legacy mark at
  `land.go:56` (a record without a branch) goes with its test lines.

The adapter side of grants — `DirGrantHarness`, `GitGrantHarness`, `GrantIssuer`,
`HookGranter`, `Resumer` (`harness/backend.go:15-60`) — becomes one Permissions
capability.

## Control and channel

- `internal/control` (525 / 484): carry over to `core/control`. Compact, interrupt and
  clear travel as files in the run's control directory; the asker owns removal and the
  answer is written once (`control/control.go:1-12`). Produced by `cli/steer.go` and
  `wrap`; served by `harness/codex/server_steer.go:29` and `gateway/steer.go` on Codex
  and by the plugin module's poll on Claude Code (`plugin.js:198`). It stays the core
  transport; how the mod hears a request is proposal 6.
- `internal/channel` (1 290 / 2 117): change, to `core/channel`. The record of how a
  launch's mail travels — tool, shell or none — folded by event time
  (`channel/record.go:1-6`). Any adapter with a tool needs it. The Claude MCP branches go:
  `Claude Harness` (`record.go:56`), `SessionStarted` and `CallSeen` (`events.go:9,28`),
  `MainReconnect` and the "server gone" class (`notices.go:33,115`,
  `history.go:19-34,292-300`). `selection.go` (240) is the Codex conversation
  selection, and the fold branches on the harness id (`selection.go:57`,
  `history.go:202,287`). None of that stays in the core, which knows no harness: stage 2
  moves it to `adapter/codex` behind a capability, or generalizes it into neutral events
  and data. An import check would not catch it, since comparing a string needs no
  import; the 2.0 rules state it. The selection oracles follow the chosen mechanism and
  keep checking event order.

## Wrap

`internal/wrap`: 2 409 / 4 736. It becomes `host`.

| Part | Files | Lines | Verdict |
|---|---|---:|---|
| lifetime and notices | `signals.go`, `availability.go`, `departure.go`, `session_notices.go`, `compaction_letters.go`, `grants.go`, `grant_resume.go`, `names.go`, `hold_notices.go`, `backend.go` | 1 187 | carry over |
| the launch | `wrap.go` 381, `serving.go` 99, `claim.go` ~85 | 565 | change: no gate announcement, tool choice or MCP start in `wrap.go` (92–96, 132–138, 149–163, 198–205); delivery is chosen by capability |
| the mail tool's MCP lifecycle | `mailtool.go` 171, `mailtool_launch.go` 113, `channel.go` 269 | 553 | Codex-only, to `adapter/codex` and `tool`: the Claude transport (`mailtool.go:62`), the hello timer from the first session start (104–106) and "server gone" go |
| cutover | `cutover.go` 69, `prepareRun` and `writers` in `claim.go` (~35) | 104 | drop |

Tests: cutover 182 (`launch_order`, `launch_evidence`, `grant_temp`) drop; channel and
mail-tool tests 711 Codex-only; the rest, 3 843, carry over — among them
`ownership_test.go` (process group, signals, cleaning only its own socket),
`stop_follow_test.go` and `signals_test.go`, the host's process fault tests.

## Sessionstate, role, brief, alias

| Package | Lines | Verdict | Notes |
|---|---:|---|---|
| `sessionstate` | 395 / 176 | carry over, `core/session` | the snapshot every adapter's telemetry speaks; the wall-clock branch at `store.go:89` goes |
| `role` | 256 / 133 | change, `core/role` | `Find("worker")` (`role.go:68-71`) goes; the playbook lines "On Codex … / On Claude Code …" about grants (`playbook_main.go:41-51`) come from capabilities |
| `brief` | 81 / 62 + 248 golden | change, `core/brief` | the mail-tool text (`roles.go:43`) names the tool set of decision 9; golden files are regenerated |
| `alias` | 432 / 589 | carry over, `host/launch` | imports `harness` only for `Flag` and `MatchFlag`, which move to the adapter API |

## Infra

| Package | Lines | Verdict | Notes |
|---|---:|---|---|
| `state` | 934 / 529 | carry over, `infra/state` | `ToolConfigPath` (`paths.go:70-73`, Claude `--mcp-config`) goes; `KeeperAddress` follows the keeper; the fault seam (`fault.go`, `fault_build.go`, build tag `rewakefault`) carries over |
| `proc` | 301 / 294 | carry over | process tree and identity |
| `boottime` | 91 / 0 | carry over | the boot id; its comment's reference to the cutover goes |
| `buildtime` | 29 / 25 | carry over | ldflags timings for tests |
