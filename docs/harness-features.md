# Harness feature map

September 20, 2026, at commit 4cfd3cf. What each harness can actually do today, so
orchestration can move between them without guessing. The map is meant to be edited:
every row has a stable ID, a new harness gets **a new column**, and a row is split
rather than stretched when one cell would have to say two different things.

Registered harnesses — the ones `rewake` can launch — are listed in
`internal/harness/catalog/catalog.go`: Codex and Claude Code. A column may also be
added for a harness that is **not** registered, so the remaining work stays visible;
its header must say `UNREGISTERED/planned`, and such a column carries only
**research** or **missing** marks. A planned column never implies launcher support.

Versions: Claude Code 2.1.270 and Codex CLI 0.154.0 were the versions the delivery
research was gathered on, September 15-16, 2026 ([research.md](research.md)). A
read-only `--version` check on September 20, 2026 reported the same two versions.
Harness versions are not stored in the session registry, so a row can only cite the
version its evidence was taken on.

## Legend

| Mark | Meaning |
| --- | --- |
| **live** | observed working on that harness, with a date and evidence in the row |
| **impl?** | implemented and unit-covered, but no observed run on that harness |
| **missing** | not implemented for that harness |
| **research** | investigated only, or an explicitly paused branch |
| **n/a** | not applicable to that harness by design — never used for a shared feature |

Shared implementation is not evidence of a live run: most of the mailbox lives in
`internal/inbox`, `internal/cli` and `internal/wrap` and is available to both
harnesses, so a cell says **live** only where that harness was observed doing it.
Evidence older than the current versions is labelled with its date; a fresh
regression on today's versions is a reasonable ask, but it is not the same as
"never tested".

## Capability map

