# Research: live delivery and conversation-selection checks on Codex CLI

Split from [research-codex.md](research-codex.md) by subject on September 27, 2026: that
document carries the standing facts about Codex CLI — delivery, the sandbox, worktrees,
the app-server, workspace roots, the plan tool; this one carries the dated live probes of
delivery and conversation selection under load: a compaction held against a task, a pair
of real sessions driven end to end, and the terminal's selection between two installed
versions.

Tags: **[verified live]** — live on the named versions; **[source]** — read in the
source; **[docs]** — official pages. Facts here age with harness versions: recheck
before touching an adapter.

### A delivery during a long compaction

**[live; Codex CLI 0.155.1; September 26, 2026; a worker's message status and its
telemetry snapshot read afterwards; times as the state directory logged them]** main
compacted a worker whose context was 85% full (about 220K tokens) with `rewake compact`
and sent it a task seven seconds later. What happened, with the rewake of that day:

- The compaction was requested at about 19:41:22.95; the task was sent at 19:41:29 and
  held as `pending` by the mark.
- At 19:42:42.95 — 80 seconds from the request, 73 from the send — the mark's bound
  ended the hold and main's wait at once. The task went as `turn/start`, and the server
  refused it: the status read `failed`, "native request refused: failed to submit turn
  input: ActiveTurnNotSteerable { turn_kind: Compact }; delivery was not retried
  automatically". So a compaction still refuses input at 80 seconds, as the probe of
  September 24 saw at its start ([research-protocol.md](research-protocol.md#compaction-and-interrupt-on-request)).
- main's letter came at the same moment: the compaction "failed", started and not ended
  when the wait did.
- The telemetry counted the compaction at 19:43:06.49, its asker main — about 104 seconds
  from the request — and the context went from 85% to 0%. The compaction had succeeded;
  nothing told main.

The fix of the same day ([2026-09-26-codex-compact-hold.md](roadmap/2026-09-26-codex-compact-hold.md))
holds a compaction seen running up to 10 minutes, takes this refusal for a wait, and
reports a compaction outliving the wait by its end.

### Live messaging checks of September 26, 2026

**[verified live; Codex CLI 0.155.1; September 26, 2026; rewake builds 0ee310e and
fa5ece0]** Two real sessions in a private room, state directory and workspace, both on
a cheap model at effort low, the TUI driven through a pseudo-terminal and the socket
watched by a passive relay that logged methods, ids and timings only. What the
harness showed; what rewake did with it is in
[the record](roadmap/2026-09-26-live-checks.md).

- **Everything reaches the running turn.** A grouped notice of three notes, a task and
  its addendum sent during a 20-second shell command were each answered with the id of
  the turn already running, and the turn read all five and ended once.
- **`/new` is refused while a turn runs**: the TUI answers `'/new' is disabled while a
  task is in progress.` After the turn ended, `/new` switched the conversation, and a
  delivery went to the new one.
- **A short compaction.** `thread/compact/start` was followed 300 ms later by the
  compaction's turn and its `contextCompaction` item; the turn ended 3.58 seconds after
  it started. A `turn/start` sent 29.5 ms after that end was accepted.
- **An interrupt ends the turn, not the command it started.** `turn/interrupt` was
  answered by `turn/completed` with status `interrupted` 27 ms later, while a `sleep 20`
  the turn had started ran on: it wrote its end marker 20 seconds after it started, and
  its `item/completed` arrived after the turn's end. Whether a later version stops the
  command is not read in the source.
- **A Codex session inside another Codex session's sandbox runs no command.** Every
  shell call failed before starting because the nested sandbox could not open its mount
  registry lock ([traps.md](traps.md#a-codex-session-started-inside-codexs-sandbox-runs-no-command)).


### The terminal's selection on 0.157.1

**[verified live; Codex CLI 0.155.1 and 0.157.1; September 26, 2026]** A rewake build of
e3a90cf ran real terminals and app-servers of both versions in disposable containers
with networking disabled, each probe with its own HOME, CODEX_HOME and REWAKE_DIR. No
model call reached a provider: the 0.155.1 control accepted a mailbox turn, which was
interrupted without a model response.

- **0.157.1 was never selected.** Startup and `/new` created threads the server
  answered writable (`canAcceptDirectInput: true`); `/resume` of a persisted
  conversation in the same process and a separate `codex resume <id>` launch got
  successful native replies too. The gateway stayed unavailable on all four, and a
  delivery failed with `delivery thread is unavailable: context deadline exceeded:
  selected conversation is not ready; wait for native resume to finish or select
  /resume or /new`. The 0.155.1 control selected and delivered after every path.
- **What changed is `runtimeWorkspaceRoots`**: 0.155.1 sends an array on these
  requests, 0.157.1 sends null; `permissions` was null on both. The gateway of that
  day asked for one of the two ([gateway.md](archive-1.x/gateway.md#compatibility-and-limits)).
- **The request forms, 0.157.1.** Startup keeps a `startup-thread-start-<uuid>` id;
  `/new` a numeric id and `threadSource: "user"`. An ordinary resume has a numeric id,
  `threadId`, `history: null`, `path: null`, `excludeTurns: true` and no
  `threadSource`. Every one of them carries a `config` object holding
  `web_search: "cached"`; 0.155.1 sends the same key, beside a personality.
- **The reads did not change**: numeric metadata reads before an explicit resume, UUID
  metadata reads refreshing the overview, and after an in-process resume a numeric
  loaded list, one numeric metadata read for each other loaded thread, and a numeric
  `thread/goal/get`. `includeTurns` was left out of those reads.
- **An empty new conversation cannot be resumed**: it has no rollout, and the resume
  failed natively with `no rollout found for thread id ...`. The successful 0.157.1
  resumes reused a private conversation the offline 0.155.1 control had persisted; no
  personal session or transcript was used.

**[source; release tags rust-v0.155.1 and rust-v0.157.1]**

- The ordinary builder of the terminal's configuration overrides
  (`config_request_overrides_from_config`, `tui/src/app_server_session.rs:1795` in
  0.157.1) always writes `web_search`, one of `disabled`, `cached`, `indexed` and
  `live` (`protocol/src/config_types.rs:376`); start, resume and fork all take it. In
  remote mode the roots are not sent: `workspace_roots_from_config` answers none.
- `excludeTurns` is true on the paginated resume; a legacy history makes it false, and
  the serializer then leaves it out (`rollout_history.rs:161`, protocol
  `v2/thread.rs:424`). Requiring it would refuse a valid resume.
- The helpers carry `web_search` too, so only their request-id prefixes, `temporary-`
  and `tui-dynamic-`, tell them apart: the temporary helper writes its own
  configuration with `web_search` `"disabled"` (`tui/src/temporary_structured_request.rs`,
  about line 98), and the dynamic helper's start takes the builder's.
- `PreserveExistingThread` builds a resume of defaults only (`:2065`), not with the
  ordinary builder; `resume_thread` then lets the terminal's tool transport add its
  own server to the configuration (`rollout_history.rs:157`, the key
  `mcp_servers.codex_tui` at `:248`). The transport itself is outside the two trees.
  This resume also attaches subagents and refreshes a cached snapshot
  (`app/session_lifecycle.rs:364`, `app/thread_routing.rs:1662`), so a resume of
  defaults is no selection by itself.

The same day the gateway took the configuration as the terminal's mark
([roadmap](roadmap/2026-09-26-codex-0157-recognition.md)). The five recorded paths are
kept projected — the lifecycle requests with their parameters, the replies' routing
fields — in `internal/harness/codex/gateway/testdata/tui-paths`, and replayed through a
real gateway: each selects what the terminal selected, on both versions. A fork and a
`PreserveExistingThread` resume of 0.157.1 were not driven live; the rules for them rest
on the source above.
