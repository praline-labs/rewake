# Compact sessions and grouped incoming mail

September 19, 2026. Owner decision: group all nearby incoming kinds, including tasks,
questions, notifications and reports. Accumulate while delivery is waiting; ready mail reaches active work through native
steering, or wakes idle work, without waiting for the current turn to end. This is separate from the open main-readiness
and missing-notice incident; batching does not claim to repair it.

## Session table

Text list output uses one aligned row per agent. Verified main sees Session, Role,
Status, Model, Effort, Context, Compactions and Age. Room appears once. A common
working directory and common harness appear once in the header; differing values
add Directory and Harness columns. Addresses and values are never cropped. Quoted
paths/settings escape control characters so a value cannot introduce another row.
Unknown, stale and partial observations remain explicit.

Worker and plain-shell tables omit all telemetry columns. Machine-readable list
output retains the existing model, exact values, field visibility and empty-list
shape. Individual inbox messages keep their compact name-prefixed state header;
the table changes only the session list's text presentation.

## Overview, selected read and read-all

`rewake inbox --peek` shows available unread metadata: stable opaque ID, sender,
kind, timestamp and a bounded first-line preview. Text and JSON exclude full bodies.
The preview uses the existing notice sanitizer and a conservative 96-cell budget.
Main-only state remains subject to the same verified identity policy. No waiter,
task read receipt, consumption or causal task-boundary advancement occurs. Peek has
no dispatch side effect and writes no overview counter. Repeated peeks leave every
message available. The overview may become outdated before a
subsequent command; selecting an unavailable ID refuses rather than choosing another.

`rewake inbox --message <id>` prints one available message in its normal full view,
then marks only that message read. Output failure leaves it unread. Tasks/questions
create their normal obligations only when actually read; notifications/reports do
not. Text retains the usual main state header and JSON retains the normal message
view. IDs are copied from peek, not parsed as timestamps or shortened addresses.

Plain `rewake inbox` still reads all available unread messages oldest first.
`--peek` and `--message` are mutually exclusive. `--owed` shows what was already
read and is still owed a report, and takes neither
([delivery.md](delivery.md#reading-again-what-is-owed-rewake-inbox---owed)). All modes use the current epoch and
the existing bounded mailbox lock. Active question-answer leases exclude their
reports from overview, selected reads and ordinary announcements. Reserved, expired,
already-read, unknown and other-epoch IDs cannot be selected as available mail.

## Collection and admission

Final owner clarification, September 19: messages should reach active work between
tool calls and wake idle work. Waiting for an entire native turn/completed was the
orchestrator's mistaken interpretation. Ready new mail is submitted promptly through
the supported native start-or-steer API; no peek, completed task, idle transition,
model-seen acknowledgement or long coalescing delay is required.

Every notice has a fixed member-ID set. The initial collection window is 150 ms,
measured from the first servicing wake and never extended by later arrivals. Mail
accumulating during destination readiness is refreshed into the admitted group.
After native dispatch, that group's membership/count/preview are immutable. Arrivals
during its ACK wait form the next group as soon as another dispatch can proceed.
Old announced-but-unread messages remain readable, never inflate later notices and
never generate another wake. Spaced arrivals may produce separate notices when each
has already been dispatched; there is no promise of one notice over several seconds.

The wrapper keeps destination reservation, metadata/read-phase readiness, per-member
scope and readability checks. Native `turn/start` calls start-or-steer atomically,
so the wrapper does not guess active/idle from a status snapshot, interrupt work or
race a separate status query against starting. Native acceptance is not proof of
model consumption. The [source and live evidence](native-terminal-progress.md)
distinguishes those boundaries. Task completion tracking still publishes normal,
error and stopped outcomes; it no longer gates announcement dispatch.

The same bounded collection rule applies to transports without correlated native
completion. Dispatch-progress receipts and peek overview counters were removed.
Ordinary per-message retry backoff still applies to pending attempts; it does not
hold unrelated fresh eligible mail. A failed or uncertain native dispatch retains
the existing no-blind-replay outcome rules.

Each announced member retains its own destination reservation/readability checks,
ID, body, kind, epoch, receipt and task obligation. Native permission preparation is
followed by final membership revalidation before sending. Consumed, newly reserved
or expired members invalidate that attempt; a later pass rebuilds the eligible set.
A read racing after dispatch remains final. Fixed counts describe dispatch membership,
not a continuously changing count of unread mail.

[Explicit Git grants](git-grants.md) wait with their task's unannounced batch. There
is no permissions-only input for an old notice. Only actual eligible
members can supply grant intent to the native request. Shared admission remains
bounded to three seconds; notification failures retain the existing no-blind-replay
and report-retention behavior.

A startup availability discovery pass publishes its independent peer notices under
one main mailbox lock. The first member cannot become readable until the pass has
finished, and the admission refresh includes the rest of that pass in one wake.
A failed partial publication retains the existing per-epoch receipts for retry.

## Transport and outcomes

Owner presentation correction, September 19: keep the short group-count header and
one indented first-line preview of the latest member, prefixed with sender and kind.
The existing preview sanitizer/budget applies to that whole line. No instructions or
multi-message digest appear in the notice; usage stays in guide/help and briefing.
A single member keeps its own first-line notice.
The multi-message notice's transport identity is a digest of its fixed member IDs, stable
for the same group. Transport-only membership is not serialized into stored messages
or CLI JSON. Each member retains its own ID, body, kind, epochs, status, receipts
and task/question obligations.

The native reservation and ACK track one `turn/start` submission for the group:
empty `input` plus [standalone mailbox `toolOutput`](native-mailbox.md). Active
steering can acknowledge the same native turn for several distinct groups.
Every member shares that ACK, while normal actual reads and causal boundaries still
decide which assignments a completion answers. No new native protocol or renderer
is introduced. Git metadata roots are considered only for an admitted member carrying an explicit
main-authorized grant, with recipient eligibility checked separately. No-flag tasks, report-only groups and ordinary
roles receive no new grant; an arbitrary first member never decides group permissions.
The current local roots are still read without history and extended additively.

The wrapper remembers all accepted member outcomes before writing any individual
status. A failed status write cannot replay another group. Actual reads remain final
even if notification later fails. Unexpired accepted reports remain readable when
only their announcement fails; failed task/notify delivery and TTL expiry retain
their existing rules. Reserved answers are not stolen, and other epochs are not
mutated. Cancellation uses the existing shutdown refusal and report-retention path.

## Evidence

Local tests cover mixed simultaneous arrivals, readiness-wait accumulation,
in-flight ACK arrivals forming the next exact group, and active same-turn delivery
without peek or terminal events. Spaced arrivals dispatch independently. Idle first
input, explicit mixed grants, body-free peek, selected/read-all consumption, leases,
TTL, cancellation, output/status failures and no old-unread replay remain covered.
Outcome tests preserve normal/error/stopped reports, including before-ACK native
terminals and their observed status ordering. Late-main discovery remains grouped.

Revision 6 passed independent review and [installed live acceptance](inbox-acceptance.md)
on September 19. Active mail was consumed between tool calls without waiting for
the original turn to finish; its report survived, and subsequent idle delivery
woke the recipient and returned a report. Untested races are not claimed as live proof.