| ID | Capability | Codex | Claude Code | Evidence / next action |
| --- | --- | --- | --- | --- |
| HF-01 | Launch, registration, name/role/room selection; `--main` is the only way to become main | live | live | [flow.md](flow.md) Act 1, `internal/registry`. Claude Code launch/delivery closed as milestone 3 on September 16, 2026 ([roadmap.md](roadmap.md)). An incoming message never promotes a session to main. |
| HF-02 | Roles: main silent, write/general reporting; role recorded and explained in the intro | live | live | [flow.md](flow.md) Act 1 step 4, `internal/role`. Shared path; both harnesses observed running under it, Claude Code currently as general. |
| HF-03 | Message kinds task / question / notify; a question to a silent role is refused | live | live | `internal/cli/send_kinds.go`. Claude-to-Claude question and answer, and a Claude-to-Codex task with its report back, observed September 16, 2026 ([roadmap.md](roadmap.md)). Daily task/report use continues on both. |
| HF-04 | Outcome `finished`, correlated to the messages read in that turn | live | live | [flow.md](flow.md) Act 5. Codex: gateway `turn/completed`. Claude Code: `Stop` hook running `rewake turn-ended`, observed September 16, 2026 and in daily use since. |
| HF-05 | Outcome `error` on a failed turn | live (native fixture) | live (dated) | Claude Code passed this on September 17, 2026 with a real usage-limit stop ([transport-milestones-2026-09-17.md](transport-milestones-2026-09-17.md)). Codex native error and scoped callbacks were observed on September 18 with a local synthetic model service ([gateway-native-evidence.md](gateway-native-evidence.md)); that run did not prove durable mailbox/registered-peer delivery. Fresh real-model end-to-end error-report acceptance is not claimed. |
| HF-06 | Outcome `stopped` on a keyboard interruption | live | missing | Derived only in the owned adapter (`internal/harness/codex/gateway/admitted_terminal.go`); observed September 17, 2026. Claude Code has no interruption source, so no `stopped` is produced there; a stopped **message** from a peer still renders as `killed` (`internal/harness/claude/notice.go`). |
| HF-07 | Fixed-membership batching of ready mail, 150 ms window, no replay of announced mail | live | impl? | Shared service `internal/inbox/serve.go`, contract in [inbox-groups.md](inbox-groups.md), Codex acceptance September 19, 2026 ([inbox-acceptance.md](inbox-acceptance.md)). No observed multi-member group on a Claude Code recipient. |
| HF-08 | `rewake inbox` read-all | live | live | Shared CLI (`internal/cli`), run in the agent's own shell. Used daily on both harnesses. |
| HF-20 | `rewake inbox --peek` and `--message <id>` | live | impl? | Same shared CLI; contract in [inbox-groups.md](inbox-groups.md). Used on Codex sessions; no recorded Claude Code use, so not claimed. |
| HF-09 | Delivery wakes an idle session | live | live | Codex: `turn/start` on an idle thread ([native-mailbox.md](native-mailbox.md)), accepted September 19-20, 2026. Claude Code: inbox socket, observed since September 16, 2026. |
| HF-21 | Delivery reaches an already working session without waiting for its turn to end | live | impl? | Codex: four sleep-10 tool calls with mid-work consumption, September 19, 2026 ([inbox-acceptance.md](inbox-acceptance.md)). Claude Code sends `priority: "next"` for exactly this (`internal/harness/claude/claude.go`), but no dated mid-turn observation exists for it. |
| HF-10 | rewake tracks the conversation a message was delivered to, and marks `threadChanged` in reports | live | missing | Codex implements `ThreadTracker` through the gateway ([gateway.md](gateway.md)); `/new` delivery with `threadChanged` observed September 17, 2026. No tracker exists for Claude Code (`internal/harness/thread.go`), so the field is omitted rather than guessed. |
| HF-19 | The harness's own conversation commands (`/new`, `/resume`, `/clear`) keep working under rewake | live | impl? | Both CLIs keep their commands; rewake adds flags for one launch only. Codex side observed with selection fencing; on Claude Code nothing in rewake observes or owns the conversation, so behaviour after `/clear` or `/resume` is untested rather than broken. |
| HF-11 | Telemetry collection: model, effort, context, compactions, activity | live | missing | Collected only from an `ObservedBackend` (`internal/wrap/wrap.go`, `internal/harness/codex/server.go`). Socket harnesses have no collector, so a Claude Code session publishes no snapshots. |
| HF-12 | Telemetry display to a verified main | live | impl? | [session-state.md](session-state.md). The main-only header is role-gated shared CLI, so a Claude Code main should see Codex workers' telemetry — but main is currently a Codex session and Claude Code runs as general, so this has not been observed from a Claude Code main. Acceptance belongs to the orchestration move. |
| HF-13 | Availability and known-departure notices for peers | live | live | [session-activity.md](session-activity.md), `internal/wrap/session_notices.go`. The current Codex main read `Session available` and `no longer available` for a Claude Code peer, including a general-claude restart on September 20, 2026. Identity-only notices are useful as they are. |
| HF-22 | Compaction-complete notices for a peer | live | missing | Same observer, but the event comes from telemetry snapshots (HF-11), which a Claude Code session does not produce. |
| HF-14 | A visible arrival line in the terminal, without an ordinary chat bubble | live | live | Codex: display-only completion accepted September 20, 2026 ([native-mailbox-ui.md](native-mailbox-ui.md)). Claude Code: the socket message is drawn natively as one `●` task-notification line (`internal/harness/claude/notice.go`), observed since September 16, 2026. Different mechanisms, same purpose. |
| HF-15 | Per-message Git metadata grants requested with `--grant-git` | live | missing (recipient) | Only an adapter implementing `SupportsGitGrant` can receive them (`internal/harness/backend.go`, `internal/harness/codex/codex.go`); refusal path in `internal/cli/send_git.go`. The **sender** may be any harness: a Claude Code main can request a grant for an eligible Codex recipient. |
| HF-16 | rewake does not overwrite caller-supplied session instructions | live | live | Codex: generated `developer_instructions` are skipped when the caller passed that key or the config file already sets it, with a launch note; profile launches are refused entirely (`internal/harness/codex/codex.go`). Claude Code: the generated briefing is appended with `--append-system-prompt` and skipped when the caller passed that flag (`internal/harness/claude/claude.go`); the two precedence rules are not identical and only these coded cases are claimed. |
| HF-17 | Permission boundary for workers | live | live | Launch selects the policy; the role decides who may *request* an extra Git grant ([git-grants.md](git-grants.md)), it does not shrink the roots the owner's launch already allows. rewake state is a lock-protected shared directory, so read receipts and reports require write access to `REWAKE_DIR`: a fully read-only policy without a writable state directory is not covered by the current acceptance. A read-only project with separately writable state is a different policy to investigate. |
| HF-18 | Unified automated workflow checks and acceptance records | research | research | Acceptance is written by hand into dated documents ([inbox-acceptance.md](inbox-acceptance.md), [native-mailbox-acceptance.md](native-mailbox-acceptance.md), [native-mailbox-ui.md](native-mailbox-ui.md)). Individual isolated fixtures already produce machine records, but there is no unified repository-owned acceptance runner for either harness; the shared runner is an open research task ([check-runner.md](check-runner.md)). |

