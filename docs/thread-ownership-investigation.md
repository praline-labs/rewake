# Loaded roots do not identify the terminal's conversation

Investigation on September 17, 2026. Installed CLI: 0.154.0. Source snapshot:
`44b9011611e1f4213ef34bd51b33476475803a94`. Source paths below are relative to
the upstream Rust workspace. The original investigation launched no native harness;
its live inspection used initialization, loaded-list and metadata-only thread/read,
without subscription, resume, history reads or state repair. Later isolated native
execution and the narrow test-service metadata exception are recorded below.

## What the incident establishes

The task was read at 19:08:06 +03:00. Its `LINK-WRITE-OK` report was published
at 19:08:08, with a completed publication receipt and the correct reply id and
recipient epoch. The recipient refused its announcement because two loaded
roots qualified, wrote failed status, then moved the report into done.

Subsequent metadata inspection found two non-ephemeral roots. Both had source
vscode, originator rewake, threadSource user, no parent, and the verified CLI
version. Both creation times predated this wrapper launch. One was idle and
one active. These statuses do not prove which thread the terminal displays.
This evidence rules out a newly created ephemeral startup thread for those two
identities; it does not reconstruct their resume/selection sequence.

The receiving session subsequently supplied its own CODEX_THREAD_ID and wrapper
epoch. They match the active root and addressed run in this capture, confirming
the intended recipient for this incident. This is session-supplied evidence,
not a general rule that an active root belongs to the terminal.

## Independently reproduced observer defect

The observer's thread/resume adds a subscription for its connection. Switching
the selected root overwrites subscribedThread without removing the previous
server subscription. A late successful resume can also leave an untracked
subscription when its generation no longer matches.

Upstream behavior explains why that matters:

- `tui/src/app/session_lifecycle.rs:948–967`: /new starts the replacement before
  detaching the old tracked threads.
- `:1245–1285`: resume attaches its target, then detaches the previous current
  thread when the target differs.
- `:809–825`: a late startup result is detached if no longer wanted.
- `tui/src/app/thread_routing.rs:41–49`: shutdown_current_thread unsubscribes
  the TUI connection; it does not force-close the server thread.
- `app-server/src/request_processors/thread_processor.rs:1014–1041`:
  unsubscribe removes only the requesting connection's subscription.
- `app-server/src/request_processors/thread_lifecycle.rs:56–63`: unloading
  requires no subscribers and an inactive thread. The default delay is 60 s
  (`core/src/config/mod.rs:3797–3798`). An observer prevents that countdown.

A model-free socket regression attaches the observer to A, announces active B,
then subscribes to B. It fails because A remains subscribed. This demonstrates
the retention mechanism, not the exact unrecorded sequence in the live incident.
Even correct unsubscribe would not immediately remove A from loaded-list.

## Why changing the tie-breaker is insufficient

Loaded-list exposes ids and a cursor; metadata exposes provenance and execution
status. Neither exposes the currently selected TUI thread or per-client
subscriptions (`app-server-protocol/src/protocol/v2/thread.rs:1612–1629` and
`thread_data.rs`, Thread). Global thread/started and status notifications carry
no requesting connection identity (`thread.rs:1930–1941`). Resume can omit
thread/started entirely, as already documented in server-observation.md.

Selecting the latest or active root, keeping an old cached root across an
unknown hint, or assuming a unique loaded root is still attached can deliver
to a conversation the terminal has left. A closed thread may be reloaded by
resume after the loaded-list/read check; this is not an attach-only primitive.

A robust policy needs terminal-originated selection evidence, invalidation on
detach/disconnect, and generation-bound delivery. Observer subscriptions must
follow that evidence and release obsolete and late subscriptions. Missing
evidence must refuse delivery, never resume an arbitrary persisted thread.

## Completed research and current owner scope, September 17, 2026

**Owner decision after research:** only rewake may be changed. Native Codex TUI
and app-server modifications are not available. Registering existing hooks is
permitted. The researched explicit selection/generation extension is therefore
a theoretical design outside the approved scope, not a recommendation to
implement. No ownership architecture has been selected or implemented.

The following findings were verified in source at the pinned version above,
then independently checked during round fourteen. They extend the original
incident investigation without a new live metadata capture:

- Primary and displayed conversations differ. The TUI assigns primary_thread_id
  locally in `tui/src/app/thread_routing.rs:1519`; child/side navigation changes
  active_thread_id separately (`app/session_lifecycle.rs:673`). Its subscription
  set can legitimately retain several running or blank roots
  (`app/agents_overview.rs:534–566,643–650`).
- A successful start/resume response precedes local primary installation.
  `app/session_lifecycle.rs:1028–1063` calls fallible terminal reset before setting
  the new primary; terminal I/O can fail at `:761–779`. Thus a gateway can observe
  the same successful control exchange before different local switch outcomes.
  RPC intention, connection subscriptions and a quiet interval are not a TUI
  selection acknowledgement. A read followed by turn/start also has a selection
  race; observing metadata alone does not make admission atomic.
