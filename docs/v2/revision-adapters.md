# Revision: the adapters and the mail tool

The harness contract, the Claude Code adapter against the mod, the Codex adapter and its
gateway, and the mail tool's endpoint and server. The overview is in
[revision.md](revision.md); figures are code / tests in lines. Facts about the mod are
from the stage 0 reconnaissance of Claude Code 2.1.287–2.1.289 (its documentation,
the type declarations 2.1.289 writes beside a mod, and live probes); they go into
`docs/research*.md` in stage 2 with where each was verified.

## The harness contract

`internal/harness` holds the contract (1 208 lines) and the mail tool's checks and
gates (1 018). The contract is already a required interface plus optional ones; 2.0
names the optional ones as the capabilities of the plan.

| File | Lines | Verdict | 2.0 |
|---|---:|---|---|
| `harness.go` | 57 | change: the global `registered` slice filled from `catalog`'s `init` (13–23) gives way to an explicit catalogue | `adapter` |
| `plan.go` | 182 | change: `Harness` keeps ID, Title, Summary, Examples, Notes, SingleUseFlags, ProtectedDirs, Launch; `Deliver` becomes the Wake capability; the Claude fields of `LaunchRequest`/`LaunchPlan` (`Socket`, `ObservationSocket`, `Observer`, `Lane`) and the Codex ones (`CodexHome`, `ToolLeftOut`) leave the shared types | `adapter` |
| `backend.go` | 166 | change: the optional interfaces map to capabilities, below | `adapter` |
| `flags.go`, `defaults.go`, `settings.go` | 132, 58, 225 | carry over: argument parsing, model and effort defaults, rewake's own settings file | `adapter` / `host/launch` |
| `notice.go`, `preview.go` | 126, 53 | carry over: the notice text and its preview, the same for every transport | `core/mail` |
| `environment.go` | 83 | change: `SessionEnv` to `host`; `CheckEnv` (50–83) serves only the name checks | `host`, `adapter/codex` |
| `thread.go`, `worktree.go` | 33, 33 | change / carry over: knowing the conversation; continuing in a worktree | `adapter` |
| `hooks.go` | 60 | change: the hook commands and their names go with the hidden commands; `ShellQuote` stays where used | `adapter/claude` |

How today's interfaces map to the plan's capabilities:

| Capability | Today |
|---|---|
| Launch | `Harness.Launch`, `LaunchRefuser`, `Resumer`, `WorktreeHarness`, `SingleUseFlags` |
| Wake | `Harness.Deliver`, `Backend.Deliver`, `ReservingBackend`, `Lane` |
| ToolTransport | `MailToolHarness`, `ToolWithdrawer`, `LaunchVersionReader` (Codex); the mod's registered tools (Claude) |
| TurnBoundary | `CompletionHandler`, `TurnReporter`, `SessionStartSource` |
| Telemetry | `Observer`, `ObservedBackend`, `ThreadTracker`, `ThreadSource` |
| Control | `Steerable`, `Accepting`, the control directory |
| Permissions | `DirGrantHarness`, `GitGrantHarness`, `GrantIssuer`, `HookGranter`, `ProtectedDirs` |
| PersonUI | none yet (decision 8) |

`internal/harness/catalog` (21): change. `catalog.go:17-20` registers both adapters in
`init`; in 2.0 `cmd/rewake` asks it for the catalogue and passes it on.

### Checks and gates

`mailtool.go` 104, `check.go` 301, `check_holder.go` 269, `check_diagnostic.go` 100,
`gates.go` 126, `gates_version.go` 118: Codex-only, to `adapter/codex`. They exist to
add a foreign MCP server to a person's harness without editing its configuration: run
the person's program under a bounded holder, prove the name `rewake` free, decide by
gate. With the Claude injection gone their only user is Codex
(`codex/mailtool_check.go:97`; `claude/mailtool_check.go:215` goes). The Claude shares
inside them go: `ClaudeVersion` (`gates_version.go:92-118`), gates G5 and G7 and
`outputBounds["claude"]`, `ToolName = "mcp__rewake__rewake"`, `ConfigFile`. The Claude
minimum (2.1.287) is a version check of its own in `adapter/claude`.

