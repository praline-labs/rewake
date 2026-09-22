# Later review rounds and finished changes

[Back to the roadmap](roadmap/README.md). Earlier rounds are in [reviews.md](reviews.md).

## Expanded checks — done, September 17, 2026

The stricter formatter and all configured linters now pass. Unused helpers were
removed, cleanup errors are explicitly discarded only where they cannot change
the result, and successful file writes retain their checked close path. Spelling,
comments and equivalent expressions follow the configured checks. Delivery,
reporting and signal behavior are unchanged; the suite runs with race detection
and shuffled test order. Earlier review rounds are in [reviews.md](reviews.md).

## Git writes for local continuations — done, September 17, 2026

`resume` and `fork` now keep the metadata grant discovered from launch cwd.
The earlier blanket skip prevented resumed writers from committing. Remote
execution still skips local paths with an explanation.

New `--worktree` support remains deferred after source inspection and sandbox
probes: a managed checkout's private gitdir can stay read-only despite a writable
source `.git`. Both private and common metadata roots are needed, but the private
path is allocated later by the harness. Creating the worktree first remains
supported. Research records the source locations, layout-dependent results and
the wrapper's unchanged launch cwd. Regression tests and mutations protect
resume/fork grants and the honest worktree refusal.

## Review round eleven — done, September 17, 2026

Retention now distinguishes a reserved answer from an ordinary report. Release
starts one finite delivery window, never renewed by retry; receipts outlive the
reports that reference them. Mixed thread comparison checks later deliveries,
and the publication test waits for the harness pid before checking removal.

Agent system text now lives in internal/brief, with per-role
snapshots. Role data no longer carries injected prose; harness helpers are
split into plans, flags, environment, hooks and notices.

The reporting role is now general (--general). Legacy worker records normalize
to general. General, write and main have independent short system briefings
with reviewed snapshots instead of a shared paragraph plus suffixes.

Failed turns now use hook-only error reports, with fallback to the room's main
and local retention for main's own failure. Explicit reasons stay unchanged;
empty received completions after work are textless errors. The legacy notify
failure-observation gap is documented, without reading transcripts.

Notices now include a bounded first-line preview authored by the sender. The
latest available letter supplies the preview and error color; full text remains
in inbox. Empty first lines are not skipped in search of a summary.

Validation: all five repository checks pass. Twenty targeted mutations were
caught. Isolated fake-process runs covered caller arguments, both notice
transports, error routing and blocking error replies. No real harness ran.

## Review round twelve — done, September 17, 2026

Nested-agent completions no longer settle parent tasks: agent_id filters both
success and failure before any mailbox state changes. A root agent_type still
reports normally. The regression covers both child events and the parent result.

Identified turns persist their complete report batch and waiter/message snapshot
before the first publication. Retries reuse recipients, content and report ids;
cleanup removes only that snapshot, retaining later work for its own result.

Preview coverage now measures CJK terminal columns independently of the production
width estimator. A mutation treating wide glyphs as narrow fails at 102 columns.

Validation: all five repository checks, callback regressions, targeted mutations,
and isolated fake-process delivery pass. Failed-turn observation still depends on the harness emitting a callback; the
previous legacy-notify limitation remains unchanged. No real harness was run.

## Review round thirteen — requested fixes complete, September 17, 2026

Reconnect omits the initial pagination cursor and preserves generation guards.
RPC cancellation covers writer contention and frame I/O, with no automatic resend.
Answer receipts identify the printed report, so stopped cannot archive a later
shared finished. Resume discovery uses metadata hints and loaded-list fallback,
without thread/started or history. All four requested regressions and the
generation coverage gap pass, including strict resume and shared-final scenarios.
Full milestone acceptance remains open for the owner.

## Targeted review — done, September 17, 2026

