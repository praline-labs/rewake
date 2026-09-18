# Later review rounds and finished changes

[Back to the roadmap](roadmap.md). Earlier rounds are in [reviews.md](reviews.md).

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
