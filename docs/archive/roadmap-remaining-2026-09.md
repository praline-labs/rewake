# The roadmap's remaining work of 1.x, September 19–29, 2026

The section that closed `docs/roadmap/README.md` until October 18, 2026, moved here as it
stood, its links rewritten from the archive: the owner's decisions of September 19–21, 2026
behind the work queue of 1.x and what the grants and worktrees left open.

## Remaining work — owner decisions, September 19–21, 2026

What comes next, in the order the owner set on September 21, 2026, is in
[work-queue.md](work-queue.md): the rest of the workflow suite, then pinning harness
versions in a disposable environment — its schema half closed on September 23, 2026
([entry](../roadmap/2026-09-23-harness-versions.md)), its behaviour half is still queued — then a
two-way channel for Claude Code, then the parity queue. Launch aliases, which stood second in that order, closed the same day
([entry](../roadmap/2026-09-21-launch-aliases.md)). This section keeps the decisions behind those items.

What the directory grants and the worktrees of September 27, 2026 left open — a grant
restored after a cold resume, a grant from a Codex main, a root swapped after delivery,
`land` and `finish` beside a foreign `git worktree add`, an owner file of grant rules —
is ordered in [work-queue.md](work-queue.md#later-what-the-directory-grants-and-the-worktrees-left-open-in-1x).

The native-notification priority is complete, and so is the Claude Code handoff it
pointed to: orchestration moved to Claude Code on September 21, 2026. The first three
entries of the [parity queue](harness-features.md) closed the same day — HF-12, HF-07
with HF-20, and HF-21 — each on one observed run
([claude-parity-2026-09-21.md](claude-parity-2026-09-21.md)). Queue entries are not
milestones; the remaining ones are listed in the feature map.

Check automation is the [workflow suite](../check-runner.md), recorded in
[the suite entry](../roadmap/2026-09-21-workflow-suite.md): the research answer,
[check-runner-proposal.md](check-runner-proposal.md) with its selected scenarios in
[check-runner-scenarios.md](check-runner-scenarios.md), was accepted on September 21,
2026 and the first scenario is implemented; short console output and a summary file,
and paid and manual tiers as opt-in, are still requirements rather than code.

A command that lists the models and effort levels a harness offers is **deferred**.
Owner decision, September 21, 2026, asked directly: not now. The reconnaissance that
would feed it is done and recorded in [research-launch.md](../research-launch.md) — what can be read
from each harness, how, and at what cost — so the work would start from facts rather
than from scratch. Roughly a day.

It would answer, per harness: which models are available to this account, which effort
levels each of them takes, and what the configured context window is. For Codex the
window comes from the catalogue and a configuration key, with arithmetic. For Claude
Code neither the subcommands that were read nor the catalogue cache carries one — but
the Agent SDK and the data handed to the status line do, and the reconnaissance did not
go there. So the open question is not
"is there a source" but "which of them is worth depending on", and nobody has yet found
a catalogue of windows for every model that does not require starting a session.

The sources are uneven: one protocol method, belonging to the Codex protocol, and three
unofficial ones — a debug subcommand group, an undocumented per-login cache, and the
text of a warning message. On the Claude Code side the documented route is the SDK.

The requirement that shapes it: **a source that disappears must turn its cell into
"unknown", never leave a stale truth standing.** Caches expire, debug commands are not
promises, and an account's list changes. A table that keeps showing yesterday's answer
because today's lookup failed is worse than one that admits it does not know — that is
the same mistake as a check reporting success it did not earn, one layer up.

A lead for the Claude Code telemetry collector, then missing (entry 5 of the
[parity queue](harness-features.md)). Claude Code has a channel where the harness calls
a command **we** name and hands it a structure describing the session, the context
window size among its fields — so the source is the input to a program of ours, not
text meant for a person. rewake already works this way elsewhere: the wrapper sets a
command for the end of a turn, which calls `rewake turn-ended`
(`internal/harness/hooks.go`). A different integration point, the same shape — the
harness calls what we named and hands it data — and that shape looks like a fit for
the collector.

Not proposed, and not to be done: parsing the rendering of the status line. It is a
display for a person, its format is not promised, and a reader of the screen would be
fragile for no reason. What that structure carries was established on September 23,
2026, beside what the hooks carry, in [research.md](../research.md#telemetry-sources-the-status-line-and-hooks):
model, effort and context fill are there, activity is not, and compactions come from
hooks. The collector was built the same day on both, the hooks and a tap on the status
line that keeps the person's own ([entry](../roadmap/2026-09-23-claude-telemetry.md)).

Persistent Git permissions remain a separate future item: explicit orchestrator
event, scope to run and repository, preservation across later owner turns, and revocation. A running turn keeps its prior permission context.
Only validated Git metadata roots are in scope, not tools, credentials or arbitrary
folders. Do not implement that persistence in the current batch feature.
The main-readiness/missing-notice incident stays open separately.