Scope: `5d4d370..d5335d3`, report mitigation and launch naming only. No reproduced
defects. Targeted race tests passed three times; three independent lifecycle/epoch
regressions passed ten times. Answer/receipt/retention tests passed, and all three
review mutations were detected. Six CLI report-preservation scenarios failed on
baseline and passed on `d5335d3`. Main independently passed all five checks on an
exact archive; the built binary records `d5335d3` and `vcs.modified=false`.
The [ownership limitation](thread-ownership-investigation.md) and new-binary live
acceptance remain open. The subsequent broad fourteenth review is recorded below.
Installation and session restarts remain with the owner.

## First input — done, September 17, 2026

Launch adds the role and flow only through the system briefing. The agent reads
guide on its first task; caller input and continuation arguments stay intact.
No automatic model turn or special startup receipt is created. Both adapters
preserve `--` for caller prompts. All five repository checks pass.

## Fresh-thread observation — done, September 17, 2026

Identity no longer implies subscription. Global active starts history-free resume
attempts every 50 ms while the rollout is absent; idle and lifecycle changes stop
them. A short turn can finish before subscription: observed idle without completion
after 500 ms reports "completion not observed", never an invented assistant result.
The fake scopes turn/item events to subscribed clients and delays rollout creation.
Tests cover first input, /new, retries, normal idle ordering and the short-turn gap.
Source evidence and remaining transport limits are in server-observation.md.
All five checks pass. Real-model milestone acceptance stays with the owner.

## Review round fourteen — complete with open findings, September 17, 2026

Scope: `adbb6c2..b1b9d2b`, the full range after round thirteen: continuation
permissions, fresh-thread observation, initial-input removal, report retention,
observer cleanup, naming and explicit-only main. Review completed with two P2
defects; no P0/P1 findings. **Both findings are open; no fixes were made.**
The original five repository checks passed on the pristine archive. New tests
in the isolated review fixture intentionally fail; green baseline checks do not
establish that either defect is fixed. No live model or transcript read was used.

### R14-1 — P2, replacement regression passes: failed reconnects retain closed observer clients

At `b1b9d2b`, `internal/harness/codex/server_observer_cleanup.go:38` only considers
leases whose generation or thread differs. `server.go:139` advances generation
once before reconnect retries, while `server_subscription.go:55` records each
attempted connection. A failed restore closes that connection (`server.go:160`)
but leaves the same generation/current thread, so cleanup never reaches its
closed-client check. Repeated failures retain client objects and their readers,
connection structures and callback state; prolonged retry grows memory.

The fake-process reproduction observed five failed reconnects retaining five
closed clients after about two seconds. A separate transport-only fixture made
resume exceed its attempt budget and retained three closed clients. This second
case does not depend on a particular semantic refusal remaining possible after
metadata reads. Expected: closed-client bookkeeping stays bounded regardless of
whether its recorded thread and generation are still current.

Review-only files: `review_round14_reconnect_test.go` and
`review_round14_timeout_leak_test.go`, with tests
`TestReviewRound14FailedReconnectDoesNotRetainClosedObservers` and
`TestReviewRound14TimedOutRestoreReleasesClosedClients`. The former exercises
maintain/restore; the latter uses a 150 ms fixture budget rather than production's
3-second reconnect budget. Neither starts a native harness.

### R14-2 — P2, replacement regression passes: discovery loses an observed active interval

At `b1b9d2b`, `server_reconnect.go:102–106` discards an active metadata snapshot
when statusSequence changed during subscription. A subsequent idle/systemError
only finishes an existing observation (`server_subscription.go:85–90`), so it
cannot reconstruct the discarded interval.

The strict socket reproduction returns one unambiguous root with active status.
While observer resume is in flight, global idle or systemError arrives; the typed
completion's subscriber snapshot excludes this observer. Later metadata correctly
returns idle. After the grace interval, observation remains nil and no outcome
exists in all three repetitions for both terminal statuses. Expected: retain the
observed active interval, apply its later terminal status, then emit exactly one
`completion not observed` error when no completion arrives.

This is missing coverage in the observation fallback added by `03d5196`, not the
known case where the whole active/idle interval was missed while disconnected.
A short discovered/resumed turn or failure can disappear without the promised
diagnostic. It does not depend on ambiguous root selection.

