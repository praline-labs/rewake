---
description: pi's adapter: how it launches, wakes and hears turns, its wire contract, the bundled copy, and the probes behind each fact.
---

# pi: the adapter

Verified October 18, 2026 against installed `@earendil-works/pi-coding-agent` 1.2.0.
This package, not the older reference checkout, is the source for this adapter. Since
October 19, 2026 pi is shipped with rewake at a pinned version, **1.3.0** then and **1.4.0** since
October 27, 2026 ([below](#the-bundled-pi)), which a launch of the
shipped copy requires exactly ([below](#the-bundled-pi)); the minimum, **1.2.0**, holds a pi
named with `--command`, and the gate refuses older or unreadable versions. Main also compared the 1.3.0 package on October 18: its extension
declarations were byte-identical to 1.2.0 and its changelog contained no extension, event or
`sendMessage` change. It was read as compatible, not live-probed here. Later releases still
need that comparison and the probes below before acceptance.

## Launch and extension lifetime

Read in pi's package, not this repository — `pi-coding-agent/docs/cli.md`,
`pi-coding-agent/docs/extensions.md`, `pi-coding-agent/CHANGELOG.md` and
`pi-coding-agent/dist/core/extensions/types.d.ts` — on that version:

- `-ne` / `--no-extensions` disables discovered, configured and built-in extensions;
  explicitly passed `-e` / `--extension` entries still load. The adapter adds an embedded
  TypeScript entry and its management module, written only in the run directory. Runtime
  imports are Node built-ins and that sibling module; package imports supply types and
  need no installed dependency beside pi.
- `-na` / `--no-approve` ignores project-local resources requiring trust. It does not
  edit a trust record. The adapter passed both flags until October 19, 2026, when the
  bundled pi dropped them so the person's and the project's extensions load
  ([below](#the-bundled-pi)); `-e` alone adds rewake's entry to those pi finds.
- `-a` / `--approve` trusts project-local resources for that process: `parseArgs` sets
  `projectTrustOverride`, the last of `-a`/`-na` winning, and `resolveProjectTrusted`
  (`dist/core/project-trust.js`) returns it before any extension's `project_trust`, saved
  decision or `defaultProjectTrust`, writing nothing. The adapter adds it since October 19,
  2026 unless the launch names either flag. Read on 1.3.0; 1.2.0's `dist/cli/args.js` has
  both flags.
- `--append-system-prompt` is repeatable, each a text or the path of an existing file. The
  adapter appends its briefing even when the caller already supplied one. `--no-intro`
  suppresses only rewake's briefing.
- Any `--append-system-prompt` turns off pi's discovery of `APPEND_SYSTEM.md`, which is
  otherwise the project's `.pi/APPEND_SYSTEM.md` when the project is trusted, else the agent
  directory's (`dist/core/resource-loader.js`, around lines 480 and 955; read October 21, 2026
  on 1.3.0). The briefing's flag hid a project's file from every run rewake started; the
  adapter now passes that file's path after the briefing ([launch.md](launch.md#pi)).
- `--session-id` opens or creates the named conversation. The native CLI rejects it with
  `--session`, `--continue` or `--resume`. The adapter also rejects a fork or
  `--no-session` when rewake names a new conversation. The id does not add a user message.
  Rechecked October 18, 2026 on installed 1.2.0 in `pi-coding-agent/docs/cli.md`, `pi-coding-agent/docs/sessions.md` and
  `pi-coding-agent/dist/main.js` (`validateSessionIdFlags`, `createSessionManager`): an exact id searches
  the current project's session directory and creates the session if absent. Thus a space's
  fresh random `--session-id` must not be classified as continuation beside `--worktree`;
  that mistaken classification caused the pi worker's exit 2. Continue, resume, session and
  fork flags still refuse a new checkout. Session storage can be shared through
  `--session-dir`, the environment or native settings; the fresh random id, not an
  assumption about that storage, keeps a space independent of existing conversations. These are source checks, not
  a new live acceptance run.
- `--model`, `--models`, and `--thinking` name model selection and effort. Thinking levels
  are `off`, `minimal`, `low`, `medium`, `high`, `xhigh`, `max`; a model selector can carry
  a thinking suffix. Defaults from `REWAKE_PI_MODEL` and `REWAKE_PI_EFFORT` yield to those
  explicit choices. They do not inspect or edit pi's own settings.
- Extension registration happens in its factory; connections start in `session_start`,
  when `ctx.sessionManager.getSessionId()` names the active conversation. Shutdown closes
  that connection. The handlers stay registered across the session's lifetime.

Read in `dist/bundle/cli.js`: the executable starts Node and loads the bundled runtime
in that same process. Seen in the private mock probe: the launched pid, extension's
`process.pid`, and unix socket's `SO_PEERCRED` pid were identical. A shell wrapper that
spawns pi as a child therefore fails the peer check; it must `exec` pi.

Seen in that probe: `/new` raises shutdown for the old session and two `session_start`
notifications for the new one. `/reload` raises shutdown and a fresh start in the same
pid and conversation. Connection startup must be idempotent; the shipped extension keeps
one opening promise and one ready connection per conversation.

## Wake and the end of a turn

Read in `dist/core/agent-session.js`, the extension declarations, and the installed
agent-core's `dist/agent-loop.js`:

- From 1.2.0, `sendMessage` requires `deliverAs`. The adapter uses
  `pi.sendMessage(message, {triggerTurn: true, deliverAs: "steer"})`: idle starts a run,
  busy queues a steering message for the next safe point after the tool batch.
- `sendMessage` returns no promise. Its return acknowledges native queue acceptance;
  an asynchronous model failure can follow it. The wire acknowledgment makes no claim
  that the model ran or read the inbox.
- `agent_start` handlers are awaited before the first model request. Automatic retries
  can produce more than one `agent_start` / `agent_end` pair. The extension groups them
  under one random turn id until `agent_settled`.
- `agent_end.messages` contains that run's messages. The adapter takes the last assistant
  message's text blocks and stop reason, without reading any transcript file.
- `agent_settled {aborted}` is emitted after automatic work finishes; its handlers are
  awaited, but it is a notification, not a hook that can hold the end open.
- The installed pi-ai `dist/types.d.ts` lists `pending`, `stop`, `length`, `toolUse`,
  `error`, `aborted`, `deferred`. In `dist/providers/faux.js`, `pending` is the streaming
  placeholder; `deferred` carries a handle to a result that is not yet available. Neither
  proves a completed answer. At settled, aborted wins; `stop` maps to answered; every
  other value, including no assistant message, maps to failed. There is no separate
  refusal classification in this first pass.

The extension reports no native boot-clock timestamp: Node's monotonic clock is not the
host's boot clock. The host's start acknowledgment precedes model work and supplies the
ordinary turn's lower bound. `TurnHandler.Started` receives zero, not an invented native
time, so it cannot prove that a pending mark belongs to an earlier turn whose end was
lost. This is a first-pass limit, as is the lack of held interim ends.

## What the mock probes observed

Private HOME, agent settings, working directory and loopback mock endpoint; no real model
and no personal credentials. The native RPC mode was a probe driver only; the adapter's
launch remains the normal interactive pi command.

The lifecycle probe recorded these results on 1.2.0:

1. An extension-injected idle notice produced `agent_start`, a final assistant message,
   `agent_end`, and one non-aborted `agent_settled`.
2. A notice injected while a bash tool slept appeared in the model request after that
   tool completed. The run produced one final settled event.
3. Abort during a delayed model stream produced an assistant with `stopReason: aborted`
   and `agent_settled.aborted: true`.
4. `/new` and `/reload` reopened the extension's socket; a notice after reload started a
   new run successfully. The process identity remained unchanged.

A second probe loaded the shipped `extension.ts` against a fake protocol peer and the
same mock model. It verified protocol hello, idle delivery and exact final text, a new
conversation's reconnect, busy steering, the aborted end mapping, and reload followed by
a successful wake. Interactive terminal rendering and delivery between real rewake sessions
are main's live acceptance, not claimed by these probes.

## Telemetry and remote control

Read on the same installed 1.2.0 in `dist/core/extensions/types.d.ts`,
`dist/core/extensions/runner.js`, `dist/core/agent-session.js` and
`dist/modes/interactive/interactive-mode.js`:

- `ctx.model` and `ctx.thinkingLevel` are live getters. `ctx.model.reasoning` says whether the
  model thinks at all (`supportsThinking()`), and a level of `off` is thinking off (read on the
  shipped 1.3.0, `pi-ai`'s `Model` and `agent-session.js`, October 26, 2026); the extension sends
  `thinking` from the two, none for a model without reasoning. `ctx.getContextUsage()` returns
  native token usage, context window and percent; model context window remains available
  before usage is known. Immediately after compaction, usage and percent are null until a
  later assistant usage observation. The adapter preserves those unknowns; it does not
  substitute the estimated compaction result or apply another harness's token baseline.
- State is pushed at turn and compaction boundaries and settings changes, and polled
  through that same extension once a second. A sample sequence orders pushes and poll
  answers. A disconnected connection or three seconds without a valid observation makes
  the cached sample stale. The wrapper's snapshot publisher never waits on a native call.
- `ctx.compact({customInstructions, onComplete, onError})` returns void. Native manual
  compaction calls abort before preparing its input; the adapter therefore refuses a busy
  session before calling it and holds notices pending while compaction is in progress.
  Disabling automatic compaction in native settings does not disable this manual command.
- `session_before_compact` proves a start; before it, native preparation can refuse with
  `Already compacted` or `Nothing to compact (session too small)`. These map to
  `nothing to compact`. A request with no observed start after one second returns
  `requested`; its final outcome still follows. Other failures report a fixed explanation,
  not an error that could contain conversation text.
- `session_compact` counts a completed manual or automatic compaction. The completion
  callback separately supplies `tokensBefore` and optional `estimatedTokensAfter` for a
  remote request's outcome. Native summary text is never sent to the wrapper. Counts are
  observed since wrapper start, deduplicated by event sequence on each connection; gaps
  make coverage partial. No missed event or outcome is recovered from history.
- `ctx.abort()` also returns void. Interrupt waits up to three seconds for the matching
  `agent_settled` with an aborted outcome; no settled event means an open failure, not
  proof that the turn stopped. The stopped report names its initiator. A native custom
  message, appended without starting a turn, leaves that same actor in the next turn's
  history. An idle session refuses interrupt as `no turn running`.

The prompt cache's lifetime, read on the bundled 1.3.0 on October 23, 2026. No provider tells pi
when its cache expires. A model's `promptCache` names the provider's best-effort lifetime in
seconds per retention tier, `short` and `long`, "a missing tier means the lifetime is unknown"
(`pi-ai/dist/providers/model-schema.js`; `pi-coding-agent/docs/models.md`, "Describe model input and caching": a
`models.json` model or a `modelOverrides` entry can set it). In 1.3.0's own catalogue
(`pi-ai/dist/providers/data/*.json`) only one provider's models name lifetimes, 300 s short and
3600 s long; every other model gives the hit rate alone. pi's own session requests name no
`cacheRetention`, so the tier is `long` when `PI_CACHE_RETENTION=long` and `short` otherwise, as
`getPromptCacheTtlMs` of `dist/core/cache-warmer.js` reads it (`dist/core/sdk.js` builds the
request without one; only compaction sends `none`). An assistant message's `timestamp` is when its
request started (`pi-ai/dist/types.d.ts`), which is when the provider read or wrote the cache. The
extension therefore reckons `expiresAt` as the last assistant message's `timestamp` plus the tier's
lifetime, where the model in use is the one that answered and names that tier; the host carries it
as `cacheLife` with `ttl` as `5m`, `1h` or the seconds, and `warm` while the reckoned expiry is
ahead. Known limit: pi's cache warming (`cacheWarming` in settings) re-sends a request to keep the
cache alive; in its default `streaming` mode it stops when the run settles, but in `idle` mode it
goes on up to 30 minutes, which the extension does not see — the cell then shows less time left
than there is.

**[live]** October 23, 2026, the branch's build, the bundled pi 1.3.0 through `REWAKE_PI` as main
in print mode, a private HOME (its agent directory holding only a `models.json`) and `REWAKE_DIR`
under `/tmp`, no login: a stand-in provider on `127.0.0.1` speaking `openai-completions`, its model
naming `promptCache` 300/3600, answered every request with 900 of 1000 prompt tokens cached and
first with a bash call running `rewake list` once a minute. The Cache cell read `90% 4m`, `90% 3m`,
`90% 2m`; `--json` had `cacheLife` `{"ttl":"5m","expiresAt":<the first request's start + 300 s>,
"warm":true}`. With `PI_CACHE_RETENTION=long` it read `90% 60m`; a model naming no `promptCache`,
`90%`. The listing through the shell's own reader and `stripParts` ended `· 90% 60m`.

A third mock probe loaded both shipped modules and verified on 1.2.0:

1. Protocol 2 hello and requested state exposed the selected model, thinking level,
   window and native usage; an empty conversation refused compact and interrupt.
2. An idle compaction accepted a focus in its native summary request, acknowledged
   start, emitted one completion and a final outcome with both token counts. A notice
   during that compaction was explicitly left pending. The native compactor made two
   summary requests; the focus appeared in the first.
3. Usage and percent became null after compaction, with the window still known. The
   next ordinary turn made usage known again.
4. Compact during a delayed model response was refused as `in a turn`. Interrupt of
   that response settled aborted, reported the actor, and put the actor's interruption
   trace in the next mock model request. That next turn completed normally.

The mock used synthetic usage, so the result proves the native counts were transported,
not that its summaries reduced context. Main owns interactive and real-room acceptance.

pi 1.2.0 has no built-in tool-permission gate and no model question tool: the extension
examples `question.ts` and `permission-gate.ts` are not loaded by default, and
`ui_prompt_start/end` carry a kind and a title, neither the choices nor a way to answer. So
its adapter carries no permission request to main
([permission-requests.md](permission-requests.md)).

## The adapter's wire contract

One JSON object per line, at most 4 MiB, over a private unix socket listened on by the
host. Each side numbers its requests independently with a positive `id`; only
`op: "answer"` resolves a waiter. The adapter checks `SO_PEERCRED` against the announced
harness pid and rechecks its process start time before accepting the first frame.

| Direction | Operation | Fields and acknowledgment |
|---|---|---|
| Extension to host | `hello` | `protocol: 2`, `thread`, initial `state`; success makes Wake, TurnBoundary and Telemetry live, plus Control when its directory exists and Spaces on a main's run |
| Extension to host | `closing` | `thread`; sent at `session_shutdown` whose `reason` is `"quit"`, before the link closes, waited for at most one second; a main's run in rewake's shell calls `LaunchRequest.Closing` (October 24, 2026) |
| Host to extension | `wake` | `thread`, `notice`; answer says `ok` and whether it was `steered`, or `pending` while compacting |
| Extension to host | `turn-start` | `thread`, `turn`; acknowledgment precedes model work |
| Extension to host | `turn-end` | `thread`, `turn`, `outcome`, `text` (the final text, or pi's error message when a failed turn has none); acknowledgment follows publication |
| Either | `state` | host asks with `thread`; extension answers or pushes `state` with sequence, activity, model, effort, tokens, window, percent, compacting and (from October 23, 2026, optional) `cache`: the last assistant message's `usage` as `read`, `write`, `input` (`cacheRead`, `cacheWrite`, `input`), kept from `message_end`, `null` before the first response, so an older extension leaves it out; with it, where the model names a lifetime for the request's tier, `ttl` (seconds) and `expiresAt` (epoch milliseconds), the host's `cacheLife` (October 23, 2026, optional) |
| Host to extension | `control` | `thread`, `action`, `request`, `from`, optional `focus`; answer carries outcome, reason, detail, open and token counts where known |
| Extension to host | `compacted` | `thread`, event `sequence`, optional remote `request` and `from`; acknowledgment follows counting |
| Extension to host | `compaction-result` | `thread`, `request`, `from`, outcome and optional token counts; acknowledgment follows retaining the outcome for main's letter |
| Either | `answer` | matching `id`, `ok` or operation-specific result, optional `error` |

Peer announcement, hello, wake and control wait at most five seconds. Each publication
attempt has its own five-second deadline, at most three attempts with 200 ms between them;
the extension waits twenty seconds for an end acknowledgment. The socket reader captures
the read boundary before launching publication separately. Retries reuse that captured
end and event id. Handlers and the control-directory server count among the host's workers
so a publication outlasting close retains its lease.

Disconnect withdraws the live capabilities. Reconnect backs off from 250 ms to eight
seconds while the session runs. A new connection replaces the old one; the old connection
cannot withdraw the new live set or finish a turn in another conversation. There is no
durable outbox, end replay after reconnect or transcript recovery. A notice whose
acknowledgment is lost fails without automatic resend. A control request with a lost
answer has an open outcome and is not repeated. Conversation changes are checked at
delivery, but this adapter does not reserve a conversation across inbox publication.

The current offer is Wake, TurnBoundary with Reports and AfterStart, Telemetry, Control
with compact, focus and interrupt, and Conversation through `--session-id`. HoldsEnd is
false; Limits are absent. Spaces are offered: a main's run proves them with the hello
(October 24, 2026).

`SessionShutdownEvent.reason` is `"quit" | "reload" | "new" | "resume" | "fork"`
(`dist/core/extensions/types.d.ts:637`), and `"quit"` is emitted only by
`AgentSessionRuntime.dispose()` (`dist/core/agent-session-runtime.js`), so only the process's
end sends `closing`. On an interactive quit — Ctrl+C, Ctrl+D, `/quit` — pi stops its TUI, leaving
the alternate screen, before that dispose; on SIGTERM or SIGHUP it disposes first
(`dist/modes/interactive/interactive-mode.js`, `shutdown`). Its Ctrl+Z (`app.suspend`) stops the
TUI and sends `SIGTSTP` to its group, as Claude Code does. Read on 1.3.0, October 24, 2026.

## Live acceptance on a real model (October 18, 2026)

Main's run of `.scratch/process/live-pi.sh` on the branch's build, pi 1.2.0 with the person's own
settings (provider `openai-codex`), a print-mode Claude Code main on haiku, a private
`REWAKE_DIR` (`.scratch/pi-evidence/live/`). **[live]** The boundary went live about 1 s after
the start; a question came back with the word from a file; after 45 s idle a question woke the
worker and was answered; main's status line showed the worker's context (`1% used / 272K`); an
interrupt from main during a 90 s shell sleep was taken, and the next question was answered;
`rewake compact worker-pi '<focus>'` was refused as "nothing to compact" while the conversation
was under pi's `keepRecentTokens` (20 000 by default), and after three reads of about 12 000
tokens it started, the next answer showed `compactions 1` with the context down from 15% to 6%,
and the word the focus named was kept. The compaction's letter goes to main's run, which a
print-mode main has already ended by then — the same trap as a report to it (`docs/traps.md`).

## Skills

**[the pinned 1.4.0 package, `dist/core/package-manager.js:2023-2064`, `dist/core/skills.js:238-240`,
October 28, 2026]** What a brief's skills are copied from when main is pi
([briefs.md](briefs.md#skills)). pi reads, in order: the project's `.pi/skills`, only for a
trusted project, which rewake's `--approve` makes it; `.agents/skills` in the working directory
and each parent up to the git root; the agent directory's `skills`, `$PI_CODING_AGENT_DIR/skills`
(rewake's `~/.config/rewake/pi/agent/skills` for a pi it starts), else `~/.pi/agent/skills`; and
the user's `~/.agents/skills`. It follows links, keeps the first skill of a name it finds, and
takes the name from `SKILL.md`'s front matter, else the folder; it also finds skills nested deeper.
A brief names a skill by its folder and looks only one level down. Reading is not gated: pi has no
permission prompts (`pi-coding-agent/docs/security.md:3` of the 1.4.0 package), so a pi worker reads a brief's copies with no rule.

## The bundled pi

Owner decision, October 19, 2026: pi is part of rewake, pinned, installed and updated with it,
its login and settings apart from the person's global pi, every feature working there. Read on
the 1.3.0 package (`pi-coding-agent/docs/cli.md`, `pi-coding-agent/docs/security.md`,
`pi-coding-agent/docs/settings.md`, `pi-coding-agent/dist/bundle`):

- `PI_CODING_AGENT_DIR` moves the whole agent directory: `auth.json` (written mode 0600 by
  `FileAuthStorageBackend`, `join(getAgentDir(), "auth.json")`), settings, models, extensions,
  skills, prompts, themes, `pi install` packages (its `npm/`), `trust.json` and sessions. It
  does not move the npm cache (`~/.npm`) a package install uses, ambient credentials such as
  provider keys in the environment, or a user-level `~/.agents/skills`.
- Adding to the bundled pi (owner decision, October 19, 2026): an extension is a file in
  `<agent dir>/extensions/`, or a package installed with `PI_CODING_AGENT_DIR=<agent dir> pi
  install npm:<package>`; a skill is a folder with a `SKILL.md` in `<agent dir>/skills/`. The
  launch makes `extensions/` when missing and touches nothing in it; since October 28, 2026 it
  makes no `skills/`, by the owner's decision ([briefs.md](briefs.md#skills)).
- `PI_SKIP_VERSION_CHECK=1` stops pi offering its own update; rewake updates the copy.
- A copy (fullscreen selection, `/copy`) goes through `copyToClipboard` (`dist/utils/clipboard.js`):
  on Linux `wl-copy` with `WAYLAND_DISPLAY`, `xclip`/`xsel` with `DISPLAY`; then on WSL OSC 52
  alone when `WT_SESSION` is set, else `wslpath` and `powershell.exe Set-Clipboard`; OSC 52
  (`ESC ] 52 ; c ; <base64> BEL`, at most 100,000 encoded bytes) only when nothing copied on a
  machine with no display, or always after the local write with `SSH_CONNECTION`, `SSH_CLIENT`,
  `MOSH_CONNECTION` or `PI_CLIPBOARD_OSC52=1`. Fullscreen turns on mouse modes 1000, 1002, 1003,
  1004 and 1006 and copies on select by default (`fullscreenCopyOnSelect`). Read on 1.3.0 and
  driven in a pty on October 19, 2026.
- `--version` is not state-free: `dist/main.js` builds its settings manager and HTTP setup from
  the agent directory's settings before it checks the flag, so `{"httpProxy":"not-a-url"}` there
  makes `node cli.js --version` exit 1. Every version read rewake makes gets the run's agent
  directory (found in review, October 19, 2026).
- `bin.pi` is `dist/bundle/cli.js`, and `engines.node` is `>=22.19.0`, which pi does not
  check. `node cli.js --version` prints `1.3.0` and writes nothing to HOME.
- Project trust: without `-a`/`-na`, an extension handling `project_trust` or a saved decision,
  `defaultProjectTrust` (`"ask"` by default) applies; the interactive prompt comes before any
  extension loads, the user-level and rewake's included. `defaultProjectTrust` is read only
  from the agent directory's `settings.json`, with no flag or variable, so the adapter passes
  `--approve` instead ([above](#launch-and-extension-lifetime)).

**[live]** October 19, 2026, the branch's build placed as npm places it, beside pi 1.3.0 in one
`node_modules`, a private HOME and `REWAKE_DIR` under `/tmp`, no login, worker
`--model openai-codex/gpt-5.3-codex-spark --thinking minimal`, main a print-mode Claude Code
with the person's own HOME for its login:

| Check | Seen |
|---|---|
| the shipped copy starts | pi's screen `v1.3.0`; boundary live within 2 s; the person's pi 1.2.0 on `PATH` not used |
| rewake's and a third-party extension | `[Extensions] extension.ts, third.ts`; the third-party one, in the isolated `extensions/`, wrote its marker on `session_start` |
| a `pi install` package | `pi install npm:pi-mcp-adapter` with `PI_CODING_AGENT_DIR` set put it in the isolated `npm/`; the launch listed `extension.ts, pi-mcp-adapter, third.ts`, and the adapter turned off the built-in MCP as it does under a plain pi |
| where pi writes | only under `~/.config/rewake/pi/agent`: `auth.json` (`{}`, 0600), `settings.json`, `models-store.json`, `sessions/`; no `~/.pi` |
| the directory's mode | a fresh HOME: the launch made `~/.config/rewake/pi/agent` 0700 (and the missing parents); one made before at 0755 is made 0700 |
| a foreign setting | `~/.pi/agent/settings.json` and an inherited `PI_CODING_AGENT_DIR` both holding `{"httpProxy":"not-a-url"}`, which makes a plain `--version` exit 1: the shipped copy and a `--command` pi both started; neither foreign directory was written |
| a turn without login | pi printed `Provider is not configured: openai-codex`; the turn ended `stopReason: error`, and main's inbox got an `error` report from the worker 0.2 s later, empty text. The question stayed open: the model never read its inbox, so the report did not answer it |
| a project with `.pi/extensions` | pi's "Trust project folder?" prompt; no extension loaded and the boundary did not go live within 60 s, until answered — the launch has added `--approve` since (below) |
| `REWAKE_PI` at the person's pi 1.2.0 | refused before the claim: `pi 1.2.0 was found where rewake's own copy should be; this rewake runs pi 1.3.0: reinstall rewake, or name another program with --command` |
| `--command` at that pi | started, with the note `pi 1.2.0 named with --command is not the 1.3.0 this rewake was checked with` |
| no `node` on `PATH` | `pi runs on Node.js 22.19.0 or later, and no node is on PATH` |
| the binary outside any `node_modules` | `the pi installed with rewake is not in any node_modules above <path>; reinstall rewake from npm, set REWAKE_PI to a pi 1.3.0 package, or name a pi with --command` |

A model's answer reaching main through the bundled pi needs a login in the isolated directory,
which the run had none of by decision: it is the owner's check after `/login`; the answer path
itself is the adapter's, live since October 18. `tools/release`'s dry run installed the entry
from its local registry with pi from the public one: `npm installs
@earendil-works/pi-coding-agent@1.3.0 beside the command, and it runs under node`.

**[live]** October 19, 2026, after `--approve`: an ordinary `rewake pi` on a pty, pi 1.3.0
through `REWAKE_PI`, a private HOME and `REWAKE_DIR` under `/tmp`, no login, in a project whose
`.pi/extensions/marker.ts` writes a file on `session_start`. No trust prompt; the project's
extension wrote its marker and the boundary was live 0.8 s after the start; nothing was written
to `trust.json` or `~/.pi`. The same build without the change showed "Trust project folder?" with
neither within 25 s. With `-na` typed, no prompt, the boundary live and the marker not written.

**Raised to 1.4.0, October 27, 2026.** The 1.3.0 and 1.4.0 packages were installed side by side
and compared. Byte-identical: `dist/core/extensions/types.d.ts` (the extension API rewake's
extension uses), `dist/cli/args.js`, `dist/main.js` (flags and the settings read before
`--version`), `dist/utils/clipboard.js`. What changed, none of it in rewake's contract: the
changelog's provider-attribution headers, summaries without a fixed output cap, codemode worker
queueing, model catalogue entries and defaults; `AgentSession` gained a private-field removal and
a method to omit a failed attempt; the extension loader also resolves `@earendil-works/pi-ai/models`;
`pi-tui` restores the terminal on an exit without teardown (mouse, focus, keyboard and alternate
screen modes), which does not change the modes it turns on; the clipboard helper
(`clipboard-command.js`) starts `wl-copy`/`xclip`/`xsel` detached so Ctrl+Z does not stop them,
which `docs/shell.md`'s copy path does not depend on.

**[live]** October 27, 2026, the branch's build with 1.4.0 beside it in one `node_modules`
(`REWAKE_SHELL` at an installed shell), a private HOME and a short `REWAKE_DIR` under `/tmp`, a
pty of 120 by 40, no login, `--model openai-codex/gpt-5.3-codex-spark --thinking minimal`:

| Check | Seen |
|---|---|
| the shipped copy starts | pi's screen `v1.4.0`, `[Extensions] extension.ts`, `rewake list` showed `main-pi`, the status line `idle · … minimal thinking` |
| `REWAKE_PI` at the 1.3.0 package | refused before the claim: `pi 1.3.0 was found where rewake's own copy should be; this rewake runs pi 1.4.0: reinstall rewake, or name another program with --command` |
| `--command` at the 1.3.0 `pi` | started, `v1.3.0` on its screen; the note (`… is not the 1.4.0 this rewake was checked with`, `internal/adapter/pi/launch.go`, held by `ship_test.go`) was not visible in the captured screen |

`go test ./internal/adapter/pi/... ./internal/host/launch/` and the extension's `bun test` (14
tests) pass. Not run, because they need a model and the owner's login: a turn's answer reaching
main through the bundled 1.4.0, and the telemetry's thinking level and `model.reasoning` on a real
model. `scripts/pack.sh 0.0.1` wrote `"@earendil-works/pi-coding-agent": "1.4.0"` into the entry's
dependencies.

## A mirror of a live session for an interface of rewake's own (October 28, 2026, 1.4.0)

The same questions as Claude Code's
([research-claude-mod.md](research-claude-mod.md#a-mirror-of-a-live-session-for-an-interface-of-rewakes-own-october-28-2026-21310)),
asked of the bundled pi 1.4.0 after reading its extension API (`docs/extensions.md`, `docs/tui.md`,
`dist/core/extensions/types.d.ts`). One interactive pi ran in a terminal tab of the write
session's own, in `/tmp/ui-probe-pi-36`, with `-e` naming a probe extension, `--no-session`, the
workers' agent folder as `PI_CODING_AGENT_DIR` (its settings untouched), an environment cleared to
HOME, PATH, TERM, LANG and the machine's proxy variables, the cheapest model at low thinking, four
prompts and two from the extension. The extension appended every event to a log with its own
`performance.now()` and held marked calls on 25-second long polls to a small server on a Unix
socket; its source, the server, the launch script, a log viewer and the evidence are kept in the
ignored `.scratch/ui-probe-pi/` (`probe.ts`, `server.ts`, `launch.sh`, `show.py`, `log.jsonl`,
`screens/`). Times are from the prompt's `input` event. **[live]**

- **Streaming.** Every piece of a reply reaches an extension as it streams: `message_update`
  carries the provider's `assistantMessageEvent`, typed `text_start`/`text_delta`/`text_end`,
  `thinking_*`, `toolcall_start`/`toolcall_delta`/`toolcall_end`, with the delta and the partial
  message. A reply of 1594 characters came as 327 `text_delta`s from 4.36 to 6.01 s, a median of
  5 characters (at most 13), in 70 bursts a median 20 ms apart (the longest gap 71 ms). The
  assistant's `message_start` came at 1.50 s, `thinking_start` at 2.27 s, `thinking_end` at
  4.36 s, `text_end` at 6.10 s and `message_end` at 6.11 s, carrying the whole message: its
  content blocks, `stopReason`, usage. Thinking streamed as text only when the provider sent a
  summary (one reply: 28 and 2 characters, drawn on screen as its summary line); otherwise the
  block was empty, a signature only. `provider_stream_event` fired 341 times for that reply.
  `message_start`/`message_end` fire for the system prompt, the user's message and every tool
  result too, and the prompt's text is also in `input` (with its `source`: `interactive`,
  `extension`, `rpc`). Tool arguments stream as `toolcall_delta`s of 2–4 characters (95 in the
  run); then `tool_execution_start` with the whole arguments, `tool_call` under a millisecond
  later, `tool_execution_update` with partial output, `tool_result`, `tool_execution_end` with
  the result, `isError` and `durationMs`, a `toolResult` message and `turn_end` with the
  results. Turns are `agent_start`, `turn_start`/`turn_end` per model step, `agent_end`, then
  `agent_settled` with `aborted`.
- **A long hold.** pi has no permission gate, but a `tool_call` handler is awaited without a
  time limit and returns `{block, reason}`. The probe held bash calls on the long polls: 60.0 s
  (3 polls) then allowed, 180.0 s (8 polls) then refused, 600.0 s (24 polls) then allowed; the
  handler's `ctx.signal` never aborted. An allowed call ran 17 ms after the answer; the refused one
  did not run, ended `isError` with the reason as its result, and the model went on to the next
  step. Meanwhile the screen showed the call's row (`$ echo probe-hold-10`) under pi's
  `⠧ Working` row, a free editor, the footer, and no dialog; the row then read `Took 0.0s` for
  an allowed call (timed from the answer) and `Took 3m 0s` for the refused one.
- **A question.** pi has no question tool of its own. The probe registered one (`ask_person`)
  whose `execute` shows pi's own selector, `ctx.ui.select(question, options, {signal})`, and
  races it with an answer from outside: the selector replaced the editor (`→ red`, `blue`,
  `↑↓ navigate  enter select  escape/ctrl+c cancel`) and the working row, `ui_prompt_start`
  fired with `kind: "select"` and the title only, and at 30.0 s the outside answer won; aborting
  the signal closed the selector, `ui_prompt_end` fired, and the model read "The person chose:
  Blue". Other extensions see a dialog only through those two events.
- **Input back.** `pi.sendUserMessage(text)` while idle, from a timer 500 ms after a command,
  started a turn: an `input` with `source: "extension"`, `before_agent_start` with `messageType:
  "user"`, `agent_start` 4 ms later; the screen drew it as a typed prompt, with no mark of its
  origin. With `deliverAs: "steer"` 2 s into a held call it drew `Steering: <text>` (`↳ Alt+Up
  to edit all queued messages`) and was delivered as a user message at the next `turn_start`, 2 ms
  after the call ended; `followUp`, by the docs, waits for the run to end. `ctx.abort()` 5 s into `sleep 30`
  stopped the call (`Command aborted`, `Took 5.0s`), then one empty assistant message with
  `stopReason: "error"` ("This operation was aborted"), `agent_end`, `agent_settled` with
  `aborted: true`.
- **The rest of the screen.** pi draws a header (logo, keys, the loaded extensions), the
  transcript, a working row (spinner and message), the editor and a footer (the cwd, tokens,
  cost, context use, model and thinking level); no event carries the spinner or the footer's
  numbers. An extension can redraw each of them: `setHeader` and `setFooter` replaced the header
  and footer with one-line components (the footer's statuses went with it), `setWidget` drew a
  line above the editor, `setWorkingVisible(false)` hid the working row during a whole turn, and
  `setStatus`, `setWorkingMessage`, `setWorkingIndicator`, `setTitle`, `setEditorComponent`,
  `custom()` overlays and per-tool and per-message renderers are in the API. Running the command
  again restored pi's own. Usage and context are in each assistant message's `usage` and through
  `ctx.getContextUsage()`.
- **Not tested.** `setStatus` under pi's own footer, a person answering or pressing Esc during a
  hold, `followUp`, a hold past ten minutes, a run under load, RPC mode, which by its docs
  (`docs/rpc.md`) gives a child process the same session events as JSON lines.

| What the interface needs | Claude Code 2.1.310 (a mod) | pi 1.4.0 (an extension) |
|---|---|---|
| the prompt | `prompt.submit`, `UserMessage` render | `input`, the user `message_end` |
| streamed text | `turn.step` chunks, ~9 characters 20 ms apart | `message_update` `text_delta`, ~5 characters in bursts 20 ms apart |
| a settled block | `ui.render` `AssistantMessage` | `text_end`, `message_end` |
| thinking | empty `thinking` chunks | `thinking_delta` when the provider sends a summary |
| a tool call's input | `ToolGroup` (folded), `ToolUse` (unfolded), events | `toolcall_delta`, `tool_execution_start` |
| a tool's output | `ToolResult` (unfolded), events | `tool_execution_update`/`_end`, the `toolResult` message |
| holding a call | `tool.check`, 10 min seen, no dialog | `tool_call`, 10 min seen, no gate to hide |
| a question | the model's `AskUserQuestion`, its render site | an extension's own tool on `ctx.ui.select`, closed by a signal |
| a turn as if typed | `$.prompt.submit` from a timer, marked as the plugin's | `pi.sendUserMessage`, unmarked |
| steering | (not probed) | `sendUserMessage(…, {deliverAs: "steer"})` |
| abort | (not probed) | `ctx.abort()` |
| the rest of the screen | `Spinner` render on mode changes; status, toast, band, pane | header, footer, widgets, working row: replaced or hidden |

So an adapter for pi takes everything from events, the text per delta, with nothing to unfold
and no render site to read; it can hold a call at `tool_call` for at least ten minutes, ask
the person through pi's own selector and close it from outside, start, steer and abort a turn,
and hide or replace pi's own working row, header and footer.
