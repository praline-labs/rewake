# Primary activity and service notices

September 19, 2026. This extends [session state](session-state.md) through the existing
room-main observer and ordinary notify path. Collection remains enabled for every
role; state display and automatic service notices remain main-only.

```text
write-codex: working | context 42% used / 272K | compactions 1
```

## Current activity

Confirmed primary lifecycle snapshots, metadata-only reads and status notifications
supply activity. Labels distinguish idle, working, waiting for approval/input,
not loaded and system error. A known compaction in progress adds `compacting`.
Unknown activity is never inferred to be idle. Active status with missing or
unrecognized waiting flags keeps waiting state explicitly unknown.

Activity has its own observation timestamp, freshness, event version and request
order, independent of settings revisions. Newer status notifications win over
older lifecycle/read responses; newer eligible concurrent reads can still update.
The initial thread-started snapshot cannot replace a later status change. Selection
generations fence new/resume/fork; pre-ACK candidate state waits for positive
primary proof. Side/nested activity cannot replace primary state. Reconnect marks
last activity stale until the new primary supplies a confirmed observation.

Current activity is not last outcome. A failed or interrupted turn does not latch
an error label: a subsequent native idle status displays idle. No last-outcome field
is added. A compaction completion likewise does not force idle or working; the
header shows the latest status at retrieval. Idle alone does not establish that
an assignment was abandoned and triggers no automatic restart or task resend.

## Compaction completion

Only the already-deduplicated canonical primary item completion produces a cue.
Start, failed/interrupted maintenance without completion, deprecated events and
historical/replayed items do not produce completed notices. Counter and side rules
are unchanged. Main receives a no-reply notify with the completed sequence/count, whose text reads
`Rewake: context compacted (compaction <n>).`; its normal state header shows current
primary activity and the latest counter.
The worker is not woken by this notification.

Native readers only update a bounded in-memory tail of 64 completion cues. The
existing snapshot worker persists it; the main observer handles notification I/O.
Each cue has an observed timestamp and the wrapper's unique completed sequence.
Notification identity includes both session epochs and sequence. PutOnce and the
observer cursor suppress retries/reconnect duplicates, including after normal mail
retention. A late main ignores completions observed before its own start and learns
current totals through availability, rather than receiving historical wakeups.

The tail is a bounded best-effort handoff, not an unlimited event log: if observation
or persistence lags beyond its capacity, older individual cues cannot be replayed.
The cumulative completed counter remains independent of this bound. Internal cue
history is omitted from CLI JSON. Already observed completion facts can be queued
before a known departure even if the worker ends before the next main scan.

## Known worker departure

The same main observer remembers available worker epochs and their last snapshots.
Registry removal, a definitive name/epoch replacement or confirmed process death
produces one no-reply departure notice. Native socket reconnect and missing/stale
telemetry do not prove departure. Namespace checks precede process inspection.
An absent PID, a different recorded start time or a zombie process confirms
departure; unreadable or malformed process identity and an unjudgeable namespace
remain unknown. Unknown observations retain the known worker and any pending notice
for retry. Publication rechecks the same evidence inside the receiver mailbox;
it never takes a sender name lock or cleans up registry records. Existing boolean
reachability and registry cleanup behavior are unchanged. Unknown exit causes are
never labeled as crashes. A new main does not announce workers it never observed.

A departure retains the exact old name, epoch, role, harness, room and registered
cwd. Its saved state is explicitly stale, survives removal of the observation file,
and cannot borrow a replacement run's state. The stored state is private notification
metadata; CLI output exposes it only through main's usual telemetry view. Publication
retries retain one pending identity, and durable PutOnce prevents duplicate writes.
No self-disconnect wake loop or notice to an ordinary worker is added.

All identity observations are non-mutating. Notification writes stay in the existing
main observer and use the bounded mailbox acquisition path; failure never aborts
worker startup/shutdown or ordinary reports. Existing queued-notify failure/expiry
and no-blind-replay rules remain. Kernel filesystem stalls are not claimed preemptible.

## Evidence and acceptance

ThreadStatus and active flags were read at reference revision
`44b9011611e1f4213ef34bd51b33476475803a94`,
`app-server-protocol/src/protocol/v2/thread.rs:1636`. The adapter target at the time
was 0.154.0; it moved to 0.155.1 on September 21, 2026, and these reads were not
repeated against it. Exact source/binary equivalence is not asserted.

ACT-1's evidence-aware departure repair and ACT-2's process-fixture isolation passed
independent re-review. Permission errors retain the known worker; fixture assertions
run unchanged in isolated race-enabled child processes. All five repository checks
and the reported failing pair's 30 repetitions passed before the acceptance build.

The owner installed state-only revision 4, then the reviewed activity-3 extension on
September 19, 2026. Final installed binary SHA-256:
`613b9f824d13ae9cff7849a5f3effb5a53cec2fe1fd793bfacc2f18fdf7957e3`.
These are owner-window and registered-peer observations taken while the adapter
target was 0.154.0, not only fixture results:

- Both candidates started with the normal configuration without reconnect or
  flicker. Quit logs contained only peer EOF; native stderr was empty.
- Main displayed actual model, effort and context. Worker restarts supplied automatic
  availability notes with exact identity and state headers, without a list request.
  Finished reports carried the session-name state header and blank separation.
- A replacement main discovered both running workers as already available. Old
  wrappers without activity telemetry displayed unknown, while the new main showed
  working. No historical departure for a previously closed worker was fabricated.
- Restarting write produced a not-registered departure with frozen stale old state,
  then idle availability and a reset count of zero. Restarting general similarly
  produced process-ended departure, stale old state, then idle availability/count
  zero. Process-ended was not presented as a proven crash; no worker was resurrected.
- Manual compaction incremented the observed count from zero to one. On the activity
  build, main received an automatic completed-count-one notice; its header and list
  showed idle, 0% / 828K context and compactions 1. Zero percent is native-compatible
  rounding, not evidence of empty prompt history.
- During a short task, main's list showed general working. Its automatic finished
  report and subsequent list showed idle, 3% / 828K context and compactions 1.
  The worker's real list JSON still omitted telemetry; main's list/inbox included it.

Live settings changes/rejections, approval/input waiting, native system error,
interrupted compaction and continuing automatic compaction were not forced in this
acceptance. Their independent wire/fixture coverage remains separate. No large
context was manufactured to trigger auto-compaction. Earlier main/side transport
acceptance retains its scope; not every side workflow was repeated for telemetry.
The older main-readiness/missing-notice incident remains open as a separate task.
