# Roadmap

Work for `docs/design.md` closes only after its acceptance criterion passes. This folder
holds the roadmap as one file per entry, each named by the day it closed and headed by
the words its section carried; this index lists them all. What is still open is named
below; the order of what comes next is in [work-queue.md](../work-queue.md). The
transport milestones of September 17 stay in
[transport-milestones-2026-09-17.md](../transport-milestones-2026-09-17.md), which
preserves owned-server acceptance, continuation permissions, lost reports, naming and
explicit main; the review rounds are in [reviews.md](../reviews.md) and
[reviews-later.md](../reviews-later.md).

Open [intermittent bugs](../intermittent-bugs.md): the unexplained two-root-thread
refusal on September 17, 2026 cleared after recipient restart.

## How the work is run

- One step, one commit; commit message short, subject line only, in English.
- Before every commit, the five checks listed in `AGENTS.md` are green: the
  formatter over the module's package list, `go vet`, `staticcheck`,
  `golangci-lint` and the race-detected, shuffled test run with the session's
  `REWAKE_*` variables cleared. `AGENTS.md` says why two of them take the form
  they do; this file does not repeat the commands, so they cannot drift here.
  No failing check is waived.
- Live runs happen in a separate `/tmp` directory, with their own `REWAKE_DIR`.
  The user's working harness sessions are left untouched.
- Live Codex runs spend subscription quota: cheap model, short messages, no
  more than one turn per check. Warn the owner before a run that spends quota.
- After milestones 3 and 5, a separate review agent passes over the diff:
  looking for defects and for checks that stay green on broken code. Findings get
  fixed before the next milestone.
- A milestone marked **live criterion** is accepted by running the real
  harnesses, not by tests.
- The working rules on delegation, the review chain and what the write session
  may commit were set by the owner on September 17–21, 2026 and live in
  `AGENTS.md`, under "Delegation and review" and "Commits".

## Open right now

- [Milestone 6. Ready for daily use](2026-09-16-milestone-06-ready-for-daily-use.md)
  — the publication is not done; the file keeps what is.
- [The workflow suite](2026-09-21-workflow-suite.md) — in progress since
  September 21, 2026; what exists and what remains are listed there and in
  [work-queue.md](../work-queue.md).

## Entries

Each file opens with the heading its section had. Two hold no dated entry:
[risks.md](risks.md), the risks table, and [later.md](later.md), what is deferred
without a date. Local installation without publishing is in
[install.md](../install.md).