## How the Claude Code evidence splits

Three different things are easy to conflate, so keep them apart when updating a row:

- **Historical dated acceptance.** Claude Code launch and delivery closed as milestone 3 on September 16, 2026;
  message kinds and automatic turn reporting were accepted in later milestones
  that day. A real
  `error` report from a usage-limit stop was observed on September 17, 2026. Claude
  Code also served as the orchestrator earlier in the project. These are recorded
  facts on the versions of that week, not open questions.
- **Current ordinary use.** Cross-harness task and report exchange, arrival lines and
  peer availability notices are in daily use right now, with Claude Code as general
  and Codex as main.
- **Untested new features.** Everything built after those milestones — grouped
  batching, selected reads, mid-turn steering, telemetry, the main-only header from a
  Claude Code main — has no Claude Code observation. Those are the **impl?** rows.

## Parity queue

Ordered by what moving orchestration to Claude Code needs first. Each entry names its
closing criterion. Closing a research or limitation-decision item does not make
the capability **live**: that mark still requires implemented, observed behavior
and cited evidence. Research sign-off alone does not implement HF-18.

| # | Rows | Question to settle | Closes when | Start from |
| --- | --- | --- | --- | --- |
| 1 | HF-12 | Does the main-only telemetry header work when main itself is a Claude Code session? | One observed fetch from a Claude Code main showing a Codex worker's model, context and compactions | [session-state.md](session-state.md) |
| 2 | HF-07, HF-20 | Do grouped arrivals and selected reads behave on a Claude Code recipient? | Two close arrivals announced as one fixed group, then read with `--peek` and `--message` | [inbox-groups.md](inbox-groups.md), `internal/inbox/serve.go` |
| 3 | HF-21 | Does a message delivered mid-turn reach a Claude Code session without disturbing the original task? | One observed mid-turn delivery whose original task still reports its own result | `internal/harness/claude/claude.go` |
| 4 | HF-05, HF-04 | Do the September 2026 turn-outcome results still hold on the current versions? | A fresh regression covering one `finished` and one `error` report on today's Claude Code | [transport-milestones-2026-09-17.md](transport-milestones-2026-09-17.md) |
| 5 | HF-11, HF-22 | Is there a supported way to observe a Claude Code session's model, context, activity and compactions? | A source-backed collector and observed model/context/activity/compaction cases; unsupported fields remain explicit pending an owner decision | [research.md](research.md), `internal/sessionstate` |
| 6 | HF-10, HF-19 | Can rewake learn which Claude Code conversation a message landed in, so `threadChanged` stops being silently absent? | A `ThreadTracker` implementation with evidence, or a recorded decision to leave it absent | `internal/harness/thread.go` |
| 7 | HF-06 | Should a Claude Code keyboard interruption produce `stopped` rather than nothing? | An observed interruption produces `stopped` without replay; if the harness offers no signal, document the evidence gap for an owner decision | `internal/harness/codex/gateway/admitted_terminal.go` as the reference shape |
| 8 | HF-15 | Research an explicit, additive grant mechanism for a Claude Code recipient without changing owner permissions implicitly | Supported mechanism and granted/ungranted acceptance, or a documented native limitation awaiting owner decision | `internal/cli/send_git.go` |
| 9 | HF-18 | What does an acceptance runner have to produce before it replaces hand-written records? | The research deliverable in [check-runner.md](check-runner.md) is accepted | [check-runner.md](check-runner.md) |
| 10 | HF-17 | Is a deliberately read-only worker worth supporting, given that receipts need write access? | A recorded decision, with the state-directory implications named | [design.md](design.md), [git-grants.md](git-grants.md) |

## Adding or planning a column

1. A registered harness — a package under `internal/harness/<name>` plus one line in
   `internal/harness/catalog/catalog.go` — gets an ordinary column.
2. A harness the owner wants to track before that work exists gets a column headed
   `UNREGISTERED/planned`, filled only with **research** or **missing**. Say in the
   evidence cell what would have to be built. Keep the column marked planned until registration. Only after registration and
   unit coverage may a cell become **impl?**; **live** still requires a dated run.
3. Fill every row for a new column before adding rows for it. A capability that
   genuinely cannot apply is **n/a** with a reason; a shared feature never is.
4. Add new capabilities as new `HF-nn` IDs at the end, and split a row instead of
   letting one cell mix a live half with a missing half. Never renumber existing IDs:
   other documents and handoffs cite them.
5. Keep this file under 400 lines. If it outgrows that, move the parity queue into
   its own document rather than dropping rows.