The gates are exact versions today: `closedGates` closes G1 for Codex 0.159.0 only
(`gates.go:37`), so the tool is added on that one version. Codex has no floor constant:
`server_version.go:31` pins a launch note to 0.155.1. 2.0 needs a minimum per adapter;
whether gates become ranges is stage 2 (proposal 8).

## Claude Code

`internal/harness/claude`: 2 373 Go + `plugin.js` 337 / 3 523.

| File | Lines | Verdict | Why |
|---|---:|---|---|
| `claude.go` | 312 | change | flags, defaults, `--append-system-prompt`, `--allowedTools`, `--add-dir` stay; the socket flag (25–27, 168–185), observer, collector, lane (195–219), settings hooks (224) and tool injection (229–238) go |
| `resume.go`, `worktree.go`, `protected.go` | 40, 122, 24 | carry over | resume and grants by `--add-dir`; worktree refusals; protected directories |
| `settings.go` | 229 | drop | one `--settings` layer with the Stop, observe and grant hooks and the status line, merged with the caller's; the mod replaces the hooks; kept only if command grant hooks survive stage 6 |
| `statusline.go` | 54 | drop (superfluous) | finding the person's status line for the tap |
| `lane.go`, `receipt.go`, `socket.go` | 375, 158, 112 | decided by probe | the delivery path, below |
| `replysweep.go` | 49 | drop | cleans reply sockets of dead wrappers; goes with them in every outcome but the native socket |
| `lane_interrupt.go`, `notice.go` | 29, 51 | change | "main interrupted your turn" in the next notice is needed on any transport; the `<task-notification>` wrapper only on a socket |
| `mailtool.go`, `mailtool_check.go`, `managed.go` | 85, 273, 38 | drop (MCP injection) | `--mcp-config`, the name check, the managed-MCP refusal; 8 lines of `managed.go` (the managed directory) serve the mod's load diagnosis |
| `permission.go` | 276 | change | the decision from the grant journal is pure logic; stage 6 decides whether the mod answers `classic.PermissionRequest` with it |
| `plugin.go` | 146 | change | writes the per-launch plugin directory and passes `--plugin-dir`; 2.0 drops `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS` (ignored from 2.1.287) and diagnoses `--bare`, `--safe-mode`, `disableAllHooks`, `allowManagedModsOnly` |
| `plugin.js` | 337 | rewrite as mod | becomes the one mod |

### Claude Code, mechanism by mechanism

| Mechanism | Today | 2.0 |
|---|---|---|
| launch flags, resume, worktree, `--add-dir` | `claude.go`, `resume.go`, `worktree.go` | carry over |
| one `--settings` layer of hooks and status line | `settings.go`, `statusline.go` | drop |
| command hooks `rewake turn-ended`, `rewake observe` | `settings.go:79-97`, `cli/turnended.go`, `cli/telemetry.go` | rewrite as mod: `turn.start`, `turn.complete` with `reason` (answer, aborted, refusal, error), `session.end` |
| plugin module for Esc, Ctrl+C and context | `plugin.js`, `plugin.go` | rewrite as mod: the same module grows into the mod |
| mail tool by MCP injection | `mailtool*.go`, `managed.go`, `harness/mailtool.go` | drop; the mod registers the tools with `$.tool.register` at `session.start` and answers in `tool.call`; a call a hook answers itself bypasses permission rules and dialogs, so the mod decides who may call |
| directory grants | `permission.go`, `cli/granthook.go`, `grantauth/keeper.go` | change, stage 6: `classic.PermissionRequest` and `classic.PreToolUse` in the mod instead of a process per tool call |
| compact | `plugin.js:94` `$.session.compact` | rewrite as mod, same call |
| interrupt | `plugin.js:131` `$.turn.abort` | rewrite as mod, same call; `InterruptTrace` stays |
| clear | none in the plugin | `session.end` with `reason: clear` and no new `session.start`; whether registered tools survive `/clear` is for the probe |
| telemetry | `telemetry/*` | rewrite as mod: `session.measure`, `session.usage` |
| the person's status line | `statusline.go`, `telemetry/{tap,statusline}.go` | drop; the person's UI is stage 8 |

### Delivery, decided by probe