| date | entry | what it records |
| --- | --- | --- |
| September 16, 2026 | [Milestone 1. Skeleton and interface](2026-09-16-milestone-01-skeleton.md) | command table, parsing, overview, help, failures, exit codes |
| September 16, 2026 | [Milestone 2. State and registry](2026-09-16-milestone-02-state-and-registry.md) | state directory, `/proc` reading, session records, liveness, `list` and `whoami` |
| September 16, 2026 | [Milestone 3. Claude Code: launch and delivery](2026-09-16-milestone-03-claude-code-launch-and-delivery.md) | wrapper, inbox, socket delivery, `send` end to end; live criterion met |
| September 16, 2026 | [Milestone 4. Codex: launch and delivery](2026-09-16-milestone-04-codex-launch-and-delivery.md) | thread lookup, `codex queue`, the sandbox pid namespace defect; live criterion met |
| September 16, 2026 | [Milestone 5. Intro and permissions](2026-09-16-milestone-05-intro-and-permissions.md) | the intro, `--no-intro`, the allowed-tools rule; met as a side effect |
| September 16, 2026 | [Milestone 6. Ready for daily use](2026-09-16-milestone-06-ready-for-daily-use.md) | open: the watcher, the sweep, `README.md` and `scripts/pack.sh` are done, publication is not |
| September 16, 2026 | [Milestone 7. Notices instead of pasted text](2026-09-16-milestone-07-notices.md) | one-line notices, `rewake inbox`, message kinds, turn-end reports; live criterion met |
| September 16, 2026 | [Milestone 8. Three kinds of message](2026-09-16-milestone-08-three-kinds-of-message.md) | task, question, notify and what each owes |
| September 16, 2026 | [Milestone 9. Roles](2026-09-16-milestone-09-roles.md) | the role catalogue, main reports nothing, explicit `--main` |
| September 16, 2026 | [Milestone 10. Rooms](2026-09-16-milestone-10-rooms.md) | rooms isolate discovery, addressing and delivery; acceptance verified |
| September 19, 2026 | [Gateway integration](2026-09-19-gateway-integration.md) | pointer to the installed startup and primary/side acceptance |
| September 19, 2026 | [Session state and service notices](2026-09-19-session-state.md) | scoped owner acceptance of state and activity |
| September 19, 2026 | [Compact tables and grouped inbox](2026-09-19-grouped-inbox.md) | grouped ready mail, native start-or-steer, installed acceptance |
| September 20, 2026 | [Native mailbox output](2026-09-20-native-mailbox.md) | native delivery accepted with its evidence limits |
| September 20, 2026 | [Native arrival UI](2026-09-20-native-arrival-ui.md) | the display-only Ran row, transient by decision |
| September 21, 2026 | [The Codex transport pin](2026-09-21-transport-pin.md) | pin moved to 0.155.1 behind one constant; a mismatch warns |
| September 21, 2026 | [Launch defaults from the environment and from settings files](2026-09-21-launch-defaults.md) | model and effort for one launch, strongest source first |
| September 21, 2026 | [The workflow suite](2026-09-21-workflow-suite.md) | in progress: what `test/workflow` runs and what it does not yet |
| September 21, 2026 | [The harness research, split by how a fact is obtained](2026-09-21-research-split.md) | three research documents by how their facts age |
| September 21, 2026 | [Launch aliases, on the project's first dependency](2026-09-21-launch-aliases.md) | `rewake <alias>`, the alias file, the dependency decision |
| September 21, 2026 | [A role-shaped first page](2026-09-21-role-playbook.md) | the role playbook in the briefing and the guide |
| September 21, 2026 | [The task-report scenario, as built](2026-09-21-scenario-task-report.md) | what building the first delivery scenario taught, and what it does not prove |
| September 22, 2026 | [The research packages, folded into the documents that outlive them](2026-09-22-research-packages.md) | archives deleted, their findings written into the surviving documents |
| September 22, 2026 | [The batch-arrival scenario, as built](2026-09-22-scenario-batch-arrival.md) | grouping, previews, per-message reads and no replay, with four product mutants |
| September 22, 2026 | [The mid-turn scenario, as built](2026-09-22-scenario-mid-turn.md) | a delivery steered into a running turn, and what the server decides rather than the product |
| September 22, 2026 | [A summary of a suite run instead of its transcript](2026-09-22-suite-summarizer.md) | `tools/checksummary`: a few lines and a summary.json, built from records rather than prose |
| September 22, 2026 | [A fixture for the Claude Code column](2026-09-22-fixture-claude-code.md) | the scenarios run twice; what the socket column cannot show, and says by name |

## Remaining work — owner decisions, September 19–21, 2026

What comes next, in the order the owner set on September 21, 2026, is in
[work-queue.md](../work-queue.md): the rest of the workflow suite, then pinning harness
versions in a disposable environment, then a two-way channel for Claude Code, then the
parity queue. Launch aliases, which stood second in that order, closed the same day
([entry](2026-09-21-launch-aliases.md)). This section keeps the decisions behind those items.

The native-notification priority is complete, and so is the Claude Code handoff it
pointed to: orchestration moved to Claude Code on September 21, 2026. The first three
entries of the [parity queue](../harness-features.md) closed the same day — HF-12, HF-07
with HF-20, and HF-21 — each on one observed run
([claude-parity-2026-09-21.md](../claude-parity-2026-09-21.md)). Queue entries are not
milestones; the remaining ones are listed in the feature map.

Check automation is the [workflow suite](../check-runner.md), recorded in
[the suite entry](2026-09-21-workflow-suite.md): the research answer,
[check-runner-proposal.md](../check-runner-proposal.md) with its selected scenarios in
[check-runner-scenarios.md](../check-runner-scenarios.md), was accepted on September 21,
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

A lead for the missing Claude Code telemetry collector (entry 5 of the
[parity queue](../harness-features.md)). Claude Code has a channel where the harness calls
a command **we** name and hands it a structure describing the session, the context
window size among its fields — so the source is the input to a program of ours, not
text meant for a person. rewake already works this way elsewhere: the wrapper sets a
command for the end of a turn, which calls `rewake turn-ended`
(`internal/harness/hooks.go`). A different integration point, the same shape — the
harness calls what we named and hands it data — and that shape looks like a fit for
the collector.

Not proposed, and not to be done: parsing the rendering of the status line. It is a
display for a person, its format is not promised, and a reader of the screen would be
fragile for no reason. How much that structure actually carries — model, effort,
context fill, compactions, activity, the things collected from Codex — has not been
established; that is its own reconnaissance.

Persistent Git permissions remain a separate future item: explicit orchestrator
event, scope to run and repository, preservation across later owner turns, and revocation. A running turn keeps its prior permission context.
Only validated Git metadata roots are in scope, not tools, credentials or arbitrary
folders. Do not implement that persistence in the current batch feature.
The main-readiness/missing-notice incident stays open separately.
