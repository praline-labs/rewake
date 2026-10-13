# Stage 4 audit: the rows of the session and the mail

Rows 1–12 of [the verdict table](stage4-audit.md#the-verdicts) argued — joining, launch, the
mail and its tools, the turn, wake, control, binding and grants; rows 13–22 — the worktree
and the tabs the space needs, the person's UI and the features that stay rewake's — are in
[stage4-audit-rows-space.md](stage4-audit-rows-space.md). Each row: why the verdict,
what code and documents it removes or keeps, what follows for the other harnesses, and
which property of [project.md](../project.md) it touches. Facts carry the marks set in
[the audit's opening](stage4-audit.md): *help*, *skill*, *docs*, *rewake*.

**The other harnesses.** By the owner's decision of October 13, 2026 the next harness after
Claude Code is pi, and Codex comes after it; each row says what pi offers first, then
Codex. Two readings made that day are folded in, neither run live:

- *pi reading* ([stage4-reading-pi.md](stage4-reading-pi.md)): pi's documentation and
  source at 0.85.1 (a reference tree at `05c6229`) and the documentation and changelog of
  the npm package 1.2.0, the version installed since that day; *help* on pi means
  `pi --help` of 0.85.1. Its probe items are cited PI1–PI14 in its numbering.
- *Codex reading* ([stage4-reading-codex.md](stage4-reading-codex.md)): a review session's
  reading of codex-cli 0.160.1 by help, its app-server schema generated that day, and a
  reference tree at `8c0f701` read as source. Its probes are cited CX1–CX8 in its
  numbering. Codex is read here for later: by the same day's
  decision, only its entry point is built before 2.0.

## 1. Joining: wrapper or mod — OPEN, P11

**Why.** Argued in [the central question](stage4-audit.md#the-central-question-claude-code-to-claude-code-through-the-mod-alone):
the mod can carry every job the wrapper does, but not the wrapper's authority. The anchor
needs a process no model command can precede, which holds only if a process the mod starts
at the first `session.start` can detach and outlive reloads (*skill*: `$.process.spawn`
children die with the module; `$.process.run` is one-shot with a timeout). And the
decision's proof rests today on one process being both the anchor a caller descends from and
the final writer it talks to; under the join they split, and the caller-to-final-writer
proof, a late opt-in and the anchor's death during an admitted decision are obligations no
contract answers yet. P11 settles the first; the second needs a replacement authority
contract, and without one the wrapper stays.

**Code.** If the mod's join is taken: `internal/host/launch` keeps the launch of a harness
for the wrappers that remain; the run in `internal/core/registry` names an ancestry anchor
(the harness process) apart from its final writer (the detached process), and the
authority of [stage3-decision.md](stage3-decision.md) is restated for that split under a
contract still to be written; `internal/adapter/claude/claude.go` shrinks to a launcher that adds
`--plugin-dir` and the join variables. If not: nothing changes beyond the mod.

**Other harnesses.** pi loads in-process TypeScript extensions per run with `-e <path>`,
with no configuration edited, and installs them with `pi install` (*help*), so the same
choice exists for it: a wrapper, or a person's own `pi` with the extension that joins.
The extension runs in the pi process with full Node access, so its pid is the harness's
and a wrapper that spawns `pi` knows it; whether the node launcher re-executes is PI10
(*pi reading*). Codex has no in-process
module: *Codex reading* finds a join without the wrapper "feasible to investigate; not
proven" through hooks, an MCP server or a cooperating app-server client, none of which
alone holds membership for the session's life (CX8). A mixed room therefore holds runs
anchored two ways, which the core sees as fields of a run.

**Property.** No service beside the sessions; the person's configuration never edited by
rewake; no intervention in a session the person started. Each holds under the conditions
the central question names.

## 2. Launch: name, room, briefing, plugin, defaults — SPLIT, P13, P14

**Why.** The name, the room and the role are rewake's registry and stay. Claude Code has
its own display name, `-n/--name`, shown in the prompt box, `/resume` and the title
(*help*), which native `SendMessage` also addresses by (*docs*): passing rewake's name
there makes the session findable by both. The briefing today goes through
`--append-system-prompt`; with `--system-prompt-snapshot` on by default, a resume reuses
the prompt written on the first request even if the new launch passes another text, until
a compaction (*help*). A briefing changed between launches — a role changed on resume —
is then not seen. `prompt.section` and `prompt.context` (*skill*) are candidates, both
cached: a section's answer until the mod invalidates it, the context once per conversation
until it is invalidated or re-read. Whether either brings a changed briefing into a resumed
conversation, and with which invalidation, is P14.

**Code.** Kept: `internal/host/launch` registration, `internal/adapter/settings.go` (rewake's
own launch defaults, not Claude Code's settings). Added: `--name`. Moved, if P14 shows the
snapshot bites: the briefing from `claude.go`'s flag to the mod's prompt event.

**Other harnesses.** pi has `--name`, `--append-system-prompt` (repeatable, per run, not
stored as a message) and `--model` with a thinking suffix or `--thinking` (*help*): the
same split. `--session-id` lets rewake name the conversation before the first turn
(*pi reading*), as Claude Code's own `--session-id <uuid>` does (*help*). *Codex reading*: native
execution, rewake's registration kept; a bare binary supplies no identity or report route.

**Property.** The core knows no harness: the name goes to each harness through its adapter.

## 3. The shell allowance `Bash(rewake:*)` — KEEP

**Why.** The command line is the first-class interface (`project.md`); a model in a
Claude Code session asks before each `rewake` command without it. The mod's own tools are
answered by the mod and need no allowance (*skill*).

**Code.** Kept in `claude.go`. Under the mod's join the person grants it once in their
own settings, or each command is asked; rewake never writes it.

**Other harnesses.** pi has no permission prompts by design; `--tools` is an allowlist,
and a run that uses it must name rewake's tools (*help*, *pi reading*). Codex keeps its own sandbox and approval.

**Property.** The command line is the interface; the person's configuration unedited.

## 4. The mail — KEEP

**Why.** Native `SendMessage` (*docs*): plain text, no kinds, nothing kept for a session
that is not running (*skill*: not delivered to), at most 100 held and 50 queued, identical
repeats dropped, rate-limited, Claude Code only. rewake's mail is durable, typed, owes one
report, is withdrawn and edited, proves reading, and crosses harnesses.

**Code.** `internal/core/mail`, `host/delivery`, `cli` unchanged.

**Other harnesses.** pi: no native messaging between sessions (*pi reading*). *Codex reading*: Codex's task tools
and message board are scoped to its own server or subagent tree; none supplies kinds,
proof of reading or one report per task. KEEP is the only verdict that lets a pi or Codex
worker sit in a Claude Code main's room at all.

**Property.** Every rule of the mail; one channel for every harness.

## 5. Native `SendMessage` inside a rewake session — MOD, P13

**Why.** A model in a rewake session sees `SendMessage` and `ListAgents` beside rewake's
tools and may answer a task through them: the report then reaches the other session's
inbox without rewake knowing, and the task stays owed. A `session.send` hook may rewrite,
redirect or refuse a send (*skill*); refusing a send to a room member with a hint to use
`rewake send` keeps one channel. `ListAgents` shows native sessions, not the room.

**Code.** New, in the mod. Not built for a session outside a room.

**Other harnesses.** None: neither pi nor Codex offers a parallel channel to the same peers
(*Codex reading*: its task tools address its own threads only).

**Property.** One report per task through the channel that counts it.

## 6. Reading mail by tool — MOD, P2 P4 P8

As designed ([the mail tools](design-claude.md#the-mail-tools)): `$.tool.register`
answered by the mod over the run's socket, a read counted only on proof. Nothing native
replaces it: the native inbox delivers text into the conversation and proves no read.
**Other harnesses.** pi: `pi.registerTool` in the extension, schemas in TypeBox, the
extension talking to the run's socket directly from Node; binding ids from the call's
`toolCallId`, the session id and a tracked turn index; no reserved fields known; tools run
in parallel by default (*pi reading*; PI2, PI7). *Codex reading*: dynamic tools on the app-server (`dynamicTools` at thread
start) are the native candidate, preferred over the archived MCP injection; the tools run
outside the worker's filesystem sandbox; writing a response is not a read receipt (CX1).

## 7. Turn boundary and the confirmation — MOD, P3

As designed ([the turn boundary](design-claude.md#the-turn-boundary)). One addition from
the skill: `classic.Stop` carries `background_tasks` and `session_crons`, so a session
that ends its turn with a background command still running is visible to the mod — a
worker's end is then not the end of its work, and the confirmation should say so rather
than count the report early. **Other harnesses.** pi: the end is
`agent_settled`, not `agent_end`, which retries and steering may follow; `agent_before_settle`
may append an entry and ask for one more request — a native confirmation — and `aborted`
names a stopped turn (*pi reading*, 1.1.0 and 0.87.0; PI6). *Codex reading*: `turn/started` and `turn/completed` are
observations; a Stop hook can block with feedback and continue the same turn in source,
and its trust and failure paths are open (CX2).

## 8. Telemetry — MOD, P6

As designed ([telemetry](design-claude.md#telemetry)): `session.measure`,
`$.session.usage()`, model and effort from the mod; no transcript is read. **Other
harnesses.** pi: context use, cost, model and thinking level native in the extension;
no provider limits; context use empty right after a compaction; a virtual model names the
physical one on its messages (*pi reading*; PI8). *Codex reading*: token usage, context window, rate limits,
model and effort are native notifications, bound to the seated worker's thread; account
limits are not per-worker quotas.

## 9. Wake and delivery — NATIVE or MOD, P1

**Why.** rewake's wake on Claude Code is a line written to the session's inbox socket —
the same socket documented messaging uses (*docs*: a message lands between tool calls and
wakes an idle session). That is the native carrier already; what stays rewake's is what
the line says and its proof. The hidden `--messaging-socket-path` flag can go: the socket
is exported as `CLAUDE_CODE_MESSAGING_SOCKET` (*docs*), readable by the mod, and the
fallback directory is fixed. Two native facts bear on it: a session in `bypassPermissions`
holds messages from a sender not in bypass by default, and held messages expire after five
minutes under `-p` (*docs*); and `$.prompt.submit` from the mod is the other candidate,
compared in P1.

**Code.** `adapter/claude/lane.go`, `socket.go`, `receipt.go`, `notice.go`, `replysweep.go`
stay until P1 picks; `host/delivery/held.go` follows the hold's outcome.

**Other harnesses.** pi: the extension wakes an idle session and steers a busy one with
`pi.sendMessage` (`triggerTurn`, `deliverAs`); a steer lands after the current tool batch,
so a long shell command delays it, and since 1.2.0 a send without `deliverAs` throws while
a run starts (*pi reading*; PI3). *Codex reading*: `turn/start` and `turn/steer` native, durable mail
and the announcement kept; idle, busy and interrupted behaviour on the pinned version open
(CX3).

**Property.** A notice wakes; the mail itself is never lost with it.

## 10. Compaction and interrupt — MOD, P7

As designed ([control](design-claude.md#control)): `$.session.compact`, `$.turn.abort`.
Native messages run no command (*docs*: commands in the text are not executed), so
neither verb can ride the native carrier. **Other harnesses.** pi: `ctx.compact`
takes custom instructions — the instruction main may pass — and `ctx.abort` interrupts;
queued steers continue after an abort unless cleared (*pi reading*; PI5). *Codex reading*: `thread/compact/start` takes only
the thread id, so the instruction main may pass with a compaction is not carried by plain
compact (CX3); `turn/interrupt` is native.

## 11. /clear and resume binding — MOD, P9

Resume itself is Claude Code's, and `classic.SessionStart` reports its source, `fork`
since 2.1.214 (*docs*); `/clear` gives `session.end` without a new `session.start`
(*skill*). The binding of a run to its conversation follows those events, as designed
([/clear and resume](design-claude.md#clear-and-resume)). `resume.go`'s `--add-dir` stays
until stage 6. **Other harnesses.** pi: `--continue`, `--resume`, `--session`, `--fork`,
`--session-id` (*help*); `session_start` names startup, new, resume or fork; a new session
is reachable only from a command's context, and the extension is rebuilt on every switch,
so its binding is restored on `session_start` (*pi reading*; PI4). *Codex reading*: start, resume and fork native; resume restores
saved dynamic tools but needs a live handler; cwd, tools and grants rebound on a switch
(CX4).

## 12. Grants — SPLIT, stage 6

**Why.** The authority — which main grants which directory for which task, and the end of
it — is rewake's and harness-neutral ([grants-authority.md](../grants-authority.md)). Its
application on Claude Code is stage 6's mod (`classic.PermissionRequest`,
`classic.PreToolUse`), as designed. Inside a Claude Code worktree the native isolation
checks already block writes outside it and allow Git there (*docs*).

**Other harnesses.** pi: no sandbox and no permission system, by design; `tool_call` can
block the built-in file tools outside a grant, but a shell command cannot be checked by
path, so a grant is enforced only under an OS sandbox the wrapper starts (*pi reading*;
PI9). Its protected directories: `.pi/` and `~/.pi/agent`. *Codex reading*: native enforcement, task
authority kept; a Git grant names both the private and the common Git directory, resolved
through Git, not guessed (`research-permissions.md`, September 17 on 0.154.0); active-turn
grant and revoke open (CX5).

**Property.** Grants scoped by task and revoked at its end.