- parentThreadId, source and originator are provenance, not present selection.
  A saved child can be explicitly opened as primary. Direct input is a separate
  capability: `app-server/src/request_processors/thread_input.rs:12–35` rejects
  parent-owned V2 children while allowing other source/version combinations;
  `tui/src/app_server_session.rs:377–381` honors canAcceptDirectInput.
- Ordinary turn/start already uses an in-memory get_thread lookup
  (`app-server/src/request_processors/turn_processor.rs:371–386,527–533`;
  `core/src/thread_manager.rs:1552–1558`). It does not cold-resume a missing
  thread. Observer thread/resume can load persisted state and is a different
  operation; neither supplies authoritative TUI selection.
- SessionStart is queued during backend initialization
  (`core/src/session/session.rs:1790–1817`) and consumed by run_turn
  (`core/src/session/turn.rs:320`; `core/src/hook_runtime.rs:126–175`). It cannot
  identify a fresh UI-selected conversation before its first turn, nor does it
  attest cached UI switches. Its name must not be read as a UI-ready hook.
- CODEX_THREAD_ID comes from the producing command's thread
  (`core/src/unified_exec/process_manager.rs:1364–1375`). That command may outlive
  a switch or run in a background thread. It does not attest current primary.

The research audited startup, resume/fork, /new, return to earlier roots, side and
nested navigation, command-center roots, background helpers and reconnect paths.
Twelve synthetic state/source-order checks passed and three protocol-model
mutations were detected. The identical-control-trace counterexample was synthetic,
not captured native traffic; no native extension was built or tested. These
results support the technical limits, not a repaired or live-accepted transport.
Version-string agreement does not establish installed-binary equivalence to the
source snapshot. The pinned unload default remains 60 seconds; documentation for
other versions must not replace that evidence.

Explicit rebinding and registration through a first-turn hook remain unselected
options within rewake's scope, not features or complete ownership contracts.
No latest/active/arbitrary-root heuristic is accepted as a reliable substitute.

### Practical rewake-only intent gateway research

Research completed September 17, 2026; this option is **unselected and
unimplemented**. Its restricted contract would bind to the last successfully
accepted ordinary startup/new/clear/resume intent on the wrapper-owned TUI
connection. It would not attest the currently rendered screen. Existing hooks
remain allowed; native changes remain forbidden under the owner's scope.

Source at `44b901161` distinguishes ordinary TUI request construction from helper
traffic: fresh startup uses startup-thread-start-* IDs; ordinary requests use
integer IDs, while dynamic/temporary helpers have separate prefixes
(`tui/src/app_server_session.rs:1699–1717,2054–2161`;
`tui/src/dynamic_tools.rs:638–648,1230–1236`;
`tui/src/temporary_structured_request.rs:101,131,185`).
Normal resume includes config and runtimeWorkspaceRoots; PreserveExistingThread
starts mostly with defaults, but later MCP configuration can add config. Config
presence alone is therefore not a primary-intent classifier. These implementation
conventions require supported-binary validation, not just a matching version string.

A possible gateway would project method/request/thread IDs, source, non-null field
flags, direct-input capability and errors, forwarding other payload bytes without
inspecting or storing prompts, history or config values. It would track wrapper
epoch, connection incarnation and intent generation: a new recognized request
invalidates readiness; only its matching latest successful reply binds a writable
target. Failed, unknown, read-only or disconnected states stay unavailable. Late
replies cannot restore an old target. Returning by ordinary resume to an earlier
root remains supported; no permanent retired-root blacklist is appropriate.

Delivery admission and TUI lifecycle requests would need shared ordering, with a
bounded wait for turn/start acknowledgement, not model completion. An independent
read-then-send client retains the race. External clients/runtime changes still
prevent a universal atomic ownership guarantee. No uncertain model work should
be automatically resent, and no newest/active/arbitrary-root fallback is accepted.

Cached command-center selection, side/child navigation, fork and reconnect remain
ambiguous: PreserveExistingThread serves selection and non-selection paths;
cached blank roots may switch after only thread/read. Unexplained different-root
activity must invalidate restricted binding, accepting false unavailability.
Same-thread resume can be a local no-op; reconnect can resume a viewed child.
Successful RPC still precedes fallible local rendering. These gaps require separate
classification or explicit rebinding, not an implied screen-selection guarantee
(`tui/src/app/agents_overview.rs:478–513,596–604`;
`tui/src/app/thread_routing.rs:202–225,1636`;
`tui/src/app/session_lifecycle.rs:383`; `tui/src/app/reconnect.rs:65`).

Six synthetic metadata classifier checks passed; no native TUI gateway, WebSocket
proxy or delivery scheduler was tested. Transport/report/permission regressions
would need separate coverage before adoption. Explicit binding and first-turn
hooks remain alternatives; hooks do not cover every empty or cached selection.
No policy approval, implementation or automatically queued diagnostic follows.

### Completed owner-managed metadata probes

