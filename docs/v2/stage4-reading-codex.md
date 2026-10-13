# Stage 4 reading: Codex

A dated record, kept as written. The Codex side's review session read Codex for the
[stage 4 audit](stage4-audit.md) on October 13, 2026: the installed codex-cli 0.160.1 by
`--version` and help, its app-server schema generated that day, a reference tree at
`8c0f7011674145da7e10627769fb221c74fbf7a0` read as source, and the `mods` skill of Claude
Code 2.1.296. Nothing was run live. The audit cites its probes as CX1–CX8, which are P1–P8
in its own table under "Checks still OPEN" and P8 under "Joining without the launch
wrapper". The saved help (`cli-0.160.1.txt`) and the generated schema (`schema-0.160.1/`)
it names stay outside the repository, in the audit's working files; their links are
plain names here. By the owner's decision of the same day Codex gets only an entry point
before 2.0 ([the stage plan](design.md#the-stage-plan-from-here)), so this reading waits
for its adapter after the release.

---

A1: use native execution and control, keep cross-harness mail, and seat workers in an externally owned checkout.

Reading record, October 13, 2026. No implementation or live-session acceptance.

## Scope and evidence

Read `docs/project.md` in full, then `docs/v2/design-codex.md`, `ideas.md` sections
4–5, the stage-4 audit requirement, and the worktree and permission research.
NATIVE means use the harness primitive and remove its duplicate in the adapter;
it does not mean deleting the neutral contract that translates that primitive.
SEAT means another owner supplies the resource and its lifecycle. KEEP names the
project property a native facility does not supply. OPEN names a concrete check.
Different verdicts in one row apply to these different responsibilities.

Evidence keys used below:

- **C**: installed `codex-cli 0.160.1`, checked on this date with `--version` and
  help only. `--help`, `app-server --help`, `exec --help`, `resume --help`,
  `plugin --help`, `cloud --help`, `agents --help`, `queue --help` and
  `remote-control --help` are saved verbatim in `cli-0.160.1.txt`.
- **S**: app-server schema 0.160.1, generated on this date with
  `codex app-server generate-json-schema --experimental --out <evidence-dir>`.
  Files are in `schema-0.160.1/`; names below refer to its `v2/`
  directory unless said otherwise. A schema proves a surface, not live behavior.
- **R**: reference tree `8c0f7011674145da7e10627769fb221c74fbf7a0`, read on this
  date; paths below are relative to `codex-rs/`. The checkout's HEAD names that
  commit. This is the newest source locally available, not an assertion about the
  newest released binary. The October 11 recon places it after release 0.162.1;
  new source features below must not be attributed to the installed 0.160.1.
- **D**: dated prior evidence in `docs/research-codex.md`,
  `docs/research-codex-conversation.md`, `docs/research-permissions.md` and
  `docs/research-worktree.md`. Each use below gives its version/date; an old live
  observation is not a new live check.
- **M**: `mods` skill, snapshot 2.1.296 of October 11, 2026. Today's
  `claude --version` returned `2.1.296 (Claude Code)`. Read `SKILL.md`, the map's
  process and classic-event entries, `ProcessSpawnRequest`, `ProcessSpawnResult`,
  `WorktreeCreateHookInput`, `WorktreeRemoveHookInput`, `ClassicResultFields`,
  `ClassicResultOf`, and the corresponding declarations. The skill remains the
  API reference; this record states implications, not a replacement API manual.

CLI help and schema generation exited 0. They emitted a nonfatal read-only PATH
alias warning. No install, configuration write, Git command, Go command, model
call, live worker, shared daemon change or pin change was performed. New files
are confined to this audit's scratch directory.

## Feature decisions

| Row | Installed offer, C/S | Newest source offer, R | Verdict and seating requirement | A main on this harness |
|---|---|---|---|---|
| Launch and registration | Interactive CLI, `-C`, `--remote`, `--no-daemon`; private app-server on stdio or an explicit Unix socket; start/resume/fork return thread identity and effective settings (`ThreadStartResponse`). | `tui/src/lib.rs:1028` distinguishes explicit remote, shared local daemon and embedded operation. Explicit remote selects that endpoint. | **NATIVE** execution; **KEEP** room/name/role/run registration and one-build lease. **SEAT** into the given cwd with a bound endpoint and environment; a per-run server is one candidate, not the only possible join. Starting a bare binary supplies no rewake identity or report route. | Can launch its own threads and processes. It cannot natively register a worker of another harness into rewake's room. Keep the same neutral registration. |
| Mail and reading by tool | `ThreadStartParams.dynamicTools`; top-level `DynamicToolCallParams` has `threadId`, `turnId`, `callId`, tool and arguments; response has content and success. CLI also offers MCP and plugins. | `app-server/src/bespoke_event_handling.rs:1090`, `dynamic_tools.rs:18`, `core/src/tools/handlers/dynamic.rs:186` carry native calls and results. | **NATIVE candidate / OPEN P1** for tool transport: prefer dynamic tools over rebuilding MCP injection. **KEEP** the command table, mail records, bindings and read proof. Run the tools outside the worker's filesystem sandbox; cwd access alone cannot make sandbox-independent mail. | Native same-server task tools exist, but no native mixed-harness task/report ledger. Use the same rewake tools as a worker. |
| Turn boundary and end confirmation | `turn/started`, `turn/completed`; `TurnStatus` includes completed, failed, interrupted and inProgress. `HooksListResponse` knows Stop and Interrupt. No end-confirmation server request appears in `ServerRequest.json`. | Stop can block with feedback and continue the same turn (`hooks/src/events/stop.rs`; `core/src/session/turn.rs:655–710`). Completion notifications are observations, not a veto. | **NATIVE** events; **KEEP** mapping to owed tasks and one report per task. **OPEN P2** for a trusted per-launch Stop hook as the confirmation seam; failure/interrupt need their own handling. Another owner's child-exit event is not the worker's turn end. | Can observe its own turns; cannot infer another harness's final answer from its own turn or a process exit. Neutral accounting remains. |
| Telemetry: context, limits, model, effort | `ThreadTokenUsageUpdatedNotification` carries last/total usage and nullable context-window size; account rate-limit read/update; lifecycle replies and `ThreadSettingsUpdatedNotification` carry model/effort. | Same native stream; server-selected defaults mean request fields can be absent. `tui/src/app_server_session.rs` and October 11 recon show the changed bootstrap. | **NATIVE** measurement, **KEEP** neutral presentation and freshness. Bind notifications to the seated worker's selected thread. Do not read transcripts, scrape the parent's telemetry or infer model/effort from launch requests. | Native own-task UI; rewake must aggregate other harnesses. Account limits are not per-worker quotas. |
| Wake and steering | `turn/start`, `turn/steer` with `expectedTurnId`; CLI `queue`, plus experimental `thread/queue/*`. | Queue service and app-server turn operations are separate mechanisms; TUI task tools can start a follow-up on another thread (`tui/src/dynamic_tools.rs:684`). | **NATIVE** start/steer, **KEEP** durable mail and announcement identity. **OPEN P3** for pinned-version idle/busy/interrupted behavior. A creation hook or parent's wake cannot wake the seated worker. | Native thread prompts are useful within its own server. Mixed-harness addressing and delivery still require the neutral channel. |
| Compaction and interrupt | `thread/compact/start` has only `threadId`; `turn/interrupt` identifies the thread and turn. | `app-server/src/request_processors/thread_processor.rs:2459` submits native compact. No per-call summary instruction in that request. | **NATIVE** operations, **KEEP** main-only routing and result reporting. **OPEN P3** for the optional instruction that the project lets main pass to compaction; do not claim that plain compact implements it. | Native control of its threads; other harnesses still use their adapters. |
| `/new`, resume and conversation binding | start/resume/fork, `thread/loaded/list`, `thread/read`, lifecycle notifications. Resume requires only `threadId`; dynamic-tool definitions occur on start, not resume. | `core/src/session/mod.rs:798` restores saved dynamic tools when none are supplied. TUI resume can send only the id; current TUI requests need not contain the old web-search marker (October 11 recon, R `tui/src/app_server_session.rs`). | **NATIVE** conversation lifecycle; **KEEP** session-to-selected-thread binding, **OPEN P4**. Rebind cwd, tool availability and grants on a switch. An old thread id or persisted tool definition does not prove a live handler for the new run. | The same issue applies to main; a thread list does not identify which thread a person currently selected. |
| Task grants, including Git | CLI sandbox, `--add-dir`, approval policy; schema has runtime roots, sandbox/profile selection, thread settings, and permissions approval requests. | Settings updates acknowledge queuing, not application (`request_processors/turn_processor.rs:829,940–991`). `turn/settings/update` changes reviewer/model/effort/tier, not writable roots. Protected Git paths remain in `protocol/src/permissions.rs`. | **NATIVE** enforcement; **KEEP** task-scoped authority and revocation. **SEAT** with checkout cwd plus the effective roots, private gitdir and common gitdir when Git is granted. **OPEN P5** for active-turn grant/revoke and resume. | Native approvals do not represent a grant from a particular main for one rewake task. Keep that lifetime and authority even if native permissions perform the write checks. |
| Worktree creation and lifecycle | CLI and `exec` expose `--worktree`; `-C` seats in an existing directory. Help alone does not establish lifecycle behavior. | Local managed worktrees, explicit removal browser and create/list tools; creation is detached, and tool creation does not change cwd or grant permissions (`worktree/src/lib.rs:62`; `tui/src/managed_worktree_tool_specs.rs`; `managed_worktree_tools.rs`). | **SEAT** when another harness owns the checkout; do not also create a rewake checkout or attach rewake land/finish ownership. **NATIVE / OPEN P6** if this harness owns one. Differences in naming, location or branch policy are not by themselves reasons to keep duplicate code. | Native creation/removal can be considered as owner, subject to local-mode limits and awareness of foreign workers. No native equivalent of rewake's whole land/finish contract was found; ordinary Git can handle landing when explicitly chosen. |
| Room view | `codex agents` browses sessions on the shared local app-server; schema lists threads and status. | TUI list/read/wait task tools and worktree browser (`tui/src/dynamic_tools.rs:157`; `worktree_browser.rs`). | **KEEP** the mixed room view: session names, roles, owing and waiting. **NATIVE** own-task navigation can coexist. A native process/thread list lacks the room's other harnesses and mail obligations. | A native agent overview can be the owner's own UI, not the source of truth for the mixed room. Do not read other threads' conversations to reconstruct reports. |
| Person UI, stage 6 | Native terminal, approvals and thread navigation; no plugin UI-module loader is advertised by C. | Status/title configuration (`config/src/types.rs:925–939`), hooks display and task UI. Plugin manifest has resources, not an in-process UI module (`plugin/src/manifest.rs`). | **NATIVE** terminal/approvals; **KEEP** a plain CLI view where needed. **SEAT** a room view in the other owner's pane through neutral data. No custom UI framework is needed for parity now. | No loadable pane/render hook equivalent was found. Keep CLI access without replacing the person's configured status line. |
| Headless workers | `exec --json`, `exec resume/fork`, output schema and final-output file; app-server supports stdio/Unix. | `exec/src/lib.rs:2313–2342` reads piped stdin to EOF: it is initial input, not a continuing prompt stream. | **NATIVE** one-shot headless runs; **OPEN P7** for a persistent headless worker on app-server, only if the owner chooses the fallback in ideas section 4. A parent holding stdin open is not sufficient. | Can run background threads and noninteractive processes; still needs rewake registration, mail and final-report accounting for a mixed room. |
| Extensions and hooks | Plugin CLI and 12 hook event names/trust states in S, including tool, compact, session, subagent, Stop and Interrupt events. | Plugins bundle skills/MCP/apps/hooks; Rust extension contributors are compiled in (`ext/extension-api/src/registry.rs`; `cli/src/main.rs:2062`). External-process hooks can supply context and Stop feedback; no runtime mod loader found. | **NATIVE** hooks where they remove adapter machinery and can be enabled for one run. **KEEP** app-server integration for idle wake, controls and tool registration; **OPEN P2** for hook trust and result semantics. Do not edit the person's hook/plugin configuration. | Its hooks belong to its own sessions. They cannot observe another harness's turns merely because it launched that process. |
| Native delegation, queue, remote/background service | `queue`, `agents`, `remote-control`, `cloud`; schema exposes durable queue operations. | TUI task tools address threads outside a subagent tree; the separate multi-agent tools and experimental message board are tree-scoped (`ext/agent-message-board/src/host.rs:16`). TUI task transport may be approval-gated MCP (`tui/src/dynamic_tools_mcp.rs`). | **NATIVE** operations within their scope; **KEEP** cross-harness mail. A remote cloud task does not supply the same local checkout. An existing native daemon could own execution, but rewake must not add an always-on broker or confuse daemon lifetime with membership lifetime; see the wrapperless-join assessment below. | Can own native background tasks, but neither those nor the message board supply the mixed room's kinds, proof of reading and one report per task. |

## Seating in another harness's worktree

The proposed division is viable without preserving rewake's entire worktree manager.
One owner creates, records, prepares, lands and removes the checkout. The worker
adapter receives a ready path and uses it. The following are requirements of that
handoff, not a proposal for another general lifecycle service.

1. **Actual cwd.** Pass the canonical checkout/subdirectory to the worker process,
   app-server/thread and rewake registration. Resume must be checked against its
   effective cwd; a launch flag alone can leave a resumed conversation elsewhere.
   Without this, edits or Git commands can reach the source checkout, while the
   owner's removal check watches the wrong path. C/S support the inputs; R
   `tui/src/app_server_session.rs` and D, September 26–27 on 0.155.1/0.157.1,
   establish why effective response values matter.
2. **Filesystem and Git.** Making the checkout writable is separate from allowing
   commits. Resolve both private Git metadata and common metadata through Git's
   layout, not a guessed directory name. D, `research-permissions.md`, September
   17 on 0.154.0: granting only the private gitdir failed object writes; only the
   common directory failed `index.lock` in a deeper managed layout; both worked.
   R `protocol/src/permissions.rs:2365` still protects a linked gitdir target.
   Without the Git grant the worker may edit successfully and fail at `git add`
   or commit. Under remote TUI, apply roots on the server: R
   `tui/src/startup_orchestration.rs:23,95` rejects local `--add-dir` and writable
   root overrides with `--remote`. The mod's own permissions do not grant the
   seated worker anything. P5 rechecks this on the pinned binary.
3. **Environment and configuration.** Build a fresh launch environment, replacing
   the parent's rewake identity with the worker's run/name/room and same-build
   executable. Do not pass the parent's mail socket/token, tool-call identity,
   conversation id, approval context or executor endpoint as the worker's own.
   Keep intentionally selected authentication/configuration and ordinary build
   environment; do not blanket-delete the user's environment or copy the owner's
   writable roots. R `core/src/exec_env.rs` describes thread/session/tool identity;
   current `internal/host/launch/environment.go` installs run identity, while
   `adapter/claude/claude.go` lists that owner's child markers. Explicitly review
   source and target markers for a mixed launch. R `worktree/src/git.rs:143`
   also clears inherited Git repository selectors before operating by cwd.
4. **Existing native resources.** Use the worker's intended configuration home;
   do not clone or edit the person's config, hooks or plugin stores. Select a
   deliberately chosen endpoint, not a shared daemon by accident. Preserve declared
   restrictions and distinguish its process lifetime from room membership. C
   provides per-invocation configuration and explicit remote selection; R
   `tui/src/lib.rs:1028` states endpoint selection. A fresh directory
   cannot fix a worker attached to the wrong server or executor.
5. **Owner awareness.** The owner needs the actual path and the worker's live
   lifetime before it may remove a clean checkout. Native ownership metadata is
   not occupancy: R `tui/src/worktree_browser.rs:1` says so explicitly, and its
   removal delegates to `WorktreeManager::remove`, with no foreign-session check.
   If the creator cannot veto removal, an explicit keep-until-workers-stop policy
   is a possible limited workflow; do not silently promise automatic safety.
   P6 must exercise the real owner's removal path. This does not require copying
   every old rewake removal guarantee before trying native ownership.
6. **Prepared contents and landing.** Accept the owner's native setup/copy policy.
   R `worktree/src/lib.rs` creates from a commit; D, September 27 on
   0.155.1/0.157.1, records no ignored-file copying. A missing local configuration
   or dependency can break the worker's build, but is preparation to document,
   not proof that rewake must own the checkout. A detached native checkout needs
   an explicit way to retain/land its commits before removal. Use native actions
   or ordinary Git; rewake's fast-forward-only land/finish and external-directory
   layout are not independently required by `docs/project.md`.

## What the lifecycle-owner mod supplies, and what it does not

M supplies a process-launch surface and an owner-side pane. It can launch the
worker adapter with a chosen cwd, display room data, and arrange native worktree
actions. It does not automatically become the worker's tool transport, sandbox,
turn boundary or conversation binding. Those remain the worker adapter's native
integration, which is what makes the same seating usable by other harnesses.

Three constraints matter for the owner's example (M, map entries cited above):

- **Creation is not a completed-path notification.** The creation event's input
  names a request and cwd. Its supplied worktree path is an override produced by
  a hook. Leaving the path unset lets native creation proceed. Therefore an
  observer must establish how it learns the completed native path; it cannot
  launch the worker by assuming that the event already carries it. P6.
- **A pane and output stream are not an interactive terminal.** The documented
  process launcher has finite initial stdin, then EOF, and no PTY/input/resize
  handle. The mod's pane is an owner-side interface. A full worker TUI needs a
  real terminal or a deliberately hosted terminal bridge, as `docs/project.md`
  permits for a session the person asks to host. Do not quietly replace the
  preferred full session with headless execution. P7.
- **Child lifetime follows the process stream/module.** Unloading the mod or
  ending the stream can terminate its child. A reload must not be assumed to
  preserve the worker. Environment overrides also do not constitute a clean
  environment. Let the launched adapter construct its own environment and report
  worker exit; P7 checks reload, owner exit and pending reports. If workers must
  outlive the owner, use an explicitly independent terminal launch rather than
  relying on this child handle.

The removal event's generic blocking result is a candidate, not evidence that
every native removal route awaits a veto before deleting files. The skill does
not settle that operational question; P6 does. No need to duplicate the skill's
API in the stage document.

## Joining without the launch wrapper

**Feasible to investigate; not proven by the reading.** A mandatory launch wrapper
is an implementation choice, not the only way to satisfy room membership. The
person could opt a normally launched session in through a trusted plugin/hook
configuration, or use an embedding client that already owns its app-server.
That is different from rewake attaching to arbitrary sessions behind the person's
back. The existing design's launch-only wording would need an explicit opted-in
join contract. No configuration was installed or changed for this audit.

| Entry route | What native loading gives | What it does not establish |
|---|---|---|
| SessionStart/Stop/Interrupt hooks, possibly packaged as a plugin | R `hooks/src/events/session_start.rs:45`, `stop.rs`, `interrupt.rs:25`, `session_end.rs:27`: event ids/context and callbacks; Stop can continue the turn. | The hook's `session_id` is a `ThreadId`, not a stable process/run identity. A short hook process cannot itself hold membership for the whole session. Full failed-turn coverage, abrupt death and `/new` rebinding remain unproved. Hooks alone give no app-server client, dynamic tools or idle wake; native queue is a candidate for the latter. |
| An MCP server already selected in the session's config/plugin | Native tool calls can reach an external helper independently of the shell sandbox; plugins can bundle that server (C; R `plugin/src/manifest.rs`). | MCP initialization alone does not prove the caller's rewake run/thread/turn or expose an end boundary. A companion hook/client must establish those bindings. Server scope, restart and final-result proof need checking. It must not become a permanent room daemon. |
| An app-server deliberately started by another owner, with a cooperating client | S gives native lifecycle, control, telemetry and dynamic tools without rewake creating the process. | The owner must expose a connection and explicit thread selection. Dynamic tools require the client to register/serve them; a plugin manifest alone does not do so. A second client must not guess the selected thread or race the terminal for server requests. Losing that connection is loss of capability, not necessarily death of the native thread. |
| The existing shared native daemon | C `agents`/`remote-control` and R endpoint selection show an existing owner of multiple threads. | Daemon pid/exit is not membership or worker liveness. This path changes the current private-server lifetime assumption; it cannot justify killing the daemon when a room member ends. Rewake still needs a session-scoped integration and explicit opt-in. |

Minimum useful join, independent of how it is started:

- A deliberate room/name/role claim, fresh run identity, compatible rewake build,
  actual cwd, and a live membership holder that releases or becomes detectably
  unavailable when that run ends. Do not substitute the pid of a transient hook
  or a shared server for this lifetime. The native owner may supply the holder;
  it need not be a process that launched the terminal.
- A bound thread and, for tools, call/turn ids tied to the run through a trusted
  channel. Establish and refresh them after new/resume, including the effective
  permissions and cwd. An id supplied by the model is not that binding.
- One functioning mail path plus idle notice delivery. Shell mail may suffice
  where readable; otherwise MCP or dynamic tools need proven binding and read
  completion. Hooks plus queue may be a smaller wake path than app-server control
  if their limitations fit the desired workflow.
- For a reporting worker, a proven end/confirmation path covering the outcomes
  the core requires. Without it the session cannot take tasks merely because it
  has an inbox tool. Compaction, mid-turn steering, telemetry and grants can be
  honestly absent capabilities until their native paths are connected.
- Every helper attributable to this joined run, with no user-config rewriting,
  transcript inspection or new always-on broker. A person-installed plugin is
  an opt-in mechanism; installing it is separate authorization, not this audit.

**OPEN P8:** manually start one opted-in disposable session with the proposed
hook/plugin/MCP integration, without `rewake codex`; establish the claim and ids,
idle delivery, one read and exactly one final report, then `/new`, resume,
helper disconnect and abrupt native-process death. Repeat with an explicitly
owned app-server if that is the smaller design. Verify which capabilities survive
without a proxy. This decides whether the launch wrapper can be optional; do not
restore all of its machinery merely because native registration uses another
entry point.

## Checks still OPEN

These are proposed probes for main to authorize/run, not runs made in this audit.
Start with a disposable endpoint/checkout and a stand-in response source where
possible; live model calls are needed only to establish behavior a stand-in cannot.

| Probe | Smallest useful observation | Decision it settles |
|---|---|---|
| P1 dynamic mail tools | Start with the real mail descriptors; handle one tool outside a denied state root. Compare the returned mail with the final input the model endpoint receives, including a post-tool replacement, output truncation, cancellation and disconnect. Resume that thread and switch to a new one. | Whether dynamic tools replace MCP; which event proves a read; registration/handler lifetime. S exposes ids, but writing a response is not a read receipt. R `dynamic.rs:240–273` emits completion after the response reaches core, before the handler returns its final output. |
| P2 end confirmation and hooks | Load a trusted hook for one run without editing user files; block a normal end once with unread mail, then allow it. Exercise failed/interrupted turns, missing trust and hook failure. | Which native surface implements confirmation and which ends it cannot cover. No extra user-config mutation to obtain it. |
| P3 wake/control | Idle delivery, busy delivery, interrupted turn, fresh thread, compact and interrupt on an exact selected id; check optional compaction instructions separately. | Native start/steer versus queue, without assuming that queued mail interrupts or that compact accepts a prompt. D's queue observations are older; do not adopt its ten-second latency as a current promise. |
| P4 conversation selection | Pinned CLI and the candidate stage-5 version: first launch, `/new`, id-only resume, failed resume and reconnect; observe requests/responses without reading saved transcripts. | A binding independent of the old TUI config marker, with correct cwd, tools and settings. |
| P5 seating and grants | Existing linked checkout; edit with cwd access; commit with both metadata roots; revoke for the next task; change during a busy turn; repeat after resume and in a read-only mail environment. | Effective permissions, the task boundary and sandbox-independent mail. A queued settings response alone is not acceptance. |
| P6 native worktree owner | Let the real owner create normally; obtain the finished path; seat a foreign worker; attempt removal while it is alive, including a clean checkout; then stop and remove. Cover the actual removal command, not only a synthetic event. | Path handoff and whether automatic removal can be vetoed; otherwise the limited explicit keep policy. Repeat in the opposite direction only when that owner is in scope. |
| P7 process/UI lifetime | Launch a harmless interactive terminal fixture through the proposed owner UI; exercise input, resize, owner exit and mod reload before spending a worker model call. Separately test a finite headless job or an app-server-based persistent worker if requested. | Whether full-session seating works and what child lifetime means. Holding `exec` stdin open is not a multi-turn protocol. |

## Corrections and design alignment

1. `docs/v2/design-codex.md` currently says a main on this harness is **not a
   current goal** and still lists MCP/tool transport as an open choice. The A1
   brief asks to examine the opposite direction and says dynamic tools are
   planned. This reading provides that comparison and recommends dynamic tools
   first; it does not silently change the release gate or turn a candidate into
   an accepted transport. Record the owner decision if either scope changes.
2. `ideas.md` section 5 remains a useful direction. Its shorthand that the mod
   sees creation and supplies the path needs P6's distinction between a creation
   request and a completed native path. Its Git-grant note should include both
   common and private Git metadata. Removal was already correctly left open.
3. The October 11 extension recon's broad inference that native messaging exists
   only inside a root's subagent tree is incomplete for R: TUI task tools can
   address existing threads. `send_message_to_thread` resumes the supplied id and
   starts input; it is not tree-path resolution. Availability depends on the
   connected server/TUI transport, and delegation tools may use approval-gated
   MCP. This does not establish installation on 0.160.1 or cross-harness mail.
4. The same recon's claim that resume cannot use dynamic tools is too broad:
   R restores saved definitions. Resume has no field for injecting new definitions
   in S; restoring definitions still needs a live client handler and current run
   binding. P1/P4 settle the usable path, not a blanket MCP fallback.
5. Native worktree tools in R go beyond the earlier `--worktree` reading, but
   remain local/trusted/feature-gated (`tui/src/app_server_session.rs:456–496`).
   Explicit remote startup still refuses `--worktree`
   (`tui/src/worktree_startup.rs:139`). Neither bare flag forwarding nor mere
   existence of a worktree tool solves the current remote-terminal path.

Recommendation: try native-owned worktrees with seating, native telemetry and
control, and dynamic tools before restoring old adapter machinery. Keep only the
neutral session/mail/authority obligations the project explicitly requires;
record native behavioral differences instead of rebuilding old conveniences by
default. The listed live checks remain open; no implementation permission is
implied by this reading.
