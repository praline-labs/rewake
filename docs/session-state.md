# Primary session state for the orchestrator

September 19, 2026. State collection runs for every managed role, including main.
Only a caller whose current registry name, epoch and main role are verified sees
telemetry. Write/general and plain-shell output retain their previous text and
JSON shape. Visibility is a separate CLI policy; no cross-room discovery or
messaging is added. Unsupported adapters and absent snapshots remain unknown.

Owner decisions: collect main's state for future access by another orchestrator,
but expose state only to the current orchestrator now. The final header format
starts with the actual session name and a colon, without a service prefix:

```text
write-codex: working | context 42% used / 272K | compactions 2

from write-codex ...
<original message>
```

Every fetched inbox kind receives the header, including notifications and reports.
Direct question answers use the same rule. [Current activity and service notices](session-activity.md)
add primary status, compaction-complete and known-departure notifications. The [aligned session table](inbox-groups.md#session-table) adds these values and the
confirmed configured primary-thread model and reasoning effort. The header is tool output,
not a native chat message. Agent text, receipt IDs and message storage are unchanged.
JSON nests `telemetry` before each message's original fields; list entries and
direct answers also carry it only for main. Exact token counts are preserved.

## Availability notifications

After the child and inbox service start successfully, the wrapper records initial
messaging readiness. A backend must also have an accepted initial delivery target;
a socket harness must have its messaging socket. This requires no model turn or
token event. Failed initialization is not announced. The marker is not a claim of
continuous connectivity; reconnect does not create a new launch.

Only the current room-main wrapper scans readiness records, once per second. It
publishes ordinary `notify` messages with frozen registered identity: exact name,
role, harness, room and working directory. At fetch, the common main-only header
also includes model/effort and context/compactions from that exact epoch's latest
snapshot, as in list. Unknown state does not delay useful identity. JSON includes
`availability` identity and the usual `telemetry` object. No separate list call is
needed. The note owes no reply and cannot create a successful-report loop.

A main started later labels prior ready workers as already available, rather than
new launches. Deterministic IDs include both main and worker epochs. Publication
retries use the existing durable PutOnce path under the main mailbox lock; live
epoch receipts also prevent repeats after ordinary mailbox retention. Reused names
and a replacement main have different identities. Old wrappers without a readiness
marker are not guessed ready. There is no cross-room scan.

The main waits for its usable delivery target before queuing availability notices.
Storage/publication failures retry without blocking startup. Once queued, normal
notify delivery, failure and expiry semantics apply; uncertain transport failures
are not blindly replayed with a new ID. A failed announcement cannot abort either
wrapper or alter ordinary tasks/reports. Workers receive no announcement telemetry.

## Context and settings

Context occupancy is the last reported `last.totalTokens`, never the accumulated
session total. For known usage U and usable window W above baseline B=12000:

```text
remaining = round(100 * max((W-B) - max(U-B, 0), 0) / (W-B))
filled = 100 - remaining
```

The native small-window branch W<=B yields filled=100 when usage is known.
Missing, null, negative or malformed numeric values stay unknown. Decimal K is
rounded from W/1000 for text only; no additional headroom reduction is applied.
Local additions after the last native update may be absent. This is a timestamped
reported context, not continuously measured prompt size or cumulative spending.

Correlated accepted start/resume/fork replies and applied settings notifications
provide configured model/effort. A proposed settings request or its empty ACK is
not confirmation. An observed metadata-only read can refresh loaded primary
settings; no new metadata RPC or history read is issued for telemetry. If the
experimental notification is absent, values remain stale/unknown until another
confirmed snapshot. Null effort never becomes an invented default. Lifecycle replies
must match the current selection generation and its accepted primary or proved fork
candidate. A lifecycle reply can seed settings when no newer confirmed snapshot
exists: proposals and rejections alone are not confirmation. Seeding does not clear
an unresolved proposal or manufacture an applied-model-change barrier. An already
observed turn boundary and its context survive that initial seed; rejecting the
proposal lets new usage from the same running turn become fresh without another
turn start. Valid pre-ACK applied snapshots retain precedence, and an actual applied
settings change still rejects old-turn usage until a newly observed turn begins.

Metadata reads retain the request generation and a separate proposal/rejection/
notification fence. Confirmed request serials order eligible read replies: a later
concurrent read can update settings, while a late older reply cannot overwrite it.
Accepting a read advances that serial, not the mutation fence. Unloaded/unknown
metadata also advances the ordering watermark without blocking newer eligible reads.
A lifecycle seed never rewinds the watermark of a newer read.

Settings requests mark old measurements stale and keep bounded per-request proposal
identities. A rejection resolves only its own proposal. Once all proposals reject,
prior confirmed settings and valid usage recover; a newer pending proposal remains
pending. This correlation also survives a side workflow preserving the same primary.
At most 256 unconfirmed proposals are retained; overflow stays uncertain until a
confirmed settings snapshot, without unbounded growth. An applied model change clears old
usage/window. To avoid attributing delayed usage from an already running turn to
the new configuration, context waits for a newly observed turn start and its token
update. Initial lifecycle/replayed usage can populate an otherwise fresh selection.
A new primary intent clears the small connection-local cache; resume without a
usage replay remains unknown. Confirmed side opening/closing preserves primary
state; side and nested-thread updates cannot replace it. Unclassified selection
and reconnects invalidate freshness.

## Completed compactions

The counter means unique observed primary `contextCompaction` item completions
since wrapper start. Manual and automatic paths share that canonical event; no
subtype is guessed. Item start can show `in progress` but never increments. Generic
turn completion, compact ACKs, deprecated events and historical items embedded in
resume/fork/read replies do not count. A live completion can count without its start.
An active item also retains its turn identity: the matching failed/interrupted or
completed terminal turn clears progress without incrementing. A different retained
turn cannot clear a newer compaction; this applies to pre-ACK staged events too.

Deduplication is wrapper-scoped by thread and item ID, with turn ID retained for
correlation. It survives new/resume/fork/reconnect; a new wrapper starts at zero.
During a pending fork, the still-live accepted parent remains eligible. Child events
wait in bounded staging until the workflow proves primary or side; side completions
are excluded. Known connection/selection gaps make coverage `partial`; missed events
are not reconstructed from history.

At most 4096 completed identities are retained for a wrapper. Once that bound is
reached, unseen completions stop incrementing and coverage becomes partial; old
identities are never evicted and recounted. Each connection stages at most 64
compaction events and eight candidate thread snapshots. Staging overflow marks
coverage partial without refusing delivery. A positively accepted primary (or proved
fork child) displaces obsolete candidates before allocation, so a full candidate
cache cannot permanently suppress its snapshot. This marks coverage partial and
does not evict the separate completed-compaction dedup history. The displayed count is an observed
lower bound whenever coverage is partial, not an estimate of unseen work.

## Persistence and freshness

An optional generic backend snapshot interface feeds one wrapper-owned worker,
independent of socket readers, completion publication and mailbox delivery.
It samples at most four times per second and atomically replaces one snapshot
(maximum 16 KiB) under `observations/<name-and-epoch-digest>.json` in that room.
There is no event backlog or monitoring process; a storage failure loses telemetry
and is retried on the next sample without failing delivery. Shutdown waits at most
500 ms for this worker; a kernel filesystem stall cannot be preempted. Epoch files
remain with the private room state, retaining only the latest snapshot for each run.

Visibility, sender-state and availability identity checks are non-mutating registry
reads. They do not acquire cleanup/name locks while main's mailbox is held; normal
registry cleanup stays with its existing explicit callers. This avoids coupling
optional statistics to another session's name lock. It is not a claim that arbitrary
kernel filesystem stalls are preemptible. Empty shell/worker lists retain the
preexisting JSON `sessions:null` representation.

Inbox and direct-answer lookup use the message sender's exact epoch, never a
reused name's new run. A missing epoch/snapshot stays unknown. A stopped sender,
a mismatched live registry epoch or a publication heartbeat older than two seconds
marks cached data stale. The snapshot can lag native events by a sampling interval.
JSON separates publication time, last observation, context/settings observation
times, selection freshness, field freshness and compaction coverage. Stale values
may be displayed with explicit labels; cached data is not a fresh measurement.

## Evidence and acceptance

Protocol research used reference revision `44b9011611e1f4213ef34bd51b33476475803a94`,
against native version 0.154.0, which was the adapter target then; the target moved to
0.155.1 on September 21, 2026 without repeating this research. Exact reference
source/binary equivalence is not asserted. Relevant paths: `tui/src/token_usage.rs`,
`app-server-protocol/src/protocol/v2/thread.rs`, `v2/item.rs`,
`app-server/src/bespoke_event_handling.rs`, `request_processors/token_usage_replay.rs`
and `core/src/context_manager/history.rs`. Facts and formulas were supplied in the
worker-state task's research notes; no private transcript or prompt/config body
was inspected.

Deterministic socket, wrapper, storage and CLI tests cover the formula/unknowns,
applied settings and stale windows, pre-ACK staging, side exclusion, manual/live
completion, replay/dedup/epoch reset, reconnect coverage, storage failure, sender
identity, main-only text/JSON visibility, initial readiness, late-main discovery,
room/epoch isolation and availability retry dedup. Revision 4 passed review and
was installed by the owner. Observed state/model/effort, join notices, worker JSON
privacy, finished-report headers and one manual compaction passed owner checks;
[the scoped acceptance record](session-activity.md#evidence-and-acceptance) records them.
The reviewed activity/compaction/departure extension was then installed and passed
the main owner scenarios on September 19: working/idle, worker replacement with stale
old state, automatic manual-compaction notice and worker JSON privacy. The linked
acceptance record preserves unexercised settings/waiting/error/automatic-compaction
cases and the separate open readiness incident.
