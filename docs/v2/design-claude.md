# Design: the Claude Code adapter

Everything on the Claude Code side goes through one official mod, from Claude Code
2.1.287 on (decision 1). The mod is TypeScript the binary generates or embeds for one run
([answer 9](design.md#the-owners-answers)); the Go
side of the adapter launches the harness, receives the mod's calls at the host's
endpoint, and answers them through the core. The overview is in [design.md](design.md).

This document keeps rewake's decisions about the mod: what it is used for, what the host
does with each call, and what is still to be proved live. It does not describe the mod
API. No work on the mod — planning, code or review — starts without the `mods` skill
loaded ([AGENTS.md](../../AGENTS.md#working-on-the-claude-code-mod)), and a call, event
or field named below is named only to say what rewake does with it; its shape, limits and
version history are the skill's ([where the API facts live](#where-the-api-facts-live)).

## Offered capabilities

| Capability | How | Stage 4 probe |
|---|---|---|
| Launch | `claude` with one `--plugin-dir` | interactive first launch in an untrusted directory |
| Wake | one of three candidates | P1 decides |
| TurnBoundary | `turn.start` and `turn.complete`, reported without holding (S4-6); `classic.Stop` with `block` later | P3, for the hold |
| Telemetry | context and configured model through the mod | built October 18, 2026; live acceptance pending |
| Control | compact with focus and interrupt through the mod | built October 18, 2026; live acceptance pending; no clear |
| PersonUI | stage 6: `$.ui.status`, panes | stage 6 |
| Spaces | a main run's hello admitted; a worker named by `--session-id <uuid>` | P4 |

ToolTransport and Permissions left the design by the owner's decision of October 18, 2026,
with the mail tool and grants ([the mail tools](#the-mail-tools)).

## Where the API facts live

A fact about the mod API is not written in this repository: the `mods` skill carries it
and is kept current with every Claude Code release (owner's decision, October 11, 2026).
That covers the events and their fields, the `$` calls and their refusals, limits and
budgets, the static-analysis rules, the reasons a mod does not load, the plugin layout,
testing a mod, and what changed between versions. Before a stage 4 run, compare the
version the skill was taken from with the installed `claude --version`.

What stays here: rewake's decisions, its invariants, and what its own probes observed.
An observation of a given day goes into the research documents with the version it was
made on, as a dated record; it is not a statement of the API. A claim below marked
*to verify* is one the skill does not cover, or one where it disagrees with the design;
the probe named beside it decides, and the decision is not changed until then.

## Loading the mod for one run

The wrapper writes the mod into the run's directory and passes `--plugin-dir <dir>` to that
launch only. Nothing in the person's settings changes (L1); the directory goes with the run
(L3).

The mod's files — a manifest named `rewake`, the hooks file, `hooks/rewake.ts`, its outbox in
`hooks/outbox.ts` and the generated `protocol.gen.ts`, each named in the `embed` line, since a pattern skips the
manifest's dot directory — are embedded from `internal/adapter/claude/mod/` and written by
the run's service into `<run dir>/mod` before the harness starts; the launch names the run's
mod socket in `REWAKE_MOD_SOCKET`, replacing any inherited one, which the adapter also lists
among the variables a run does not inherit. The socket sits beside the run's inbox socket in
the room's `sock/`, as `<name>.<run>.mod.sock` or a hashed name within the 103 bytes a
socket's path may take; a killed wrapper's mod socket is swept with the dead reply sockets,
by a dial refused (built in S4-5, October 13, 2026).

The module stays inside the skill's static-analysis rules: at `session.start` it reads the
socket's path, sends `hello` with its load's id, the session's id and the harness's version,
and keeps the answer; outside a rewake run it reports to nobody. `modhost` answers on the
socket, checking each request by its peer (SP4), and writes the admitted hello to
`<run dir>/hello.json`, what a person or a live check reads of the load while the run lives.
Its handlers count among the run's workers ([design-api.md](design-api.md#handlers-that-write-the-room)):
a closed server begins no record; under one lock the peer is checked again, then the hello is
bound, recorded and made live, so a harness replaced while the body was read gets no effect.
The answer says whether the run hosts spaces — a main run does — and an admitted hello of such
a run makes Spaces live ([design-api.md](design-api.md#spaces)).
Three tests hold the files: the embedded copy equals the source and every file of the source
is embedded or named as not shipped (its tests and the schema); the written mod passes
`claude plugin validate --strict`; the module's own tests pass in `claude plugin test` with
the kit. The last two skip, naming why, where `claude` is not installed.

The plugin name gives the tools their prefix, `mcp__rewake__<name>`, which no mod can
avoid (`.scratch/v2-recon/mods-tool-naming.md`). A person's own MCP server named
`rewake` would show the same prefix; whether the two collide is P8 (*to verify*). The
skill also records a refusal when another installed plugin of the same name loads first:
a person's plugin named `rewake` is a cause of not loading, named below.

## When the mod does not load

The skill lists the causes and how each shows (flags, settings keys, managed policy, an
untrusted directory, the remote switch, a worker that crashed). rewake finds out in three
ways:

1. **Before the harness starts**, from what the launch itself knows: the mod socket's path
   past what a unix socket allows, a run without its directory, the mod's files not written,
   its endpoint not listening. A known cause is named (L2: where, never what), and refuses
   the launch of a role that owes reports ([readiness](stage4-turns.md#readiness-and-admission)).
   Reading the person's flags and settings files for the keys the skill names is not built;
   whether to run `claude plugin test` from a directory with no mod as a preflight is for
   stage 4 to try.
2. **At the start**, by the mod's hello: `session.start` is awaited before the first prompt,
   and the mod's first act there is a hello to the endpoint. No hello is not proof of failure
   (C3): the run's record shows the boundary not live, and the wrapper writes nothing on the
   terminal the harness holds.
3. **While it runs**, by the heartbeat: Claude Code unloads a mod that crashed the hooks
   worker, and every non-built-in mod after three crashes, and tells rewake nothing; the lapse
   of the mod's held poll withdraws what it proved ([the live set](stage4-turns.md#the-live-set)).

Until a hello arrives, TurnBoundary, Telemetry and Control are not live
([design-api.md](design-api.md#two-levels-offered-and-live)), and a session running without
its mod takes no tasks ([answer 3](design.md#the-owners-answers)). P5 includes the losses.

## The mail tools

The mod registers no mail tools: the mail tool left the design by the owner's decision of
October 18, 2026, before it was built on this adapter, and a session reads and sends
through the shell commands. Its one tool, since October 20, 2026, is `worktree_status`,
which runs `rewake worktree status --json` by argv and answers in place of the engine's core
([worktree-tools.md](../worktree-tools.md)). Its design — the tools' registration, the permission check, a
call carried to the endpoint and a read proven by the final result — is in
[rules-tools.md](../archive/rules-tools.md) and [mail-bridge.md](../archive/mail-bridge.md).
What stays of it is how the host tells the mod's requests from a shell command's.

### The socket and its peer

**Telling the mod from a shell command.** Every request to the endpoint is checked on its
own: the peer of its connection, by `SO_PEERCRED`, must be **the harness process itself**
— the pid the wrapper started, alive with its start time before and after the answer, as
the grant authority checked its peer ([grants-authority.md](../archive/grants-authority.md),
steps 1–2). A descendant
of the harness is never accepted, since a command the model runs is one. No secret, token
or session carries authority from one request to the next, so a mod reload, a respawned
hooks worker or a dropped connection changes nothing: each request proves its origin
again. This holds only if `$.http.fetch` connects from the harness's own process: P4,
**answered positive on October 13, 2026, 2.1.296**
([the record](../research-claude-mod.md#p4-the-peer-of-the-mods-requests)) — every
request the probe's mod made, through a reload, the keeper's hold and its polls, named
the harness process itself, and a model's shell child was refused; rewake's own mod's
hello was admitted the same way in S4-5. Still open: a hooks worker respawned after a
crash, and a shell child started before a reload. A negative result on a later version
has **no fallback**: every capability that reaches the host through the mod — TurnBoundary, Telemetry, Control, Spaces — would not be live, a role that
must report would refuse to launch (answer 3 of
[design.md](design.md#the-owners-answers)), and stage 4's working pair would wait for
another trusted channel of events, designed and reviewed; admitting a helper process back
is not that channel. A secret the host hands the mod is not one: after the first load a
shell command may already run, and on a reload it could read and present a reissued
secret first. The threat is the one 1.x tickets answer — a model's command imitating the
tool; a process of the same user rewriting the mod's files is outside it.

The same holds for the endpoint's other callers in the mod — the hello, turn events,
state samples and control answers. Control uses the same heartbeat socket and peer check;
no helper process carries a request or its authority.

## The turn boundary

| Event | The mod does | The host does |
|---|---|---|
| `turn.start` (`turnId`) | sends `turn/start` once, for the main loop alone: a subagent's, with `agentId`, is not sent | `Started` with `at = 0`: no timed start ([the start time](stage4-turns.md#the-start-time)) |
| `turn.complete` (`turnId`, `reason`, `answer`) | sends `turn/end` under a fresh end id until its receipt; `answer`, `aborted`, `refusal`, `error` mapped to answered, aborted, refused, failed | the turn end operation (E7), every retry handed on as its first capture |
| `classic.Stop` | later, with P3: asks the host whether this end may close | the confirmation check ([design-rules.md](design-rules.md#turn-outcomes)); the mod returns `{ block: reason }` when it says go on |
| `classic.StopFailure` | not hooked: `turn.complete` with `error` is the failed end | — |

Events with `agentId` are a subagent's and are not sent. An interrupted turn gives
`turn.complete` with `aborted` and no Stop (2.1.280,
[research.md](../research.md#interrupting-a-turn-commands-and-the-stop-hook)).
S4-6 reports without holding ([stage4-turns.md](stage4-turns.md)). Corrected October 16,
2026: `classic.StopFailure` no longer marks a turn failed; `turn.complete`'s `error` does.

The confirmation moves from the settings Stop hook to `classic.Stop`: it fires where the
hook would, without one configured, and may answer `block` with a reason; a repeated
stop is marked. The skill's types allow it; that the chain works live — the order of
`classic.Stop` and `turn.complete`, the block continuing the turn — is P3 (*to verify*).

## Telemetry

Built October 18, 2026: the mod samples native context and the configured model after its
hello, on `session.measure`, at the main turn's start and end, after each main-loop
`turn.step`'s response (from October 21, 2026: `session.measure` comes only at a turn's end,
so a long turn's fill waited for the next heartbeat, up to 15 s), at compaction boundaries,
and after each heartbeat answer. It sends complete samples through `mod/state`, with the
load, heartbeat generation, selected conversation and a load-local sequence. The host
accepts only its current live generation and newer samples; unknown tokens clear an older
reading. It publishes cached observations through `core/session.Snapshot`, without reading
transcripts, wrapping the person's status line or measuring a full context breakdown.
The native model window and percentage are used as given. Effort is what the latest
main-loop `turn.step` asked for, kept by the host across loads; before one, the launch's
`--effort`, marked as the launch's (`reasoningEffortSource`), or unknown. From October 23,
2026 a worker's sample names a dialog it waits at that nobody is asked about, and the host
tells main of it once ([permission-requests.md](../permission-requests.md#a-dialog-nobody-is-asked-about)). Rate limits remain
absent. A lapse makes every field stale until a new generation supplies a sample.

The prompt cache (October 23, 2026) is the usage the API reported for the latest main-loop
`turn.step` response (`usage.cache_read_input_tokens`, `cache_creation_input_tokens`,
`input_tokens`; Claude Code 2.1.302): the mod keeps it and sends `cacheRead`, `cacheWrite` and
`cacheInput` with every sample, an additive change of `mod/state`; a response with no usage and a
subagent's leave the last. The host publishes `Snapshot.Cache` (`cache` of the telemetry): the raw
counts and `hitPercent`, the share of all the request's input tokens — read, written and neither —
that the cache served. It is the last request's, not a running total, since that is what says whether
the prefix is still warm; providers count input differently, so the harness's own usage is the only
source and rewake adds nothing to it. pi's extension sends the same three counts from
`message_end` ([pi record](../research-pi.md)); a harness or an older mod that reports none leaves
the field out, and `rewake list` shows a `Cache` column only when some session has it.

Completed compactions have a load-local count; the host folds its increases into the run's
count and bounded notification events. A reload keeps the run's count and marks coverage
partial. Native manual and automatic compactions are observed through the mod's event hook;
remote ones through the native call's result, so re-entry cannot count one twice. A skipped
or failed compaction clears the in-progress state and adds no completion. Managed-policy
limits and the tests' scope are in [the parity record](../research-claude-parity.md).

## The cache ttl

October 23, 2026. The mod cannot read how long the prompt cache lives; Claude Code's status-line input
carries it as `prompt_cache` (`warm`, `ttl` of `5m` or `1h`, `expires_at` in epoch seconds,
`requests`, `misses`, `hit_ratio` …), once a request was made (read from the strings of 2.1.302; a live
sample needs a login). A worker's run therefore sets `statusLine` in its `--settings` overlay —
`'<rewake>' statusline`, `refreshInterval` 30 or the person's own — which beats the folder's
`settings.json`. The hidden `rewake statusline` reads that input, posts `ttl`, `expiresAt` and `warm`
to the run's host at `status/line` of the mod socket, and prints the person's own `statusLine` command
from the folder's `settings.json` (run with the same input) or nothing, always exiting 0. The expiry is
absolute, so a slow timer loses nothing; the command runs after each response too. The post (2 s) and the
person's command (5 s) run side by side with deadlines of their own; the person's command runs in a process
group of its own with its output in a pipe rewake reads, killed whole at the deadline, so it cannot keep the
harness's output open, and `REWAKE_STATUSLINE_PASSTHROUGH` in its environment makes a `rewake statusline` it
calls print and post nothing. The stdin read is bounded in time (3 s) as well as size.

`status/line` is the one endpoint served to the harness's descendants and not only its own process
(`fromHarnessOrChild`): the status line is Claude Code's child, and the payload is telemetry that is
acted on nowhere — a Bash call posting it is harmless (main's decision, October 23). The host keeps
the last in `Snapshot.CacheLife` (`cacheLife`: `ttl`, `expiresAt`, `warm`), beside `Cache`, which comes from
the responses' usage; `rewake list` shows the Cache cell as `90% 42m` (`—` once expired, `<1m`), the
remaining time counted by the reader's clock. Main's own run has no such status line. pi has no
observed expiry (only a model's best-effort lifetimes per retention tier), so nothing is shown there.

## Control

Built October 18, 2026: compact with optional focus and interrupt. Clear is not offered.
`control.Serve` reads the existing request directory; the host queues the request for the
current load and generation and wakes its held `mod/alive` poll. An `AliveAnswer` carries
at most one `ControlCommand`, dispatched once. The poll chain immediately renews while the
command runs separately. `mod/control` carries its initial and final answers, matched to
that dispatched id and generation. These two endpoints and `mod/state` use the generated
mod protocol and the same peer checks as the hello; no new journal or durable outbox is added.

Compaction refuses a running main turn or another operation in flight. The mod returns
`requested`, passes the focus to native compaction, and later returns `done` with native
before/after counts when supplied, or a refusal or failure. The host keeps that final result
in the existing compaction outcomes for main's letter. It never sends the summary.

Interrupt names the running main turn and waits for its matching `aborted` completion.
The stopped report names the requester, and the mod appends that actor to native history
without starting a turn. A normally completed turn refuses the interrupt. A missing end
after three seconds gives an open failure; an end heard later still carries the actor.

The host waits five seconds for a control acknowledgement. A command not dispatched by then
is removed and refused; one already dispatched has an unknown outcome and is not repeated.
An old generation cannot take commands or update state, even though its durable turn ends
remain acceptable under the separate outbox protocol. A reload or failed final post can
lose a compaction outcome; main asks again, as the current product policy permits.
Telemetry and Control are live only while the admitted hello's heartbeat holds, alongside
the turn boundary; Control also needs the run's control directory.

## Wake and delivery

Three candidates (decision 6), decided by P1:

| Candidate | What it is | Known |
|---|---|---|
| `$.prompt.submit` | the mod starts a turn with the notice | waits until idle and starts a turn; no steering mid-turn; the model sees a frame naming the plugin unless sent as the person's (the skill); a repeat of `session.start` does not repeat an earlier submit |
| the 1.x line | a line on the session's inbound socket, at a path rewake fixes with the hidden `--messaging-socket-path` (`harness/claude/claude.go:33`), with the inbound gate's receipts | built and tested in 1.x (`docs/delivery-adapters.md`); steers between tool calls |
| the documented messaging | the same inbound socket as documented: `CLAUDE_CODE_MESSAGING_SOCKET` and `CLAUDE_CODE_MESSAGING_TOKEN`; the mod sees `session.receive` and may consume it | documented, not tried by rewake; whether it differs from the 1.x line beyond the token is part of P1 |

The notice could reach the mod by the same stream as control; the host keeps deciding
what to send and when ([design-api.md](design-api.md#wake)). The fate of the 1.x lane,
`inbox/held.go` and its receipts follows P1 ([revision-core.md](../archive/v2/revision-core.md#the-delivery-server-and-heldgo)).

## /clear and resume

`/clear` ends the conversation and continues the process under a new session id, without
a new `session.start`; resume gives its own end reason (the skill; whether a branch is
named is P9). The mod tells the host each change of session id, and the run's
conversation binding moves with it, so a call bound to the old conversation cannot
commit into the new one (T2, T7). The skill names `classic.SessionStart`, which fires
after a clear, a resume and a fork (the branch) with its source, as the natural carrier of that
telling; the choice between it and the end event is for stage 4.

## The workers' screens

The mod draws no worker. Its pane, `/workers` and the `workers/*` endpoints were removed on
October 18, 2026, when the host mode replaced them; since October 19, 2026 rewake's shell draws
the workers on the person's terminal ([shell.md](shell.md); the pane's design is archived,
[workers-panel.md](../archive/v2/workers-panel.md)).

## What the mod never does

It reads no transcript and no prompt text (`turn.start`'s `text` is not forwarded); it
forwards the end's `answer`, which is the report, as 1.x forwards the Stop hook's last
message. It replaces no status line, sends no settings, and does no heavy work in a
hook: the hooks worker is shared and a mod that stalls it is unloaded.

## Before the probe: what Claude Code already does

Stage 4 starts with an audit, before the probe and before any adapter code (owner's
decision, October 12, 2026). Much of the 1.x Claude Code side — the settings hooks, the
telemetry collector, the wake line, the worktree launch — was written before mods existed
and copies what Claude Code or a mod may now give natively. Each such feature is weighed
against the installed Claude Code and the `mods` skill: kept as rewake's, handed to a
native feature, or built on a mod event instead. Only what survives the audit is carried
into the adapter or the probe.

The audit was written on October 13, 2026, on 2.1.296
([stage4-audit.md](../archive/v2/stage4-audit.md), its rows in [stage4-audit-rows.md](../archive/v2/stage4-audit-rows.md) and
[stage4-audit-rows-space.md](../archive/v2/stage4-audit-rows-space.md)).
In short, as proposals for main and the owner, with main's decisions of the same day
marked:

- **Joining** stays open: work between Claude Code sessions could run through the mod
  alone, with `rewake claude` reduced to a launcher, the harness process anchoring the run's
  ancestry and a process the mod starts at the first `session.start` — detached, outliving
  reloads — serving it as endpoint, lease and final writer. That split loses the
  decision's same-process proof; the caller-to-final-writer proof, a late opt-in and the
  anchor's death during an admitted decision are open obligations of P11, and the wrapper
  stays unless a replacement authority contract is accepted. P11 runs first (decided by
  main). Its first half held live the same day: a process the mod starts at the first
  `session.start`, before any prompt, is the harness process's child, may detach, outlives a
  module reload and ends with the harness ([the record](../research-claude-mod.md#p11-a-process-the-mod-starts));
  the three obligations are untested.
- **Kept as rewake's**: the mail, the registry, the room view, the shell allowance, grant
  authority, the version gate and the scrubbing of children's variables. Native messaging
  keeps nothing for a session that is not running and knows no kinds or reports, so it
  serves as the wake carrier only, which it already is.
- **Built on the mod**: reading by tool, the turn boundary, telemetry, control, the
  conversation binding, and a `session.send` guard that keeps a report off native
  `SendMessage` (P13); `--name` passes rewake's name to Claude Code's own (P13), and the
  briefing may move to a prompt event, cached as those are, if the system prompt's snapshot
  on resume hides a changed one (P14).
- **The worktree's whole life** goes to Claude Code, even with another harness inside (the
  owner's direction). The keeper, after the first probe: a subagent of main with worktree
  isolation, of an agent type the mod declares, spawned by the mod itself with
  `$.agent.spawn` — no turn of main's model — and held by the mod's blocking tool for the
  space's life. Claude Code makes its checkout from `origin/<default>`, locks it while the
  keeper runs, and removes it unchanged or keeps it changed when the keeper ends
  ([the record](../research-claude-mod.md#p12b-a-subagent-of-main-as-the-worktrees-keeper)); the base the work's
  branch needs is open. The background-session keeper is dropped: workers close with main by
  the owner's decision ([answer 7](design.md#the-owners-answers)), so a keeper outside main
  has no purpose. The mod's refusal of a removal while a worker sits there stays conditional
  on P12; if the keeper fails, rewake keeps one lifecycle that follows Claude Code's
  conventions. Either way most of the adapter's own worktree launch goes.
- **The target** the owner set the same day is a space: main creates a pseudo-terminal rewake
  owns, shown as a tab in main's Claude Code, a worktree may be made in it, and a worker is
  seated there; stage 4's first end to end is that flow with a Claude Code worker, if P11,
  P12, P12b and P15 allow ([the target](../archive/v2/stage4-audit.md#the-target-a-space)); its design is
  [stage4-space.md](stage4-space.md). Closing main
  closes its spaces; the space's record keeps each worker's session id for a restore
  ([answer 7](design.md#the-owners-answers)) and its kept checkouts for a reminder.
- **Worker tabs**: the pseudo-terminal belongs to the space's terminal process, a rewake
  process whose identity P11 and P15 decide, so any harness can be hosted. The tab is
  preferably the keeper subagent's entry in Claude Code's tasks list (the owner's idea): the
  mod draws the terminal in the keeper's own tool row, or in its pane or band for that view,
  and forwards keys; P15 runs beside stage 4's probe (decided by main). Its first half drew
  a cell grid and a key surface in the keeper's tool row; Escape, Ctrl+C, Ctrl+X and paste
  did not reach the key surface, so a prefix key carries them to the seated harness (the
  owner's direction, [two levels of UI](../archive/v2/stage4-audit.md#two-levels-of-ui)). Its second half,
  P15b, drew a real pseudo-terminal there, carried the keys and the prefix to it as terminal
  bytes and ran a nested Claude Code inside
  ([the record](../research-claude-mod.md#p15b-a-real-pseudo-terminal-in-the-keepers-row)).
- **Background sessions** under Claude Code's supervisor are dropped, and P16 with them,
  by the owner's decision that workers close with main
  ([answer 7](design.md#the-owners-answers)).

## The stage 4 probe

Main runs it live on the installed Claude Code, before the adapter is built; each answer
goes into the research documents with its version. The skill answers some questions in
part; what it leaves open is named in the row.

| # | Question | Decides |
|---|---|---|
| P1 | Wake: each candidate idle and busy, under `-p` and interactive, after `/clear` and a compaction; steering; how the person and the model see it; identical-text suppression; a held line's outcome | the Wake mechanism, and the fate of the lane and `held.go` |
| P2 | Does `classic.PostToolUse` or `classic.PostToolBatch` carry the **final** result for a mod-answered call, and does `classic.PostToolUse` fire at all for one the mod answered without passing it on: with a settings hook that replaces a short result after the mod; with a large result turned into a preview; with the event or the host's answer lost; with the turn aborted between the result and the next request. Each case compared with what a stand-in model API received | whether reading tools are live, and the effective bound (T6); the observed order goes into the fixture |
| P3 | `classic.Stop` with `block`: the turn continues; a second Stop with the repeat mark; one `turn.complete` at the end; an aborted turn; does a main-loop `tool.call` ever fall outside its turn's start and complete | the confirmation, and the turn a call belongs to |
| P4 | Which process or thread `$.http.fetch` with `socketPath` connects from (the skill says one shared worker thread, the 2.1.280 record a process per plugin): is it the harness process itself, by `SO_PEERCRED`, on every request; the same after a reload, a worker respawn, a repeated hello, a repeated native call; a shell child started before the reload tries the endpoint and is refused | whether any capability that reaches the host through the mod — ToolTransport, TurnBoundary, Telemetry, Control — can be live; negative, reporting roles refuse to launch and stage 4's pair is blocked. **Answered positive, October 13, 2026** ([the record](../research-claude-mod.md#p4-the-peer-of-the-mods-requests)): every request the probe's mod made was the harness process itself, through a reload, the keeper's hold and its polls, and a model's shell child was refused; still open: a hooks worker respawned after a crash, a shell child started before a reload |
| P5 | What each cause the skill lists shows when the mod does not load, interactive and `-p`; a same-named installed plugin; does the remote switch reach `--plugin-dir`; what the host sees when a loaded mod is unloaded later, when the whole mod set is unloaded after worker crashes, or when its stream child dies | the preflight, the notice, the withdrawal of a lost capability |
| P6 | Cadence of `session.measure`; is the breakdown call, which measures against the compaction window, acceptable (the skill says `percent` is not against it) | whether the window parser goes |
| P7 | `$.session.compact` idle and mid-turn; `$.turn.abort`; does `$.command.run` with the clear command work live, and does it raise `turn.start` and `turn.complete` | Control's verbs |
| P8 | Reserved fields under forged arguments; `isDeferred: false` in the registration, and whether the model then calls the tools; a `permissions.deny` rule and `$.tool.check` on a mod tool, with neither an effect nor shell advice after a deny; whether the denial answer reaches the model as an error where the design assumed plain text; a person's MCP server named `rewake` | the tool set's safety; with no reliable check, no tools |
| P9 | Tools after `/clear`; session ids across `/clear`, resume, branch, and `classic.SessionStart`'s source; are turn ids ever reused across an interrupt, a reload, `/clear`, a resume | the conversation binding; whether a retry may mark (T7) |
| P10 | A `$.process.spawn` child across turns, reloads and `session.end`, and its environment against the Bash tool's | the control and notice stream; why the endpoint and not a child |
| P11 | A mod's join without the wrapper: whose child a `$.process.run` child is and whether it may detach and outlive a reload, a worker respawn and `session.end`, ending with the harness process; whether it starts before any model command at the first `session.start`; the inbox socket's path from `$.env.get`; the hello naming the harness's pid and build, checked by the detached process against the peer of the mod's requests (P4). Its open obligation, which this row does not yet test: a late opt-in, a join after model commands ran, where the first `session.start`'s ordering does not hold. Runs first (decided by main, October 13, 2026). **First run, October 13, 2026** ([the record](../research-claude-mod.md#p11-a-process-the-mod-starts)): a `$.process.run` child is the harness process's own child, started at the first `session.start` before any prompt; it detaches with a double fork and `setsid`, outlives `/reload-plugins` and ends with the harness; the inbox socket's path comes from `$.env.get`. Still open: `session.end` without an exit, a worker respawn, the harness killed outright, the hello checked against the peer (P4), and the late opt-in | whether `rewake claude` can become a launcher, the harness process anchoring the run's ancestry; negative, the wrapper stays as designed |
| P12 | Claude Code's own checkout from a worker's `-w` and `EnterWorktree`, and a resume into it: where it lands and how the mod learns the finished path; `classic.WorktreeRemove` answered with a refusal while a rewake session sits there, at exit, `ExitWorktree`, the clean unnamed exit, `claude rm` and the sweep, and whether it fires at all for a checkout made without a hook; the mod not answering or failing; a `git worktree lock` Claude Code did not take on each of those routes; the isolation checks inside the checkout. For the fallback: `classic.WorktreeCreate` answered with rewake's checkout. Seen in P12b's runs, October 13, 2026: Claude Code Git-locks a subagent's checkout itself, and Git keeps one lock per worktree, so rewake cannot add its own there ([the record](../research-claude-mod.md#mains-exit-with-a-worker-inside)) | whether the mod's veto holds on every removal route (row 13 of the audit) and that `adapter/claude/worktree.go` goes; with no reliable veto, the explicit keep policy |
| P12b | The worktree's keeper for a worker of another harness. **Chosen after the first run, October 13, 2026** ([the record](../research-claude-mod.md#p12b-a-subagent-of-main-as-the-worktrees-keeper)): a subagent of main with worktree isolation, of an agent type the mod declares, spawned by the mod with `$.agent.spawn` without a turn of main's model and held by the mod's blocking tool as a loop of polls, since `$.http.fetch` gave up after 30 s; its checkout is made from `origin/<default>`, locked while it runs and removed unchanged at its end; when main exits with the keeper running, the checkout stays, locked under a dead pid. The background-session keeper is dropped by [answer 7](design.md#the-owners-answers) (it also refused to start in an untrusted directory), and a named spawn gives no teammate. Still open: the base the work's branch needs, what sweeps a checkout an exit left locked or a keeper kept with changes, the seated worker's Git metadata writes, the cost of a long hold, and the second call that came under an agent id `$.agent.list()` never listed — seen again in every P15b run that held the keeper, about 30 s after the first call ([the record](../research-claude-mod.md#the-keepers-second-call)) | which keeper holds a space's checkout and that `host/worktree` goes; negative, one rewake lifecycle that follows Claude Code's conventions |
| P13 | Native `SendMessage` from a rewake session to a room member refused by a `session.send` guard, and what the model sees; `ListAgents` beside the room; `--name` as the address and the title; a rename by `/rename` | the guard and the name |
| P14 | The briefing on resume with `--system-prompt-snapshot` on and off, a role changed between launches; the same text from `prompt.section` and from `prompt.context`, both cached by the engine, and the invalidation each needs to show the changed text | the briefing's carrier |
| P15 | A pseudo-terminal of the space's terminal process drawn by the mod where the keeper subagent is opened from the tasks list: first in the keeper's own tool row in its transcript — may a transcript row carry the cell grid and the client element and take keys, its height in the agent's view with fullscreen on and off, its repaint by `$.ui.blit`, scroll and focus — then in the pane or the band, whether it follows the view switch, may be opened on it, and its size; in either, Escape and its remapping, Ctrl+C, Ctrl+X, Alt, paste, function keys, focus in and out, a size of 200 by 50, the redraw cost over `$.http.fetch`; a reload of the mod and main's exit with the worker alive. The two levels of UI ([the target](../archive/v2/stage4-audit.md#two-levels-of-ui)): which keys main's Claude Code keeps — switching between agents, Escape back to main's prompt — and how the seated harness still gets them, by a remap or a button; the rows and columns the inner screen gets under the outer chrome, fullscreen on and off; whether two nested tab bars confuse the person; what the space's entry shows (name, harness, state). Stage 6 by [ideas.md](ideas.md#1-sessions-as-tabs-inside-claude-code); runs beside stage 4's probe (decided by main, October 13, 2026). **P15a ran October 13, 2026** ([the record](../research-claude-mod.md#p15a-the-spaces-terminal-in-the-keepers-own-tool-row)): the keeper's view opened from the tasks list by Down, Down, Enter; its tool row carried a Raster with a Client line under it, repainted by `$.ui.blit` once a second and mounted only while the view is open; a grid of 196 by 44 rendered at 200 by 50 without fullscreen, about 16 rows going to Claude Code's chrome; after a click the Client got letters, Alt as `meta`, F1, the arrows and Tab, and not paste, Ctrl+X, Ctrl+C or Escape. **P15b ran the same day** ([the record](../research-claude-mod.md#p15b-a-real-pseudo-terminal-in-the-keepers-row)): a real pseudo-terminal of 160 by 30, its screen kept by a VT emulator, drawn in the keeper's row; the keys and the prefix ([two levels of UI](../archive/v2/stage4-audit.md#two-levels-of-ui)) reached it as terminal bytes, 31 ms from key to echo at the median; vi, less and a nested Claude Code worked inside, the nested one answering a prompt, repainted live, and ended by the prefix's Ctrl+C twice. Still open: which Ctrl keys beyond A and B reach the Client; paste; the keeper's second call about 30 s after the first; fullscreen, not run, since a fullscreen start writes a boot canary into the configuration the person's own sessions share; the pane and the band | whether worker tabs are buildable and the launch of every adapter owns a terminal |
| P16 | `claude --bg` and `--bg --exec` as the host of a rewake worker: the supervisor's lifetime against the wrapper's, `claude agents --json` fields, `claude rm` and the worktree. Dropped by the owner's decision of October 13, 2026 that workers close with main ([answer 7](design.md#the-owners-answers)) | whether background sessions can carry a worker |
