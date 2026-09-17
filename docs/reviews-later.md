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
acceptance remain open. The broad fourteenth review round has not run.
Installation and session restarts remain with the owner.