`socket.go` writes a user line with `priority: next` to `--messaging-socket-path`;
`lane.go` proxies the socket's lines, reads `peer_message_status` receipts within a
300 ms window and holds notices up to 3 s until the status line is drawn
(`lane.go:35-47`); `receipt.go` parses the receipts. Three candidates: the mod's
`$.prompt.submit` (waits for idle, then starts a turn); today's socket; the
harness's native messaging socket, which the mod also sees as `session.receive`.

Independent of the outcome: the notice text (`harness/notice.go`), the delivery
outcomes of `inbox.Result`, the Wake capability, the interrupt trace, availability and
departure, and the delivery loop in `host`. With `$.prompt.submit` the socket, lane,
receipt parsing, reply sweep, `Drawn` and the opening limit all go (~760 lines); with
today's socket four files stay (~360); with the native socket the receipt logic stays
but reads mod events.

Questions for the probe: does `crossSessionInbound` apply to `$.prompt.submit`; does a
busy session queue it rather than steer; does it fire from a timer in an interactive
session; what the person sees (the plugin's frame against "● …"); is a submit lost on
`/clear`; is there still a window before the interface is drawn.

### Telemetry

`internal/harness/claude/telemetry`: 1 778 / 1 673. Short-lived `rewake observe` and
`status-tap` processes send datagrams to the wrapper; the collector folds them into a
`sessionstate` snapshot.

| Files | Lines | Verdict |
|---|---:|---|
| `event.go`, `plugin.go`, `send.go` | 323 | rewrite as mod: the wire from the mod to the wrapper; `HookEvents` and the status-line kind go |
| `collector.go`, `collector_turns.go`, `state.go`, `turnstart.go`, `clock.go` | 795 | rewrite as mod (the Go receiving side): the fold and the stopped-turn publication stay; `Drawn` and `LaunchedWith(--autocompact)` go; the turn start becomes a core record written from `turn.start` |
| `decode.go`, `statusline.go`, `tap.go` | 417 | drop |
| `window.go` | 243 | drop if the probe shows `session.usage` plus the environment give the auto-compact window; otherwise its environment-and-settings half moves into the mod |

## Codex

`internal/harness/codex` 3 884 / 6 300 and `codex/gateway` 5 346 / 7 584: Codex-only.
No Claude code in either, apart from one comment (`gateway/steer.go:15`).

| Part | Lines | Verdict |
|---|---:|---|
| `codex.go` | 399 | change: split along capabilities; the empty `Deliver` (296) goes; comments about 0.155.1 (55, 145, 180–185) are rewritten |
| `server*.go` (app-server session, events, delivery, config) | 750 | carry over |
| `server_version.go` | 204 | change: the 0.155.1 pin and `transportNote` (31–36) give way to a minimum version |
| `server_steer.go` | 98 | carry over: serves the control directory |
| grants and git (`server_dirgrant*.go`, `server_gitwrite.go`, `gitwrite.go`, `gitmetadata.go`, `landlock.go`, `configtext.go`) | 984 | carry over (`use_legacy_landlock` is a Codex feature name, not a legacy mark) |
| `continuation.go`, `launch_refusal.go`, `notice.go`, `mailbox_brief.go`, `rpc.go`, `websocket.go` | 569 | carry over |
| `mailtool.go` | 89 | carry over: the `-c mcp_servers.rewake.*` leaves |
| `mailtool_check.go`, `mailtool_inject.go`, `mailtool_layers.go`, `server_mailtool.go` | 791 | change: G2 lands here |
| gateway: core, connections, events, websocket | 1 139 | carry over; `connections.go:147` loses its legacy branch |
| gateway: state, intent, metadata, fork, selection, publication | 1 125 | change in `state.go`, `metadata.go`, `metadata_validation.go`, `fork.go`: the 0.155.1 recognition goes |
| gateway: delivery, reservation, admission, operations | 1 258 | carry over |
| gateway: observe, activity, proof, mark | 746 | carry over |
| gateway: telemetry | 669 | carry over |
| gateway: `steer.go`, `bridge.go` | 409 | carry over |

**Legacy marks.** All nine `legacy(` marks in these packages are `codex <0.157.1`, all in
the gateway: `state.go:96,99`, `connections.go:147`, `fork.go:19`, `metadata.go:177`,
`metadata_validation.go:36`, and in tests `state_test.go:30`, `tui_paths_test.go:42,48`.
They go with about 8 lines of code, the `roots`/`permissions` fields of the metadata,
the 0.155.1 start form in 28 test files (69 uses of `runtimeWorkspaceRoots`) and the
131 lines of `testdata/tui-paths/0.155.1-*.jsonl`.

**Where G2 lands.** A new file reads, without network, the plugin cache under
`CODEX_HOME` and each manifest's `mcpServers` or `.mcp.json`, and refuses from a closed
list of words like the other refusals (`mailtool_inject.go:41-52`). It is called in
`CheckMailTool` between `takenWhere` and `requiresMCP` (`mailtool_check.go:51-54`),
before the claim; and in `toolInjection.judge` (`mailtool_inject.go:179`), where the
comment at 148–149 already names step 3 as G2's — so the same check runs at start and
on every thread. `thread/start` with `selectedCapabilityRoots` is refused or checked in
`stepZero` (`mailtool_inject.go:134`).

## The mail tool: bridge, endpoint and server

| Files | Lines | Verdict | 2.0 |
|---|---:|---|---|
| `bridge/bridge.go` | 214 | change: ticket, `CallKey`, `Digest`, `Exposure`, `EndGate` are neutral; `ClaudeTransport` (88) goes | `tool` |
| `bridge/parts.go` | 69 | carry over: cutting an answer into parts under the cap | `tool` |
| `endpoint/endpoint.go`, `gate.go`, `channel.go` | 606 | carry over: the socket, hello, peer credentials, the end gate | `tool` |
| `endpoint/calls.go`, `client.go`, `config.go`, `protocol.go`, `events.go` | 965 | change: the Claude halves go — `promptSeen` and prompt limits, `Observe` and the `hook` role, `claudeHook` (`events.go:92-141`), `LimitsProven` | `tool` |
| `endpoint/limits.go` | 119 | drop (MCP injection): hook limits for Claude's MCP output | — |
| `bridge/server/*` | 802 | Codex-only: the MCP stdio server, frames, encoder, child | `tool/mcp` |

The split follows a dependency that already holds: `server` imports `endpoint`, never
the reverse. `tool/mcp` alone knows MCP and stdio; its 3 523 lines of tests (a rig that
builds the binary, the fault seam) go with it.

**Decision 9 changes the server.** Today it lists one tool, `rewake`, taking
`{"words": [...]}` (`bridge/server/server.go:224-234`). 2.0 lists the set — inbox, send,
pending and the rest — each with its own schema, the same set the mod registers. The
schemas belong to `tool`, so both transports read them from one place; `cli.ToolWords`
turns into that mapping.

**Open for stage 2: the mechanism, not the guarantees.** Whether the mod's tools reach
the wrapper endpoint (ticket, confirmation, end gate, as on Codex) or something else is
stage 2's choice; the endpoint code takes the first as it is, with a transport value for
the mod. Either way the mod keeps rules 2, 6, 7 and 8 of `mail-bridge-server.md` as
guarantees: a trusted binding to call, conversation and turn; a read only on proof of
its own whole result within the effective limits; no commit past its turn's end. A
reading tool without that proof refuses before its effect, and a `rewake` child process
that marks read on printing is not a way round it
([rules](revision-rules.md#mail-bridge-servermd-rules-111)).

## Proposals

Proposals for the owner, not decided:

1. Drop `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS` from the launch (`plugin.go:35-40,76`);
   diagnose what keeps mods off instead.
2. Drop the status-line tap and everything behind it: `session.usage` and
   `session.measure` give context, window, rate limits and cost, and the managed-policy
   class "status line overridden" disappears.
3. Replace the 250 ms poll of the control directory (`plugin.js:198`) with a child the
   mod spawns (`$.process.spawn`) that streams requests, keeping the files as the core
   transport.
4. Drop `telemetry/window.go`, the port of Claude Code's auto-compact window parsing, if
   the probe allows.
5. Drop `turnstart.go`'s file marks on the boot clock: `turn.start` carries `turnId`.
6. Keep one WebSocket client for Codex: `codex/websocket.go` (232) and
   `gateway/websocket*.go` (310) implement the same client with different limits.
7. Merge the two passes over Codex metadata (`gateway/metadata.go`,
   `metadata_validation.go`, `read_context.go`) once the 0.155.1 fields are gone.
