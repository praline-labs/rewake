---
description: The Claude Code adapter as one mod — loading it, when it does not load, the mod↔host protocol, the peer check, tools, telemetry and the cache's life, control.
---

# The Claude Code adapter

Everything on the Claude Code side goes through one official mod, from Claude Code 2.1.287 on.
The mod is TypeScript embedded in the binary and written for one run; the Go side of the
adapter launches the harness, answers the mod's requests at the run's endpoint, and reaches the
core through what the host hands it. A Claude Code session's turns have their own document,
[claude-turns.md](claude-turns.md).

This document keeps rewake's decisions about the mod: what it is used for and what the host does
with each request. It does not describe the mod API. No work on the mod — planning, code or
review — starts without the `mods` skill loaded
([AGENTS.md](../AGENTS.md#working-on-the-claude-code-mod)), compared with the installed
`claude --version`. A call, event or field named below is named only to say what rewake does
with it; its shape, limits and version history are the skill's, and what rewake's own probes saw
is in [research-claude-mod.md](research-claude-mod.md), each record with its version.

## What the adapter offers

| Capability | How | Where |
|---|---|---|
| Launch | `claude` with one `--plugin-dir`, the briefing in `--append-system-prompt`, the worker profile for a worker | [launch.md](launch.md#claude-code) |
| Wake | a line on the session's inbound socket, with the inbound gate's receipts | [delivery-adapters.md](delivery-adapters.md#claude-code) |
| TurnBoundary | `turn.start` and `turn.complete` of the main loop, reported without holding, proven after the harness starts | [claude-turns.md](claude-turns.md) |
| Telemetry | native context, the configured model and its name, effort, thinking, compactions, the prompt cache and its life | [below](#telemetry) |
| Control | compact with an optional focus, interrupt; no clear | [below](#control) |
| Mirror | a run launched with `LaunchRequest.Mirror` sends its conversation's live events to the wrapper's model, and takes the face's prompts | [below](#the-mirror), [face.md](face.md) |
| Spaces | a main run, once its hello is admitted | [rules/adapter-api.md](rules/adapter-api.md#spaces) |
| Conversation | `--session-id <uuid>` | [rules/adapter-api.md](rules/adapter-api.md#conversation-names) |

The person sees the workers in rewake's shell ([shell.md](shell.md)), never inside Claude
Code. TurnBoundary, Telemetry, Control and Spaces are live only while the
run's admitted hello keeps its heartbeat ([the live set](claude-turns.md#the-live-set)); Wake is
the inbound lane's and does not depend on the mod.

## Loading the mod for one run

The mod's files are embedded from `internal/adapter/claude/mod/`, each named in the `embed` line
of `mod.go`, since a pattern skips the manifest's dot directory: the manifest `rewake`
(`.claude-plugin/plugin.json`), `hooks/hooks.json`, the one module `hooks/rewake.ts` and the
files it imports — `outbox.ts`, `heartbeat.ts`, `management.ts`, `asks.ts`, `calls.ts`,
`dialogs.ts`, `closing.ts`, `worktree.ts`, `briefs.ts`, `enter.ts`, `mirror.ts`, `permits.ts`, `sizes.ts` — and
the generated `protocol.gen.ts`. The run's service
writes them into `<run dir>/mod` before the harness starts, and the launch passes
`--plugin-dir <run dir>/mod` to that launch only. Nothing in the person's settings changes (L1);
the directory goes with the run (L3).

The launch names the run's mod socket in `REWAKE_MOD_SOCKET`, replacing any inherited one, which
the adapter also lists among the variables a run does not inherit. The socket sits beside the
run's inbox socket in the room's `sock/`, as `<name>.<epoch>.mod.sock` or a hashed name within
the 103 bytes a socket's path may take; a killed wrapper's mod socket is swept with the dead
reply sockets, by a dial refused.

At `session.start` the module reads the socket's path and says hello; outside a rewake run it
reports to nobody. `modhost` answers on the socket and writes the admitted hello to
`<run dir>/hello.json`, what a person or a live check reads of the load while the run lives. Its
handlers count among the run's workers
([rules/adapter-api.md](rules/adapter-api.md#handlers-that-write-the-room)): a closed server
begins no record; under one lock the peer is checked again, then the hello is bound, recorded
and made live, so a harness replaced while the body was read gets no effect. The answer says
whether the run hosts spaces — a main run does — and an admitted hello of such a run makes
Spaces live.

Three tests hold the files: the embedded copy equals the source and every file of the source is
embedded or named as not shipped (its tests and the schema); the written mod passes
`claude plugin validate --strict`; the module's own tests pass in `claude plugin test` with the
kit. The last two skip, naming why, where `claude` is not installed.

The plugin's name gives its tools their prefix, `mcp__rewake__<name>`, which no mod can avoid.
Another installed plugin named `rewake` that loads first keeps this one from loading (the skill
lists it among the causes).

## When the mod does not load

The skill lists the causes and how each shows: flags, settings keys, managed policy, an
untrusted directory, the remote switch, a hooks worker that crashed. rewake finds out in three
ways:

1. **Before the harness starts**, from what the launch itself knows: the mod socket's path past
   what a unix socket allows even hashed, a run without its directory or without an inbox socket
   of rewake's, the mod's files not written, its endpoint not listening. A known cause is named
   (L2: where, never what), and refuses the launch of a role that owes reports
   ([readiness and admission](claude-turns.md#readiness-and-admission)). The person's flags and
   settings files are not read for the keys the skill names.
2. **At the start**, by the hello: `session.start` is awaited before the first prompt, and the
   module's first act there is the hello. No hello is no proof of failure: the run's record shows
   the boundary not live, and the wrapper writes nothing on the terminal the harness holds.
3. **While it runs**, by the heartbeat: Claude Code unloads a mod that crashed the hooks worker,
   and every non-built-in mod after three crashes, and tells rewake nothing; the lapse of the
   mod's held poll withdraws what it proved ([the live set](claude-turns.md#the-live-set)).

Until a hello arrives, TurnBoundary, Telemetry, Control and Spaces are not live, and a session
running without its mod takes no tasks: a task or question to it exits 1, naming why
([rules/adapter-api.md](rules/adapter-api.md#turnboundary)).

## The protocol

**Transport.** HTTP/1.1 over the run's mod socket. The mod reads the path with `$.env.get` and
connects with `$.http.fetch`. The path is no secret: no request carries authority but its peer.
Each wrapper answers its own run's mod — main's and every worker's alike.

**Bounds.** `$.http.fetch` gives up after 30 s, so no request is held longer than 20 s
(`maxWaitSeconds`); a wait is a loop of polls. A request body is at most 64 KiB (`maxBodyBytes`),
a turn's end 1 MiB, its answer cut by the mod past that and saying so
([the size](claude-turns.md#the-mods-side)).

**Endpoints.** Every one a `POST` with a JSON body under `/v1/`, answered 200 with its answer or
another status with a refusal:

| Path | Caller | What it carries | Where |
|---|---|---|---|
| `hello` | every load, at `session.start` | the session id, the harness version, the load's id and number; answered with the run's role, whether it hosts spaces, whether the face draws it (`mirror`), the heartbeat's generation | [the live set](claude-turns.md#the-live-set) |
| `turn/start`, `turn/end` | every role's mod, for its own conversation | a turn's start; its end under an end id, retried until its receipt | [claude-turns.md](claude-turns.md) |
| `mod/alive` | every admitted load, in a loop | the heartbeat; it also carries at most one control command to the mod | [the live set](claude-turns.md#the-live-set), [control](#control) |
| `mod/ended` | every admitted load, once | the epochs of other runs whose ends the store keeps; answered with those rewake proves ended | [the outbox](claude-turns.md#the-mods-side) |
| `mod/state` | every admitted load | native state samples | [telemetry](#telemetry) |
| `mod/control` | every admitted load | a control command's acknowledgement and final outcome | [control](#control) |
| `mirror/events` | a load whose hello answer said `mirror` | batches of the conversation's events for the face; 404 for a run not drawn, 409 for another load | [the mirror](#the-mirror) |
| `session/closing` | every load, at `session.end` with an exit reason | the harness has begun to end its process; a main in rewake's shell has its closing screen started by it | [shell.md](shell.md#who-owns-which-pty) |
| `status/line` | `rewake statusline`, the status line of every run with a mod | the prompt cache's life, effort, the model's name, thinking; not an endpoint of the mod | [the cache's life](#the-cache-ttl) |
| `ask/open`, `ask/poll`, `ask/settle` | a worker's mod | a permission request or a question for main, held by polls, ended with the first answer | [permission-requests.md](permission-requests.md) |

**One description.** The protocol is one JSON schema,
`internal/adapter/claude/mod/protocol.schema.json`. A small generator of rewake's own,
`tools/protogen`, reads the subset the schema uses — objects, strings, integers, booleans,
arrays, enums, references — and writes the TypeScript types the mod imports
(`protocol.gen.ts`) and the Go types `modhost` decodes into (`protocol_gen.go`). A test
regenerates both in memory and fails when a checked-in file differs, so neither side can drift
from the schema. An additive field is a change of the protocol all the same: a newer mod's field
is refused by an older host's reader, a case one build per room never meets.
The reviewer's acceptance of an added field checks the direction that does happen: a newer build
reading what an older one left on disk, a missing field read as before. The fields added through
October 26, 2026 — the cache telemetry and `cacheLife`, `status/line`, `worker.type`,
`configuredModelName`, `worker.label`, `thinking` — were accepted so on October 28, 2026.

### The peer

Every request to the endpoint is checked on its own: the peer of its connection, by
`SO_PEERCRED`, must be **the harness process itself** — the pid the wrapper started, alive with
its start time before and after the answer (SP4). A descendant of the harness is never
accepted, since a command the model runs is one. No secret, token or session carries authority
from one request to the next, so a mod reload, a respawned hooks worker or a dropped connection
changes nothing: each request proves its origin again. A secret the host handed the mod would
not do: after the first load a shell command may already run, and on a reload it could read and
present a reissued secret first. The threat is a model's command imitating the mod; a process
of the same user rewriting the mod's files is outside it.

That `$.http.fetch` connects from the harness's own process was probed positive on 2.1.296 and
holds through a reload and the mod's polls
([the record](research-claude-mod.md#p4-the-peer-of-the-mods-requests)); a hooks worker
respawned after a crash and a shell child started before a reload are not probed. The check has
no fallback: were it to fail on a later version, every capability that reaches the host through
the mod would not be live, and a role that reports would refuse to launch.

`status/line` is the one exception: its peer may be any descendant of the harness
(`fromHarnessOrChild`), since the status line is Claude Code's child and its payload is
telemetry acted on nowhere — a shell command posting it is harmless.

A launch through `--command` keeps the check as it is: equality with the process the wrapper
started, not ancestry. A wrapper script that ends in `exec claude …` keeps its pid and start time
across the exec and is admitted; one that runs Claude Code as its child never is: the run starts,
its boundary is never live and it takes no tasks. rewake cannot tell the two apart before the
start, so that is a documented limit, not a refusal.

## The tools

The mod registers no mail tools: a session reads and sends through the shell commands. Its tools
answer with rewake's own `rewake worktree …` command, run by argv through the room's own copy of
its build (`REWAKE_EXECUTABLE`, [rules/state.md](rules/state.md#the-rooms-own-copy)):

- `worktree_status` on every run: the checkout the session works in and what a space's close
  would do with it;
- `worktree_list` and `worktree_land` on a run whose `REWAKE_ROLE` is `main` — a choice of what
  is shown, not an authorization;
- `worktree_enter` on a worker whose template says `enter_worktree = true`
  (`REWAKE_WORKTREE_ENTER`), with the model's own `EnterWorktree` refused there.

Their contracts, shared with pi's extension, are in [worktree-tools.md](worktree-tools.md).

## The turn boundary

| Event | The mod does | The host does |
|---|---|---|
| `turn.start` (main loop only) | sends `turn/start` once | records the start with no time ([the start time](claude-turns.md#the-start-time)) |
| `turn.complete` (main loop only) | sends `turn/end` under a fresh end id until its receipt; `answer`, `aborted`, `refusal`, `error` mapped to answered, aborted, refused, failed | the turn end operation, every retry handed on as its first capture |

An event that carries `agentId` is a subagent's and is not sent. An interrupted turn gives
`turn.complete` with `aborted`. `classic.StopFailure` is not hooked: `turn.complete` with
`error` is the failed end. `classic.Stop` with `block`, which would hold an end, is not used,
so Claude Code does not hold an interim end (O1). The design is
[claude-turns.md](claude-turns.md).

A `/clear` or a resume ends the conversation and continues the process under a new session id,
without a new `session.start`. The mod reads `$.session.id()` at each event, so the turns after
it are reported under the new conversation; the run, its spaces and its mail go on.

## Telemetry

The mod samples native context and the configured model after its hello, on `session.measure`
(which comes only at a turn's end), at the main turn's start and end, after each main-loop
`turn.step`'s response, at compaction boundaries, and after each heartbeat answer, so a long
turn's fill grows as it runs. It sends complete samples through `mod/state`, with the load,
heartbeat generation, selected conversation and a load-local sequence. The host accepts only its
current live generation and newer samples; unknown tokens clear an older reading. It hands each
sample, and each status line, to the session's observer (`core/session.Observer`), which merges
them into the published snapshot ([session-state.md](session-state.md#persistence-and-freshness));
the host keeps only the wire's order — load, generation, sequence — and the dialog it tells main
of. Nothing reads transcripts or measures a full context breakdown. The native model window and
percentage are used as given; a launch's `--autocompact` sets the window the conversation is kept
to — for a worker 272k unless a flag, a template or `REWAKE_CLAUDE_CONTEXT` says otherwise, and for main the model's own unless one of them names it
([launch-defaults.md](launch-defaults.md#the-context-window)) — and the observer recomputes the
percent against it.

The model's name is the status line's `model.display_name`, posted the same way
(`StatusLineRequest.modelName`) and kept across loads, since the mod has only the id
([research](research-claude-mod.md#the-models-name-and-thinking)); it follows `/model`, and
`rewake list` shows it, less a trailing parenthesis such as ` (1M context)` — the native window,
which the Context column shows as the run is held to it — over the template's `launch.label` and
the id. The session's thinking setting is the status line's `thinking.enabled`
(`StatusLineRequest.thinking`, `thinking` of the telemetry), kept the same way. It is the setting,
not what a request asked: no event of the mod carries a request's thinking. Known limit: Claude
Code tells it for every model, so a model without thinking reads as on.

Effort comes from two sources, and the observer shows the one whose value changed last: what the
latest main-loop `turn.step` asked for, and the status line's `effort.level`, which `rewake
statusline` posts beside the cache (`StatusLineRequest.effort`) and which follows `/effort` before
the next request. A sample repeats the last request's effort and the status line its setting on
every run, so a repeat changes nothing. Both are kept across loads; before either,
the launch's `--effort`, marked as the launch's (`reasoningEffortSource`), or unknown
([research](research-claude-mod.md#effort)). A worker's sample names what a dialog it holds for main asks
(`waiting`: `permission` or `question`), and a dialog it waits at that nobody is asked about,
which the host tells main of once
([permission-requests.md](permission-requests.md#a-dialog-nobody-is-asked-about)). Rate limits
are not reported. A lapse makes every field stale until a new generation supplies a sample.

**The prompt cache** is the usage the API reported for the latest main-loop `turn.step` response
(`usage.cache_read_input_tokens`, `cache_creation_input_tokens`, `input_tokens`): the mod sends
`cacheRead`, `cacheWrite` and `cacheInput` with every sample; a response with no usage and a
subagent's leave the last. The host publishes `Snapshot.Cache` (`cache` of the telemetry): the
raw counts and `hitPercent`, the share of all the request's input tokens — read, written and
neither — that the cache served. It is the last request's, not a running total, since that is
what says whether the prefix is still warm; providers count input differently, so the harness's
own usage is the only source and rewake adds nothing to it. A harness or an older mod that
reports none leaves the field out, and `rewake list` shows a `Cache` column only when some
session has it.

**Compactions.** Completed compactions have a load-local count; the observer folds its increases
into the run's count and bounded notification events. A reload keeps the run's count and marks
coverage partial. Native manual and automatic compactions are observed through the mod's event
hook, remote ones through the native call's result, so re-entry cannot count one twice. A skipped
or failed compaction clears the in-progress state and adds no completion. On a managed
installation a native compaction is not counted
([research](research-claude-mod.md#compactions-counted)).

## The cache ttl

The mod cannot read how long the prompt cache lives; Claude Code's status-line input carries it
as `prompt_cache` (`warm`, `ttl` of `5m` or `1h`, `expires_at` in epoch seconds, …) once a
request was made (read from the strings of 2.1.302). Every run with a mod therefore sets
`statusLine` in its `--settings` — `'<rewake>' statusline`, `refreshInterval` 30 or the person's
own — which beats every settings file but a managed one: a worker's in its overlay, main's in a
`--settings` of that key alone. Main has none where its launch
passes a `--settings` of its own, which a second would replace, or a `--setting-sources`, whose
layers rewake's reading of the person's line would not keep to — the launch says so in a note —
and none under `--command`. Claude Code merges the key field by field, so the person's `padding`
stands. The hidden `rewake statusline` reads that input, posts `ttl`, `expiresAt` and `warm`, the
effort, the model's name and `thinking.enabled` to the run's host at `status/line`, and prints
what the person's own `statusLine` command prints, run with the same input, or nothing, always
exiting 0. The person's line is the one Claude Code would draw without rewake's, reading only
`statusLine` of each file: the user's `settings.json` (`$CLAUDE_CONFIG_DIR`'s — the workers'
folder for a worker — else `~/.claude`'s), then the project's `.claude/settings.json`, then its
`.claude/settings.local.json` — at the repository's root, the main checkout's for a linked
worktree, as Claude Code keeps it since 2.1.310 — each later file's keys over the earlier's; the
project is the input's `workspace.project_dir`. A managed `statusLine` outranks rewake's, which then
does not run. The expiry is absolute, so a slow timer loses nothing; the command runs after
each response too.

The post (2 s) and the person's command (5 s) run side by side with deadlines of their own; the
person's command runs in a process group of its own with its output in a pipe rewake reads,
killed whole at the deadline, so it cannot keep the harness's output open, and
`REWAKE_STATUSLINE_PASSTHROUGH` in its environment makes a `rewake statusline` it calls print and
post nothing. The stdin read is bounded in time (3 s) as well as size.

The host keeps the last in `Snapshot.CacheLife` (`cacheLife`: `ttl`, `expiresAt`, `warm`), beside
`Cache`; `rewake list` shows the Cache cell as `90% 42m` (`—` once expired, `<1m`), the remaining
time counted by the reader's clock. Since October 26, 2026 main's run has the same status line,
so main's row gains its model's name, thinking, the cache's life and an effort that follows
`/effort` before the next request. pi has no observed
expiry; its extension reckons one into the same `cacheLife`
([research-pi.md](research-pi.md#telemetry-and-remote-control)).

## Control

Compact with an optional focus, and interrupt; clear is not offered; a drawn run also takes the
face's prompt ([the mirror](#the-mirror)). `control.Serve` reads the
run's control directory ([remote-control.md](remote-control.md)); the host queues the request for
the current load and generation and wakes its held `mod/alive` poll. An answer to the poll
carries at most one command, dispatched once; the poll chain renews at once while the command
runs separately (`management.ts`, outside the heartbeat's awaited chain, since a compaction may
take minutes). `mod/control` carries its first and final answers, matched to that dispatched id
and generation. These endpoints use the same peer checks as the hello; no journal or durable
outbox is added for control.

Compaction refuses a running main turn or another operation in flight. The mod answers
`requested`, passes the focus to native compaction, and later answers `done` with native
before/after counts when supplied, or a refusal or failure. The host keeps that final result in
the compaction outcomes for main's letter; it never sends the summary.

Interrupt names the running main turn and waits for its matching `aborted` completion. The
stopped report names the requester, and the mod appends that actor to native history without
starting a turn. A normally completed turn refuses the interrupt. A missing end after three
seconds gives an open failure; an end heard later still carries the actor.

The host waits five seconds for a control acknowledgement. A command not dispatched by then is
removed and refused; one already dispatched has an unknown outcome and is not repeated. An old
generation cannot take commands or update state, though its durable turn ends remain acceptable
under the outbox's own protocol. A reload or a failed final post can lose a compaction's outcome;
main asks again. Control is live only while the admitted hello's heartbeat holds and the run has
its control directory.

## The mirror

A run the face draws (a dev worker in the first slice, [face.md](face.md)) gets `mirror: true` in its
hello answer, and `mirror.ts` then passes its conversation to the wrapper's model in memory: the
prompts (`prompt.submit`), the main loop's text and thinking chunks as `turn.step` yields them, each
tool call with its input and result (`tool.call` observed through `next`), and the turn's state. It
reads no transcript file. A hook only appends to an outbox and returns; the `turn.step` generator yields
every chunk unchanged before queueing it. A sender posts the outbox every 50 ms, one batch of at most
48 KiB in flight with a 5 s deadline; a failed batch is dropped and counted. Each event is at most
40 KiB serialized, a block's settle comes in parts within that; what a step holds until it stops,
its open blocks and its tools' inputs together, is at most 1 MiB, each input 256 KiB, counted in UTF-8
bytes. The outbox is held to 256 KiB by dropping deltas first, then settles whole as `lost{id}`
markers, then collapsing the markers into one `resync` that counts what went. The wrapper's model
marks what was lost as a gap, never as settled. Subagents are not mirrored.

Each result of a run the face draws carries its **permit** (`permits.ts`, made only once the
hello's answer says `mirror`): `tool.check`'s hook reads the verdict of `next(e)` and returns it
unchanged; every record of a permit, in each hook, goes through `noting`, which loses a record that
fails, so a fault of the face's bookkeeping never changes, drops or holds up a verdict — seen
otherwise in review, when a failing record turned main's "Allow once" into the hook's refusal and
let a call main refused at the engine's dialog through (October 30, 2026; the kit runs the mod as a
module of its own with its built-ins frozen, so the paths were checked red and green with a throw
put into `permits.ts` by hand); an allow or a deny names a hook, a rule or the mode, an ask is settled by rewake's
permission request (`asks.ts`), the person at the dialog (`PermissionRequest` with no answer of
rewake's), or in auto mode by the classifier — `PermissionDenied`, a refusal's text, else any ask
answered without a dialog but an edit inside the session's directory (the `cwd` of the classic
events), which auto mode allows on its own. A refusal's reason carries the classifier's reason and
the link of the screen's line, cut within the 256 bytes of the model's short field. The permission mode is the `permission_mode` of `SessionStart`, `UserPromptSubmit` and
`PostToolUse`, sent as a `mode` event when it changes and once the hello is admitted; Claude Code
2.1.310 sends none with `SessionStart`, so the first prompt's event says it.

The face's prompt is one more control action, `prompt` with its `text`, carried by the `mod/alive`
poll; the mod runs it as `$.prompt.submit({text, asUser: true})` outside the poll's chain and answers
`done` once it is queued. The mod's own hooks do not see that submit, so the mod mirrors the prompt
itself when `$.prompt.submit` resolves: as taken in, or `refused` with the engine's reason when it
drops it, as it does after Esc or Stop until the person's next input.

## The band

Every Claude Code session rewake runs — main and every worker, whether or not the face draws it —
keeps its native interface and gets one row above the native input (`hooks/band.ts`, a `ui.render` hook
on `AbovePrompt`): the permission mode at its left in the terminal program's own name and colour
(`⏵⏵ auto mode`, `⏸ plan mode`, `⏵⏵ accept edits`, `⏵⏵ bypass permissions`, `⏵⏵ don't ask`, nothing
for the default), and at its right the session's rewake name (`REWAKE_SESSION`), ` · ` and the context
as `46k/272k` (`–/272k` before a count), in the theme's dim colour. It is the line the face draws as the
input's rule ([face.md](face.md#the-look)) with the same cuts — a narrow row drops the mode, then the
context, then cuts the name with `…` — so a native pane and a face pane tell the same
(`rowOf` repeats `shell/src/face/rule.ts`; the two are kept by hand, as the mod imports nothing of
the shell). It is information only: no button, nothing to answer.

Its facts are kept from other events, so the render hook waits for nothing: the mode from the
`permission_mode` of the classic events (the mirror's source: `UserPromptSubmit`, `PostToolUse`), so a
session shows none before its first prompt and a shift+tab shows at the next event — the `SessionMode`
site's `modes` was empty in all three modes tried on 2.1.310 (October 31, 2026), and no other event
or call names the live mode; the context from
`$.session.usage()`, sampled at `session.start`, after each main-loop step and at `session.measure`.
A change of either asks one redraw. A run without rewake's mod is untouched, and so is a surface that
does not draw the site (VS Code, `-p`); the person can fold the band, and it yields to a survey. A
hook that fails is dropped by the engine with its part, and nothing of the band touches a verdict, a
prompt or the hello.

Live, October 31, 2026 (brief 47; Claude Code 2.1.310, the shell from its sources under a pty of 200 by 50,
a private state directory and project, main on the cheapest model): the band read `⏵⏵ auto mode … main-claude ·
20k/1M` after the first prompt (before it, `main-claude · –/1M`: no mode yet, and the count not yet taken);
after shift+tab and a prompt each, nothing for the manual mode, `⏵⏵ accept edits` and `⏸ plan mode`, the
plan dialog hiding the band while it was up. Resized to 60 columns it read `⏸ plan mode … main-claude · 23k/1M`,
to 24 columns `main-claude` alone, the mode and then the context dropped.

## What the mod never does

It reads no transcript file. A run the face does not draw sends no prompt text
(`turn.start`'s text is not forwarded) and nothing of its conversation but the end's `answer`, which is
the report; a drawn run's conversation goes to its own wrapper in memory and nowhere else
([the mirror](#the-mirror)). It sets no status line — the launch does — sends no
settings, and
does no heavy work in a hook: the hooks worker is shared and a mod that stalls it is unloaded. It
draws nothing of the workers but the band above the input
([the band](#the-band)): rewake's shell draws them, on the person's terminal
([shell.md](shell.md)), and the mod knows nothing of who draws the screen.
