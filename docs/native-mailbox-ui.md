# Native mailbox arrival display

September 20, 2026. Accepted for the owned adapter. Mailbox toolOutput
still carries model context; after its successful turn/start ACK, the gateway also
attempts one display-only completion on the owning primary TUI connection.

The native renderer supplies the heading **Ran**. The command label is
`rewake notice --display-only`, followed by the ordinary short notice. This label
is not a CLI command or flag and is never executed. It denotes the arrival display,
not a completed shell operation. An arbitrary Rewake-only heading is not promised.
During streaming, native rendering can defer the row until the text finishes.

## Scope and transport

The item uses the real reserved primary thread and ACK turn, with a fresh 128-bit
random ID in the private `rewake-notice-display-` namespace. After ACK, the gateway
rechecks epoch, generation, connection, ready binding and unique current owner under
its existing locks. A side view does not redirect the primary's notice. The frame
enters only that connection's downstream queue, bypassing upstream traffic,
admission, telemetry, read clocks and outcome accounting.

The payload is one item/completed with commandExecution, source agent and status
completed. All required fields follow the pinned native schema, including
completedAtMs and cwd. Cwd `/` is an unused display placeholder; process/plugin/script
and duration fields are null. No item/started, turn terminal, child or picker event
is synthesized. Source userShell is excluded because its completion can release
queued human input. No shellCommand or second model input/toolOutput is sent.

Publication is nonblocking and leaves four item slots and 1 MiB of headroom in the
existing 256-item/144-MiB queue. A stale scope or cosmetic queue failure drops the
row without changing accepted delivery, closing transport or replaying mail. Native
socket failures still follow ordinary connection handling. UI visibility is never
an acknowledgement of mailbox consumption or a basis for task completion.

## Transient display is sufficient

Owner decision, September 20: arrival display without persistence is sufficient;
storing the rows may be considered later. Persistence, reconstruction and replay
across resume/reconnect/process restart are outside this stage and do not block its
acceptance. No UI-history store is added. Durable mail, read receipts, no duplicate
announcements and final reports retain their existing contracts.

Native cache/replay behavior is not specified by this feature. In the lifecycle
owner check, the owner reported seeing the row only once at the beginning and no
picker anomaly. That establishes initial visibility and no noticed repeat, not
exact retention, invalidation or exhaustive duplicate behavior.

## Evidence and installed acceptance

Pinned native 0.154.0 SHA-256:
`3188814c35471432d4123203e0eb38e5bddc60226e3d7ddf0e59e649ea140022`.
Reference revision `44b9011611e1f4213ef34bd51b33476475803a94`. The binary generated
the notification schema; source and binary identity are not assumed.

The reviewed prototype passed both independent reviews and isolated protocol tests:
correct downstream recipient, ordinary active exec continuation, streaming, no
cosmetic model-context leak or phantom outcome, queue pressure and scope races.
The owner saw idle/exec/stream rows; the exec was not interrupted and the stream row
appeared after the streamed text. Owner-mode evidence has no full event trace, so
exact visual timing is not inferred from the final screen.

A first owner /resume attempt refused CLI permission overrides. The repaired fixture
moved unchanged workspace-write/approval-never defaults into its private config;
/new/resume then produced no reported refusal. This is fixture setup, not a runtime
permission override. Ordinary user tools/config/permissions and the socket/hook
adapter are unchanged.

The production path has no prototype switch and uses
`rewake notice --display-only` to avoid split parenthesis quoting. Independent
integration review found no defect and reproduced the five checks, default wiring,
scoped display tests and both native synthetic cases. The owner then observed the
new label in a real TUI, accepted it, and exited with clean fixture cleanup. This
was a short label check; earlier exec/stream observations were not repeated or
promoted to exact visual timing evidence.

The owner installed the reviewed executable, SHA-256
`5b5f859de255714604bd610ebed8649403c5d853e5cd817296d583ec301b2586`, and restarted
main/write. A short task returned UI-INSTALLED-OK as an automatic native finished
report, which main read through inbox. Asked about visible Ran rows in the main
window, the owner confirmed seeing them. This accepts the installed display and
task/report path; it adds no UI persistence guarantee or exhaustive lifecycle matrix.

The [review history](reviews-native-ui-2026-09-20.md) preserves the initial resume
refusal, fixture-only correction and all evidence boundaries. The
[current owner-check recipe](native-mailbox-ui-check.md) uses private configuration
without CLI permission overrides. The records themselves — owner runs, fixtures, launch
files — were deleted with that research package on September 22, 2026; the
observations and hashes they carried are quoted in the review history, which is now
the only place they exist. The original marker_missing and fake-child picker findings remain as
recorded in [mailbox acceptance](native-mailbox-acceptance.md).
