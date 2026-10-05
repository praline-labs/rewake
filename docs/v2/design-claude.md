# Design: the Claude Code adapter

Everything on the Claude Code side goes through one official mod, from Claude Code
2.1.287 on (decision 1). The mod is JavaScript the wrapper writes for one run; the Go
side of the adapter launches the harness, receives the mod's calls at the host's
endpoint, and answers them through the core. The overview is in [design.md](design.md);
the facts are from `.scratch/v2-recon/claude-mods.md` (cited `mods:N`) and the 2.1.289
types (`types:N`), marked as in the overview.

## Offered capabilities

| Capability | How | Proven | Stage 4 probe |
|---|---|---|---|
| Launch | `claude` with one `--plugin-dir` | **[live]** loads in `-p` (`mods:186`) | interactive first launch in an untrusted directory |
| Wake | one of three candidates | — | P1 decides |
| ToolTransport | `$.tool.register` + `tool.call` answered by the mod | **[live]** (`mods:23-62`) | P2, P4, P8 |
| TurnBoundary | `turn.start`, `classic.Stop` with `block`, `turn.complete` | **[types]** block; **[live]** order (`mods:115`) | P3 |
| Telemetry | `session.measure`, `$.session.usage()` | **[types]**, **[live]** snapshot (`mods:142`) | P6 |
| Control | `$.session.compact`, `$.turn.abort`; clear unknown | **[types]**; 1.x uses both (`plugin.js:198`) | P7 |
| Permissions | stage 6: `classic.PermissionRequest`, `classic.PreToolUse` | **[types]** | stage 6 |
| PersonUI | stage 6: `$.ui.status`, panes | **[types]** | stage 6 |

## Loading the mod for one run

The wrapper writes the mod into the run's directory — `.claude-plugin/plugin.json` with
`name: "rewake"`, `hooks/hooks.json` naming one module, the module — and passes
`--plugin-dir <dir>` to that launch only (`mods:183-186`). Nothing in the person's
settings changes (L1); the directory goes with the run (L3) and is one of the adapter's
protected directories, so no grant reaches it. `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS`
leaves the launch: 2.1.287 and later ignore it **[doc]** (`mods:216`).

The module keeps to the static-analysis rules `claude plugin validate` reads — every
call written `$.noun.verb(...)`, event names as literals, relative imports only
(`mods:190`) — and a test runs the validator on the generated module as the 1.x plugin
host test does (`harness/claude/plugin_control_host_test.go`).

The plugin name gives the tools their prefix: `mcp__rewake__<name>`, which no mod can
avoid (`.scratch/v2-recon/mods-tool-naming.md`). A person's own MCP server named
`rewake` would show the same prefix; whether the two collide is P8.

## When the mod does not load

Known causes **[live]** (`mods:194-201`): `--safe-mode`, `--bare`, `disableAllHooks`,
managed `allowManagedModsOnly` or `allowManagedHooksOnly`, `disableSideloadFlags`
(which refuses `--plugin-dir`), an untrusted directory in an interactive session, and a
remote switch whose reach to `--plugin-dir` is unknown. rewake finds out in two ways:

1. **Before the launch**, from what it can read without editing: the person's own flags
   (`--bare`, `--safe-mode`) and the settings files Claude Code reads for those keys.
   A known cause is named in the refusal or note (L2: where, never what).
2. **At the start**, by the mod's hello: `session.start` is awaited before the first
   prompt (`types:4176`), and the mod's first act there is a hello to the endpoint. No
   hello is not proof of failure (C3); the host says what it has not observed — "the
   rewake mod has not reported in; mail runs through the shell" — once, to the person
   and in the run's record.