Review-only test: `review_round14_discovery_gap_test.go`, test names beginning
`TestReviewRound14DiscoveryActive`. An initial fixture returned active on later
reads and masked the defect; the preserved reproduction updates later reads to
idle and fails. No typed result is fabricated and no history is requested.

Source ordering was independently checked against upstream `44b901161`, CLI
0.154.0: `app-server/src/request_processors/thread_lifecycle.rs:335–344` captures
subscribers; `bespoke_event_handling.rs:183–195` publishes terminal status before
typed completion; `thread_processor.rs:2779–2865` returns loaded metadata;
observer resume attaches later in `thread_lifecycle.rs:675–705`.

### Reproduction boundary and positive coverage

The named reproduction tests live in the separate review fixture, not this
repository. With those tests present in an isolated copy of `b1b9d2b`, run:

```sh
go test -race -v ./internal/harness/codex -run '^TestReviewRound14(DiscoveryActive|FailedReconnect|TimedOutRestore)' -count=1
```

The command's selected cases are expected to fail until fixes are implemented;
running it without the review-only tests is not evidence of a fix. Use isolated
state, writable caches and no inherited REWAKE session/epoch/directory/room.

Positive checks covered cursor pagination, RPC cancellation, exact answer
receipts and stopped/final separation, metadata-only resume discovery, observer
cleanup/late acknowledgements, Git grants, report visibility, initial-input
preservation, role/naming/room behavior and CLI output. The concurrency fixture
ran 48 synchronized independent process claims. Six CLI report-preservation
scenarios and independent receipt/epoch cases passed. Empty-initial-cursor and
existence-only-receipt mutations were both detected in separate copies.

The reviewer independently confirmed the ownership research's primary/focus and
local-switch ordering claims. Synthetic selection models remain specifications,
not native/live acceptance. Under the owner's later constraint, only rewake may
change and existing hooks may be registered; native TUI/server extensions are
outside scope. Ownership selection remains open; the later owner-run metadata
probes are complete, with [evidence and limits](thread-lock-probes.md). The September 18 gateway replacement passes dedicated closed-connection cleanup
and pre-reply active/idle regressions for R14-1/R14-2. Independent integration review
and real peer acceptance remain open; see [gateway integration](gateway.md).

## Round fourteen and gateway integration — September 18, 2026