The owner-run /new, cold-resume and loaded-endpoint B-to-A probes are complete.
[Probe evidence and build limits](thread-lock-probes.md) distinguish corrected FD
captures from the incomplete initial collector. Latest lock mtime selected A
while the owner confirmed visible B. Unchanged B is not evidence of a failed
update during a genuine earlier switch: its initial /resume may have been a no-op.
A's later birth time and isolated inode/FD reuse controls support lock replacement;
its original native cause remains untraced. The later
[native four-cell matrix](thread-lock-probes.md#native-resume-matrix-september-17-2026)
confirmed conditional runtime/lock recreation for the fingerprinted 0.154.0 binary:
only empty config with no retained subscriber recreated it. Original live request,
subscriber state and process fingerprint were not captured, so attribution remains
open. These lifecycle observations do not establish selected-dialogue ownership.
The narrow test-service metadata exception and unresolved checkpoint discrepancy
are recorded with the matrix; production transcript-reading boundaries remain.
Further watcher/strace recipes are optional and unperformed, not queued work.
No binding option or repair is selected.

## Authorized bounded mitigation

The orchestrator selected bounded mitigation on September 17, 2026: preserve
report visibility and clean up this observer's obsolete subscriptions. No RPC
gateway or upstream change belongs to this patch. The ownership issue remains
open. The completed research and owner's rewake-only constraint are recorded above;
no native selection extension or replacement ownership design is approved.

The observer tracks resume attempts by connection, thread and generation.
Cleanup shares the discovery gate with resume; a selected root change wakes it
even if the replacement is idle without a rollout. Successful late replies do
not establish the current subscription when the generation changed, but their
attachments remain tracked for removal. Definite unsubscribe refusals retry
with backoff. Only the observer's connection sends unsubscribe. Round fourteen
found an open closed-client retention case during failed reconnects (R14-1);
the mitigation does not cover that case yet.

A timeout or cancellation leaves the RPC's server-side outcome uncertain: a
resume could attach after an unsubscribe, or a late unsubscribe could detach
a newer observation. Rewake closes only that observer connection and uses its
existing reconnect path. Upstream checks connection liveness before attaching
(`thread_lifecycle.rs:158–173,695–705`; `thread_state.rs` connection management),
so an operation cannot subscribe a removed connection. The TUI connection and
its thread are never explicitly closed by cleanup. Reconnection can still
refuse ambiguous roots; it does not gain an ownership signal through cleanup.

Old user roots remain discovery candidates. In particular, /new B followed by
/resume A can emit only a status hint for A. Ignoring A permanently would leave
B selected silently. Both late old-root events and a real return invalidate the
cached target; discovery refuses if two roots still qualify. Unsubscribe alone
does not bypass the default 60-second unload delay or authorize a tie-breaker.

## Failed notification preserves the report

For an accepted current-epoch finished/error/stopped report, a failed notice
keeps the text in unread. Status remains failed with its diagnostic and a durable
reportAvailable flag. Removing the queue entry ends automatic notification
attempts. Status/removal retries and recovery retain that distinction; a
concurrent successful read wins. Reading a report never creates another wait.

Expired reports do not acquire readability. Tasks and notify retain their
failure behavior. Shutdown preserves unexpired reports for their epoch, while
reserved reports stay queued/readable for their waiting command. Reservations,
exact report receipts, room/epoch filters and normal unread retention still
apply. Previously archived failures are not silently made unread again.

The original incident was recovered by inspecting the existing failed status,
archived report and publication receipt, then identifying the intended root
from session-supplied metadata. No live file was rewritten and the old report
was not resent. New behavior requires an owner-managed wrapper restart after
review and installation; this investigation performed neither.

## Regression evidence

Two regressions first failed against unchanged production code:

- TestSwitchReleasesPreviousObserverSubscription: observer A remained attached.
- TestFailedReportNoticeRemainsReadable: finished, error and stopped vanished
  from unread after their notification failed.

Both now pass. Additional contract tests cover fresh idle replacement before
persistence, late resume acknowledgement, rapid changes, refused unsubscribe,
connection loss/cancellation, old-root hints and returning via resume. Mailbox
tests cover expiry, shutdown, foreign epochs, read races, status/removal/archive
failures, recovery, reservations, exact receipts and ordinary retention.

Targeted mutations remove report preservation, durable recovery, idle cleanup,
late-resume bookkeeping and the uncertain-connection fence. Each must fail its
corresponding regression. Full repository checks remain the commit gate; these
local protocol tests do not close live ownership acceptance.

## Round fourteen follow-up

The review of `adbb6c2..b1b9d2b` is complete, with open P2 findings R14-1
(closed observer clients retained during failed reconnects) and R14-2 (an active
snapshot discarded during discovery suppresses completion-gap reporting).
Both reproduce with isolated fixtures and do not require ownership ambiguity.
Baseline checks remain green; the new failing reproductions are not fixes.
See [detailed findings](reviews-later.md#review-round-fourteen--complete-with-open-findings-september-17-2026).