Until a hello arrives, ToolTransport, TurnBoundary, Telemetry and Control are not live
([design-api.md](design-api.md#two-levels-offered-and-live)). For a role that owes
reports the owner's answer 3 applies ([design.md](design.md#the-owners-answers)): a
known cause refuses the launch; a session running without its mod takes no tasks. A mod
that loaded and is later lost — unloaded after crashing the hooks worker (`mods:204`),
its stream child gone — takes its capabilities out of the live set when the host sees
the loss; P5 includes that case.

## The mail tools

At `session.start` the mod registers each tool of the set — name, the plain description
and the input schema, all from `tool` ([design-api.md](design-api.md#tooltransport)) —
before the first prompt (`types:2940, 4176`). Registration outlives `/clear`, which
does not run `session.start` again (`mods:32`, unconfirmed: P9). Whether the tools load
deferred, behind a tool search, is P8 (`tool.describe` with `isDeferred: false`,
`mods:37`).

**Permissions.** A call the mod answers itself in `tool.call` skips the permission check
and its dialog **[doc, live]** (`mods:56-57`), so a launch needs no allowance for the
tools (1.x needed one, `mail-bridge-launch.md` rule 3). But a person's `permissions.deny`
on an `mcp__rewake__` tool would then be routed around, which C7 forbids. So before
answering, the mod asks `$.tool.check({tool, input})` (`types:2929`): `deny` returns the
person's refusal to the model as a denial; `allow` and `ask` go on, since rewake's own
tools are what the launch is for. A refusal — the person's deny, or a check that cannot
be relied on — runs no effect and **gives no advice to repeat the operation in the
shell**, where that deny does not reach (C7, `mail-bridge-channel.md` rule 7): it names
the refusal or the check that was unavailable, and the host tells main and the person
once. Whether `$.tool.check` reports a deny rule for a mod-registered tool is P8; on a
version where it cannot, the mod registers no tools — ToolTransport is not live and the
session is briefed as without it — rather than run tools under a deny it cannot see. A
transport fault or a missing read proof under a check that answered `allow` keeps the
ordinary fallback (T6, T11).

### How a tool call runs

Decided: **through the host's endpoint, not by a `rewake` process the mod starts.**

1. `tool.call` for `mcp__rewake__<name>` fires in the mod with the model's arguments and
   the reserved fields `tool` and `tool_use_id`, which no rewrite can change
   (`types:12095-12102`); a call from a subagent carries `agentId` and is refused —
   subagents use the shell.
2. The mod sends the call to the endpoint with `$.http.fetch` over the run's Unix socket
   (`socketPath`, `types:3377-3386`): the tool, the arguments, `tool_use_id`,
   `$.session.id()`, and the turn the call belongs to. `tool.call` carries no turn id
   (`types:196-206, 12093-12103`), so the turn is the one whose `turn.start` the mod
   has seen and whose `turn.complete` it has not; a call outside an open turn has no
   proven turn, and its pending mark or read is unproven (T7). That the mod sees a
   main-loop `tool.call` only between its turn's start and complete, and that turn ids
   are never reused, are P3 and P9; until P9 shows the latter, only an operation's
   first attempt may mark (T7). Time inside a `$` call is outside the hook's 10 s
   budget (`mods:81, 159`).
3. The host checks that the request came from the mod (below), makes the binding (T2)
   from those harness-supplied ids and the digest of the arguments, and runs the
   operation in a CLI child of **its own image** under the receipts (T3, T4, T10). The
   answer passes the one encoder and bound (T5).
4. The mod returns `{ result }` — the answer's text. A refusal returns as text too: the
   mod cannot set the error flag (`mods:48`).
5. **A read counts only on proof of the final result** (T6): what the model is given
   for that `tool_use_id` after every replacement and truncation, not what the tool
   returned. An event that reports the result is not yet that proof: a `PostToolUse`
   hook may replace what the model sees (`updatedToolOutput`, `updatedMCPToolOutput`,
   `types:1256-1261`), and the chain runs managed settings hooks, then the modules,
   then the other settings hooks (`types:1172`), so a person's hook can replace a
   result after the mod has seen it; a large result becomes a preview at a size not yet
   measured. The candidates are `classic.PostToolUse` and `classic.PostToolBatch`,
   the latter fired once per batch before the next model request with each call's
   `tool_response` (`types:7505-7545`). **A candidate is accepted only if P2 shows it
   carries the final result** — after another hook's replacement, after a large
   result's preview — compared with what a stand-in model API actually received
   (`tools/standin`), never with a transcript. Accepted, the host acknowledges the parts
   when the final result equals the recorded answer and arrived before the turn's end
   (T7, T8). **Without such a signal the reading tools refuse before their first
   effect** and say to read with `rewake inbox`; the other tools work.

Why not a child the mod starts: such a child is a process below the harness exactly like
a command the model runs in Bash, with the same environment (`mods:76`), so the host
could not tell an acknowledgment from the mod from one the model typed; and after an
npm upgrade the path the mod would run names a new build (T10). Through the endpoint the
answer is computed by the wrapper's own build and the acknowledgment comes from evidence
only the harness emits.

**Telling the mod from a shell command.** Every request to the endpoint is checked on
its own: the peer of its connection, by `SO_PEERCRED`, must be **the harness process
itself** — the pid the wrapper started, alive with its start time before and after the
answer, as the grant authority checks its peer (`docs/grants-authority.md`, steps 1–2).
A descendant of the harness is never accepted, since a command the model runs is one.
No secret, token or session carries authority from one request to the next, so a mod
reload, a respawned hooks worker or a dropped connection changes nothing: each request
proves its origin again. This holds only if `$.http.fetch` connects from the harness's
own process: P4. **If it does not, there is no fallback**, and more than the tools goes:
every capability that reaches the host through the mod — ToolTransport, TurnBoundary,
Telemetry, Control — is not live on that version, since its hello, turn events and
answers are refused alike. A role that must report then refuses to launch (answer 3 of
[design.md](design.md#the-owners-answers)); the shell carries mail, not a turn's end.
So a negative P4 blocks stage 4's working pair until another trusted channel of events
is designed and reviewed; admitting a helper process back is not that channel. A secret the
host hands the mod is not one: after the first load a shell command may already run, and
on a reload (`types:4172-4178`: an enable, a worker respawn) it could read and present a
reissued secret first. The threat is the one 1.x tickets answer — a model's command
imitating the tool; a process of the same user rewriting the mod's files is outside it.

The same holds for the endpoint's other callers in the mod — the hello, turn events,
read evidence, control answers. The stream child of [Control](#control) is a descendant,
so it carries no authority: it only signals that something is waiting, and the mod
fetches the request and posts the answer itself.

## The turn boundary

| Event | The mod does | The host does |
|---|---|---|
| `turn.start` (`turnId`) | remembers `turnId`; tells the host | records the start in the core (replaces the boot-clock files of `obs.turn/`) |
| `classic.Stop` | asks the host whether this end may close | runs the confirmation check ([design-rules.md](design-rules.md#turn-outcomes)); the mod returns `{ block: reason }` when it says go on |
| `classic.StopFailure` | tells the host | marks the turn failed; the report waits for the same turn's `turn.complete`, so one E7 event publishes once |
| `turn.complete` (`turnId`, `reason`, `isAborted`, `answer`) | tells the host and waits for its receipt | the turn end operation (E7), named by run and `turnId` |

Events with `agentId` are a subagent's and are ignored (`mods:126`). An interrupted turn
gives `turn.complete` with `reason: "aborted"` and no Stop (`mods:114`; `plugin.js:1-4`).

The confirmation moves from the settings Stop hook to `classic.Stop`: it fires where the
hook would, without one configured (`types:1161`), and may answer `block` with a reason
(`types:1205, 1285, 1313`); `stop_hook_active` tells a repeated stop (`types:11565`).
The types allow it; that the chain works live is P3.

## Telemetry

`session.measure` after each turn and on a change of limits, with `context`,
`rateLimits`, `cost` and the units changed (`mods:122`); `$.session.usage()` on start
(`mods:132-142`). The mod forwards both; the adapter folds them into `core/session`.
The person's status line is not wrapped (answer 4): the tap and its parser go.
`context.window` is the model's window, not the auto-compact threshold (`mods:142`);
whether `percent` is measured against the threshold is P6, and the 1.x parser
(`telemetry/window.go`) is not ported if the usage suffices (answer 5).

## Control

| Verb | How | Known limit |
|---|---|---|
| compact | `$.session.compact({ instructions })` | the host refuses during a turn (`plugin.js`) |
| interrupt | `$.turn.abort({ turnId })` | the running turn's id only |
| clear | no call in the 2.1.289 types | P7: whether `$.prompt.submit` runs `/clear`; if not, Control here offers no clear |

Requests reach the mod by a stream, not by polling (answer 6): the mod starts one child
with `$.process.spawn` at `session.start` (`types:3410-3440`), which holds a connection
to the endpoint and prints a line whenever something waits. The line is a hint only — no
request, no content, no authority: the child is a descendant of the harness, which a
model's command can be too ([the mail tools](#how-a-tool-call-runs)). On a hint the mod
fetches the request itself and answers through the endpoint, each request checked on its
own. A lost hint costs time, not a request: the mod also asks at each turn event and on
a slow timer. The request files of `core/control` stay the transport between sessions.
The child is started from the wrapper's own image, `/proc/<wrapper pid>/exe`, so it is
the wrapper's build even after the file at the install path is replaced (T10). Its
lifetime across turns and reloads is P10.

## Wake and delivery

Three candidates (decision 6), decided by P1:

| Candidate | What it is | Known |
|---|---|---|
| `$.prompt.submit` | the mod starts a turn with the notice | waits until idle and starts a turn; no steering mid-turn; the model sees "The rewake plugin sent a message" unless `asUser` (`mods:87-94`) |
| the 1.x line | a line on the session's inbound socket, at a path rewake fixes with the hidden `--messaging-socket-path` (`harness/claude/claude.go:33`), with the inbound gate's receipts | built and tested in 1.x (`docs/delivery-adapters.md`); steers between tool calls |
| the documented messaging | the same inbound socket as documented: `CLAUDE_CODE_MESSAGING_SOCKET` and `CLAUDE_CODE_MESSAGING_TOKEN`; the mod sees `session.receive` and may consume it (`mods:99`) | documented, not tried by rewake; whether it differs from the 1.x line beyond the token is part of P1 |

The notice could reach the mod by the same stream as control; the host keeps deciding
what to send and when ([design-api.md](design-api.md#wake)). The fate of the 1.x lane,
`inbox/held.go` and its receipts follows P1 ([revision-core.md](revision-core.md#the-delivery-server-and-heldgo)).

## /clear and resume

`/clear` ends the conversation with `session.end` (`reason: "clear"`) and continues the
process under a new session id, without `session.start`; resume and branch give
`reason: "resume"` (`mods:111, 118`). The mod tells the host each change of session id,
and the run's conversation binding moves with it, so a call bound to the old
conversation cannot commit into the new one (T2, T7).

## What the mod never does

It reads no transcript and no prompt text (`turn.start`'s `text` is not forwarded); it
forwards the end's `answer`, which is the report, as 1.x forwards the Stop hook's last
message. It replaces no status line, sends no settings, and does no heavy work in a
hook: the hooks worker is shared and a mod that stalls it is unloaded (`mods:204`).

## The stage 4 probe

Main runs it live on the installed Claude Code, before the adapter is built; each answer
goes into the research documents with its version.

| # | Question | Decides |
|---|---|---|
| P1 | Wake: each candidate idle and busy, under `-p` and interactive, after `/clear` and a compaction; steering; how the person and the model see it; identical-text suppression; a held line's outcome | the Wake mechanism, and the fate of the lane and `held.go` |
| P2 | Does `classic.PostToolUse` or `classic.PostToolBatch` carry the **final** result for a mod-answered call: with a settings hook that replaces a short result after the mod; with a large result turned into a preview; with the event or the host's answer lost; with the turn aborted between the result and the next request. Each case compared with what a stand-in model API received | whether reading tools are live, and the effective bound (T6); the observed order goes into the fixture |
| P3 | `classic.Stop` with `block`: the turn continues; a second Stop with `stop_hook_active`; one `turn.complete` at the end; an aborted turn; `StopFailure` then `turn.complete` as one end; does a main-loop `tool.call` ever fall outside its turn's start and complete | the confirmation, and the turn a call belongs to |
| P4 | Does `$.http.fetch` with `socketPath` connect from the harness process itself, by `SO_PEERCRED`, on every request; the same after a reload, a worker respawn, a repeated hello, a repeated native call; a shell child started before the reload tries the endpoint and is refused | whether any capability that reaches the host through the mod — ToolTransport, TurnBoundary, Telemetry, Control — can be live; negative, reporting roles refuse to launch and stage 4's pair is blocked |
| P5 | What each known cause shows when the mod does not load, interactive and `-p`; does the remote switch reach `--plugin-dir`; what the host sees when a loaded mod is unloaded later or its stream child dies | the preflight, the notice, the withdrawal of a lost capability |
| P6 | Cadence of `session.measure`; is `context.percent` against the auto-compact threshold | whether the window parser goes |
| P7 | `$.session.compact` idle and mid-turn; `$.turn.abort`; can a mod clear the conversation | Control's verbs |
| P8 | Reserved fields under forged arguments; deferred loading; a `permissions.deny` rule and `$.tool.check` on a mod tool, with neither an effect nor shell advice after a deny; a person's MCP server named `rewake` | the tool set's safety; with no reliable check, no tools |
| P9 | Tools after `/clear`; session ids across `/clear`, resume, branch; are turn ids ever reused across an interrupt, a reload, `/clear`, a resume | the conversation binding; whether a retry may mark (T7) |
| P10 | A `$.process.spawn` child across turns, reloads and `session.end` | the control and notice stream |