The earlier `adbb6c2..b1b9d2b` review found R14-1 (retained closed observer clients)
and R14-2 (lost discovery-time active intervals). [Historical findings](reviews-later.md#review-round-fourteen--complete-with-open-findings-september-17-2026)
remain recorded. The inline gateway replacement passes dedicated regressions for
both semantics; independent integration and report-fix reviews completed.

The owner accepted the V5 prototype's native fresh/new/resume routing and visible
fresh/NEW/A/B/A delivery, steering, error and keyboard stop. Production integration
now connects reservations, additive Git roots, async callbacks and durable reports;
[design, checks and limits](gateway.md), [native evidence](gateway-native-evidence.md).
CLI/ordinary fork and primary-preserving side now have protocol and integration
regressions; owner decision keeps the address on main. [Report integration repairs](report-publication.md)
address INT-1/2/3/4 and passed focused re-review. Subsequent owner installation
failed startup. [Transport repair and evidence limits](startup-transport.md):
queue repair passed review and isolated owner startup; the usual environment then
confirmed nine size-guard closes. The size repair passed review and owner fresh/resume
checks. Installed task/report exchange passed with side visible and after close, without resume/restart; [scope](gateway-native-evidence.md#installed-primaryside-delivery-acceptance--september-19-2026).

## Session-state review and repairs — September 19, 2026

Independent review of the state/availability submission reproduced six findings:
optional telemetry could acquire an unbounded cleanup/name lock under main's mailbox;
obsolete lifecycle and metadata replies could replace newer settings; rejected
settings stayed pending; failed/interrupted compaction stayed in progress; a full
candidate cache could prevent a subsequently accepted primary from getting a snapshot;
and an empty shell list changed from sessions:null to sessions:[].

Repairs use non-mutating identity reads in CLI and availability paths, request
selection/revision fences, per-request rejection tracking, turn-scoped progress
cleanup, accepted-primary cache admission with separate dedup history, and the
original nil-list shape. The review's correctness repros are permanent coverage;
additional overlaps preserve newer proposals, primary-preserving side and other
turns' compactions. Original submission and failing evidence remain unchanged.
Later revision-4 review and owner state/availability checks accepted these repairs.
The separate deferred readiness/notification investigation is not part of this repair.

### State settings ordering follow-up — September 19, 2026

The six original repros passed focused re-review. Two related regressions remained:
a rejected pre-ACK proposal suppressed a valid lifecycle confirmation, and accepting
one metadata reply incorrectly excluded a newer concurrent read. Both new assertions
passed on the first submission and failed on revision 2; those artifacts are retained.

Revision 3 separates actual confirmed settings presence, mutation/notification fences
and confirmed request serials. Lifecycle seed survives rejection without clearing an
unresolved proposal; newer pre-ACK applied values still win. Later eligible reads
can update settings in either response order, while older replies cannot replace
newer evidence. Notification, rejection and selection boundaries remain fenced.
Exact review repros and expanded ordering tests are permanent coverage. Later review
accepted these ordering fixes; live setting-change coverage remains separate. No
readiness investigation was added.

### Pre-ACK current-turn usage repair — September 19, 2026

Revision 3's settings-order fixes passed review. R3-1 found that lifecycle seeding
with an unresolved proposal could re-arm the usage gate after turn/started was
already observed. Rejection then left fresh usage of that running turn suppressed.
The seed now preserves that boundary and context instead of treating the proposal
as an applied model change. Actual applied-settings barriers remain unchanged.

The exact review reproduction is permanent coverage, with start/resume ordering
cases and applied changes before/after lifecycle ACK. Original WS and R2 repairs
remain covered. Revision 4 subsequently passed focused review and the owner state
checks below; live settings changes were not newly exercised.

### State acceptance and activity extension — September 19, 2026

Revision4 passed focused review. After installation, the owner confirmed state/model/
effort for main, worker availability notices, worker JSON without telemetry, a state
header on finished and manual compaction increment0->1 with rounded0% /828K context.
This does not prove empty history or every model/compaction workflow.

The next extension observes current primary activity independently of last outcome,
notifies main of new canonical compaction completions, and reports known worker
departure with frozen stale old-epoch state. It reuses the existing main observer,
visibility and durable notify paths. Subsequent ACT-1/2 review and owner acceptance
are recorded below. The readiness incident remains outside this feature.

### Departure identity uncertainty repair — September 19, 2026

Activity review confirmed ACT-1: a permission failure reading process identity made
the boolean liveness helper report false, producing a false departure and discarding
the known worker. No other activity/compaction blocker was confirmed.

The notice path now distinguishes confirmed absence, PID reuse and zombie state
from unknown identity. Unreadable/malformed stat data and unjudgeable namespaces
retain both the known worker and any pending notice for retry. Publication uses the
same non-mutating evidence check under the receiver mailbox, without sender name
locks. Existing boolean reachability, registry cleanup and readiness remain unchanged.

The unchanged review reproduction and retry/tail-gap test are permanent coverage.
Additional tests cover wrapper/child uncertainty, recovery, absence/reuse/zombie,
publication revalidation and stable notice identity across an uncertain retry.
Initial focused regressions and five checks passed on the exact production mirror.
Re-review accepted ACT-1 behavior but exposed the ACT-2 suite race below. No install,
commit, owner-session operation or model probe occurred.

### Process-fixture isolation repair — September 19, 2026

ACT-2 re-review found the new process fixtures racing a backend watcher left by the
backend lifetime test. Run cancellation did not join that reader before the next
test replaced the global process reader. The original failing full-suite and
reduced-pair logs remain preserved.

The process-fixture tests now execute their unchanged assertions in separate child
processes using the same test binary, including race instrumentation. Each child
runs only its selected fixture test with a bounded test timeout; errors propagate
to the parent. Production lifecycle and global reader APIs remain unchanged. This
isolates the fixtures from every unrelated test reader without timing or ordering
assumptions. A backend-only join attempt also exposed an unjoined reader in the
interrupt test; its failing log is preserved, and that production change was dropped.

The exact failing pair passes 30 repetitions under the reported shuffle seed and
race detector. Relevant regressions and all five checks pass on the exact final
production mirror. Focused ACT-2 re-review then found no blocker. Independent helper
probes confirmed child assertion/race/timeout failures reach the parent, and all five
checks passed again. Production lifecycle behavior remains unchanged.

### Installed state and activity acceptance — September 19, 2026

The owner installed the reviewed activity-3 candidate after state-only revision 4.
[The scoped live record](session-activity.md#evidence-and-acceptance) identifies the
installed hash and accepted behavior: ordinary startup, model/effort/context,
availability and late-main discovery, worker JSON privacy, finished headers,
working/idle transitions, departures with stale old state and subsequent replacement
availability, and automatic notification of a manual compaction completion.

Live settings changes/rejections, approval/input waiting, native system error and
interrupted/continuing automatic compaction were not forced. Earlier main/side
transport acceptance is unchanged, not a new claim about every telemetry workflow.
The owner authorized the complete feature commit and ordinary upstream push after
successful checks. The old main-readiness/missing-notice incident remains open and
separate from the next native-notification work.

### Grouped inbox revisions — September 19, 2026

Initial compact-table/peek/select implementation passed local checks, but live
acceptance exposed mismatched grouping expectations. Revision 2 incorrectly treated
later arrivals as covered by an already accepted notice; a two-member notice could
cover six unread items. Subsequent revisions fixed immutable membership and kept
explicit main-authorized Git grants attached to their own eligible task batch.

Revisions 3-5 held the next group until inbox overview or native terminal progress.
Review found recovery-order liveness defects, then owner-captured status ordering
(idle before interrupted; systemError before failed without later idle) disproved
an idle-after-terminal gate. Revision 5 used matching native terminals instead.
Original sources, reviews, failure proof and owner observations remain preserved.

Final owner clarification supersedes that whole dispatch gate: messages reach active
work between tool calls and wake idle work. The orchestrator had mistaken this for
waiting for a full native turn/completed. No overview or terminal is a prerequisite.
Revision 6 removes dispatch-progress receipts, owner recovery tracking and overview
counters. Ready accumulated mail uses native start-or-steer without guessing status.
The initial coalescing window remains 150 ms; in-flight arrivals form the next group.
Spaced arrivals may have separate notices once each has been dispatched. Old unread
never replays. Per-message pending backoff does not hold fresh eligible messages.

Destination readiness, reservations, immutable counts/preview, explicit grants,
leases/TTL, report retention and task-completion publication remain. Prompt-delivery
tests replace the obsolete whole-turn suppression expectations. Full inbox/socket
coverage checks active same-turn steering without terminal/peek, idle first input,
readiness accumulation, in-flight ACK arrivals, exact mixed membership and no replay.
Outcome coverage retains completed/failed/interrupted results, including before ACK.
The [native API evidence](native-terminal-progress.md) separates acceptance from
model consumption. Independent revision-6 review found no runtime blocker; P3
launch-role grant wording was corrected. The additional mixed-batch/report test
preserved all four task/question IDs for finished/error/stopped outcomes. Five checks
passed independently. Full-wrapper smoke, owner startup and installed busy/idle
acceptance then passed; [the dated record](inbox-acceptance.md) gives exact scope.
Readiness investigation, native notifications, persistent Git permissions and the
unified check runner remain separate work.

## Native arrival display — accepted, September 20, 2026

[The dated UI review record](reviews-native-ui-2026-09-20.md) covers the two
prototypes, permission-recipe repair, default integration, owner label observation
and installed task/report acceptance. Transient display is the accepted scope.
