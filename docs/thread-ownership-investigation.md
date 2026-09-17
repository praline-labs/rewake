# Loaded roots do not identify the terminal's conversation

Investigation on September 17, 2026. Installed CLI: 0.154.0. Source snapshot:
`44b9011611e1f4213ef34bd51b33476475803a94`. Source paths below are relative to
the upstream Rust workspace. No model or native harness was launched; live
inspection used initialization, loaded-list and metadata-only thread/read.
No live subscription, resume, history read or state repair was performed.

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

## Authorized bounded mitigation

The orchestrator selected bounded mitigation on September 17, 2026: preserve
report visibility and clean up this observer's obsolete subscriptions. No RPC
gateway or upstream change belongs to this patch. The ownership issue remains
open; a future solution needs an explicit terminal selection signal or a
separately verified transport design that distinguishes primary selection from
side-thread and background requests without inspecting history.

The observer tracks resume attempts by connection, thread and generation.
Cleanup shares the discovery gate with resume; a selected root change wakes it
even if the replacement is idle without a rollout. Successful late replies do
not establish the current subscription when the generation changed, but their
attachments remain tracked for removal. Definite unsubscribe refusals retry
with backoff. Only the observer's connection sends unsubscribe.

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
