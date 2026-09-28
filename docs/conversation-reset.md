# Resetting a worker's conversation, and marking a session's conversation

**Design, not built; open choices at the end.** The owner asked for two things on September 28, 2026:

- **`rewake clear <session>`**: main resets a worker to a clean conversation at a moment of
  its choosing — what `/clear` or `/new` typed in the worker's window does — the way
  `rewake compact` and `rewake interrupt` act on a worker today
  ([remote-control.md](remote-control.md)).
- **A conversation marker** in `rewake list`, its `--json`, main's notices and
  `inbox --awaited`, so main sees at once that a session's conversation is new, cleared or
  resumed. Main took a fresh conversation for an old one: `list` shows only the wrapper's
  age.

The facts below were checked on September 29, 2026 against the installed Claude Code
2.1.280 (its bundled source, by the minified names), the older Claude Code tree kept as
reference (`openagent`), the Codex tree at the release commits `be2951ea3` (0.155.1) and
`36650394c` (0.157.1), and rewake at `afd38ff`. Codex paths are under `codex-rs/`, lines
0.157.1's unless both are given. Nothing was run; what only a run can settle is in
[the probe](#the-probe).

## Claude Code: the facts

**The handle.** The function-hooks plugin rewake already loads
([remote-control.md](remote-control.md)) can run a slash command:
`$.command.run({ command, args? })`.

- Its check refuses a call from a hook that holds the turn — "it would wait on the turn
  this hook is holding" — and a malformed argument: "takes { command, args? } (the name
  without the slash)". A call from a `$.clock` callback, where rewake's module polls its
  control directory, holds no turn.
- It rejects a name the session does not know: "no command named /<x> in this session".
- It queues `/<command>` as a prompt with priority `later` and origin `plugin`
  (`DBn`, `Gke="later"`).
- Its promise resolves once the command has run, with `{ text, context? }` — the command's
  result text and meta texts. What `/clear`'s text is was not read.
- It rejects with one of four texts: "the command was removed from the queue before it
  ran", "the command left the queue as text for the model; it did not run", "the turn
  was interrupted before the command ran", and "the command did not go through
  command.run (…)".
- `$.prompt.submit` of a text starting with `/` is refused with "run one with
  $.command.run({ command })", and `$.session.id` is in the plugin's table, so the module
  can read the conversation's id before and after.

None of this is in the older tree: the plugin host is newer than it.

**`/clear` itself.** One command, `type:"local"`, aliases `reset` and `new`, an optional
`[name]`: "Start a new session with empty context; previous session stays on disk
(resumable with /resume)". So on Claude Code `/new` is `/clear`. Its body (`H4n` in
2.1.280, `commands/clear/conversation.ts` in the older tree), in order:

1. SessionEnd hooks with reason `clear`. Their budget: the older tree 1.5 s by default;
   2.1.280 takes the longest SessionEnd hook's own timeout, at least 1.5 s and at most
   60 s (`Hle`), unless `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS` is set. rewake registers
   its SessionEnd hook with a timeout of 5 s (`internal/harness/claude/settings.go`), so
   under rewake the budget is 5 s. The plugin's `session.end` fires too, with the reason.
2. The queue loses the harness's own passive notices and task notifications. rewake's
   notices are not among them: the socket queues a line with priority `next`,
   `skipSlashCommands` and no `passive` mark, so a notice waiting at the moment of the
   clear goes into the new conversation. A `/clear` sent over the socket reaches the model
   as text, and the socket's control actions offer no clear.
3. The messages are dropped; foreground tasks are killed and background ones kept; the
   working directory returns to the launch directory; file-read state and session
   environment variables are cleared; a listed set of state keys is reset (`xFt`). The
   permission context is not in that set, so the permission mode, rules, `--add-dir`
   directories and directories a hook added for the session should outlive the clear —
   read, not seen.
4. A new session id, with the old one as its parent; the old transcript stays on disk.
5. SessionStart hooks with source `clear`.

The plugin's `session.start` is raised once, when the terminal's session controller
mounts (`_raiseSessionStart`), not by `/clear`: the module is not reloaded, and its
timers keep running — read, not seen.

**Seen live before** (2.1.280, September 23, 2026,
[research-claude-control.md](research-claude-control.md#interrupting-a-turn-and-changing-the-conversation)):
SessionEnd `clear` with the old id, then SessionStart `clear` with a new one and no
`model`; the wrapper's socket survived, and a delivery and its Stop worked after it.

**What a conversation's start tells rewake.** SessionStart's `source` is `startup`,
`resume`, `clear`, `compact` or — new in 2.1.280 — `fork`. A resume or fork adds
`seconds_since_last_response` and the context size of the resumed transcript. SessionEnd's
`reason` is `clear`, `resume`, `logout`, `prompt_input_exit` or `other`. So rewake can tell
a new conversation from a cleared, resumed or forked one; `compact` keeps the conversation.

**What rewake does with it today.**

- The collector's conversation is the last `session_id` any event carried
  (`internal/harness/claude/telemetry/state.go`), published as `primaryThread`.
- It decodes `source` only to leave the activity alone on `compact`, and does not decode
  SessionEnd's `reason` at all (`decode.go`).
- The count of compactions is the run's, not the conversation's.
- `rewake list` shows the wrapper's age; `--json` shows `primaryThread` to main only.

## Codex: the facts

**`/new` and `/clear` in the terminal.** Both start a thread with `thread/start` and then
replace the terminal's own chat view and primary thread
(`tui/src/app/session_lifecycle.rs:1069`, `:1012` in 0.155.1). `/clear` also clears the
screen and sends `sessionStartSource: "clear"` (`tui/src/app/event_dispatch.rs:351-390`);
`/new` and the start at launch send none (`event_dispatch.rs:133`,
`tui/src/app_server_session.rs:1679-1684`).
The field has two values, `startup` and `clear` (`app-server-protocol/src/protocol/v2/thread.rs:50`).
Both commands are refused while a task runs (`tui/src/slash_command.rs:241`, `:260`;
`:208`, `:226` in 0.155.1), seen live for `/new` on 0.155.1
([research-codex-live-checks.md](research-codex-live-checks.md)).

**Nothing else can make the terminal switch.** The terminal changes its thread only
inside the handling of its own request's reply. A `thread/started` for a thread it does
not know is dropped once it has a primary thread (`tui/src/app/app_server_events.rs:417-437`).
No server request asks the terminal to switch or clear (`server_request_definitions!`
in `app-server-protocol/src/protocol/common.rs`). So a wrapper could start a thread, but
not make it the one the terminal shows and reads into. Forging a reply to a request the
terminal did not send is outside rewake's rules: it acts only through handles a harness
offers.

**`thread/revert` is not a way either.** It exists in both releases (`thread/rollback` in
0.155.1 only). It keeps the thread's id and replaces the persisted history of a paginated
thread with the part before a given turn, reloading the thread
(`app-server/src/request_processors/thread_processor.rs:2111`). The terminal refreshes its
view only after its own revert (`event_dispatch.rs:680-735`); a `thread/reverted` it did
not ask for is ignored (`tui/src/chatwidget/protocol.rs:348`), so the screen would go on
showing a conversation the model no longer has.

**What a conversation's start tells rewake.** The gateway sits between the terminal and
the server and sees the terminal's own requests
(`internal/harness/codex/gateway/state.go`). It binds a conversation only on a successful
reply to a request it recognized as the terminal's selection, and a generation counter
keeps an old reply from binding after a newer selection. From the request it can tell:

- a start at launch — the request id starts with `startup-thread-start-`;
- a later `/new` — another `thread/start`;
- a `/clear` — `thread/start` with `sessionStartSource: "clear"`, which the gateway does
  not read yet (`gateway/metadata.go`);
- a resume — `thread/resume`, apart from a reconnect to the same thread;
- a fork — `thread/fork` (`gateway/fork.go`).

The reply's thread carries `createdAt`, in Unix seconds, in both releases
(`app-server-protocol/src/protocol/v2/thread_data.rs:252`). The gateway's `fresh` means
"started here and has run no turn", used to refuse an empty compaction; the snapshot's
`fresh` means the observation is recent. Neither is a conversation marker.

## What a change of conversation leaves in rewake

The same on both harnesses unless said.

- **Owed tasks stay owed.** They belong to the run, not the conversation. The next report
  settles them, marked `threadChanged` with the line "the reader's thread changed after
  delivery" (`internal/inbox/thread.go`, `internal/cli/turn_reports.go`). On Codex seen live
  on 0.155.1 ([record](roadmap/2026-09-26-live-checks.md)); on Claude Code it passes the
  fixture case only (HF-10 in [harness-features.md](harness-features.md)). The model in
  the new conversation does not know the task, so that report answers other work.
- **Grants.** A grant records the conversation it went into
  (`internal/grantauth/grantauth.go`), and a resume restores it only into that one
  (`internal/grantauth/resume.go`). A grant made before a clear still works in the same
  run on Claude Code, since the hook asks the keeper by run. A cold resume of the
  conversation after the clear would not restore it. On Codex a grant's roots belong to
  the thread (`internal/harness/codex/server_dirgrant.go`).
- **A compaction's hold** on Codex belongs to the thread it was tied to
  (`internal/harness/codex/gateway/mark.go`).
- **`inbox --awaited`** shows neither the conversation a task went into nor the current
  one (`internal/cli/inbox_awaited.go`).

## The conversation marker

**The model.** The snapshot gains `conversation`. Only main sees it, like the rest of
the telemetry. `primaryThread` stays as it is.

| Field | Meaning | Claude Code | Codex |
| --- | --- | --- | --- |
| `id` | the conversation now | the last `session_id` | the bound thread |
| `began` | how it came: `started`, `new`, `cleared`, `resumed`, `forked`, `unknown` | SessionStart `source`; `startup` is `started` | the recognized request, as above |
| `since` | when rewake first saw it in this run | the event's time | the binding's time |
| `by` | the main whose `rewake clear` made it | the module's word | — |
| `createdAt` | when the conversation was created | — | the reply's `createdAt` |
| `quietFor` | for a resume, seconds since its last response | `seconds_since_last_response` | — |
| `turns` | turns this run saw in it | own `turn.start` | `turn/started` on the thread |
| `compactions` | compactions this run saw in it | as now, reset on a change | as now, reset on a change |
| `changes` | conversation changes in this run | counted | counted |

`turns` counts only what this run saw. For `resumed` or `forked` it is not the
conversation's history, and the text says "seen". A reconnect to the same Codex thread is
not a change. On Claude Code a SessionStart with source `compact` keeps the conversation,
and `/new` is always `cleared`, since it is the same command.

**`rewake list`.** The main's view adds a `Conversation` column after `Compactions`:
`new 4m · 3 turns`, `cleared 2m by main-claude · 0 turns`,
`resumed 1h · 2 turns seen`, `unknown` when nothing has been heard. `Age` stays the
wrapper's age.

**Main's notices.** The observer that announces compactions and departures
(`internal/wrap/session_notices.go`) announces a change after main's start. The snapshot
keeps `conversationEvents` beside `compactionEvents`:

- `Rewake: review-claude started a new conversation (cleared at its keyboard).`
- `Rewake: review-claude resumed conversation 8c1f02ab.`
- When the worker still owes this main tasks delivered in the earlier conversation:
  `It still owes you 1737…-a1, delivered in the conversation before; resend it if it
  still matters.`

A change main asked for with `rewake clear` gets no notice, as for `rewake compact`: the
command's answer says it. The conversation a worker starts in comes with the
availability notice, so main learns at the first sight whether the worker is fresh or
resumed.

**`inbox --awaited`.** Each message gains `deliveredIn` — the conversation id and the
`changes` count at delivery, kept beside the thread record — and `conversationChanged`
when either differs now. The count catches an A–B–A a comparison of ids misses. The line
adds "delivered in an earlier conversation (cleared 2m ago)". Nothing is resent or
settled on its own.

## `rewake clear <session>`

```
rewake clear <session> [--json]
```

In the command table under "STEER A SESSION". The call is checked whole before anything
is written, as `compact` and `interrupt` are: only main may call it; the target is a
running session of the same room, not the caller; its harness can clear. Each of these
is exit 2 and writes nothing.

**On Codex** the harness cannot clear, so the call is refused at that check with exit 2
and the next step: start a new worker with `rewake codex`, or ask the owner to type
`/new` in the worker's terminal. No thread is created first. Once Codex offers a way to
switch its terminal, the same contract applies there: idle only, and success only once
the new thread is started and confirmed selected.

**On Claude Code** the request goes through the control directory, and the module gains
an action `clear`:

1. Refuse `in a turn` when its own turn is running, like a compaction.
2. Otherwise tell rewake `clear.asked` with the request and the caller, and wait until
   it is sent, as `compact.asked` is.
3. Read `$.session.id()`, call `$.command.run({ command: "clear" })`, read the id again.
4. Answer `done` with the old and new ids, or refused or failed with the host's text.

The wait: pickup 5 s, as for the others. The outcome within 15 s: the SessionEnd budget
is 5 s under rewake, and SessionStart hooks follow.

| Outcome | Exit | Meaning |
| --- | --- | --- |
| `done` | 0 | the conversation is new; the answer names both ids |
| `refused`, `in a turn` | 1 | the worker is in a turn: interrupt it first, or wait |
| `refused`, `owes a task` | 1 | the worker owes a task to another session; the answer names the ids and senders |
| `refused`, `not answering`, `cut short`, `no control directory`, `withdrawn before it was taken`, `another request in flight` | 1 | as for `compact` |
| `failed` | 1 | the host rejected the command, with its text; `open` when no outcome came in time |

**Owed tasks.** Before it sends anything, the command reads what the worker owes, with
the same records `inbox --owed` reads. A task owed to another session refuses the clear:
that session waits for a report which would then come from unrelated work. Tasks owed
to the caller are closed on `done`: the worker's wrapper, which hears the module's
`clear.asked` and the SessionStart `clear` that follows, writes a `stopped` report —
"main-claude cleared the conversation with rewake clear; the task was not finished" —
as it writes one after an interrupt, so main's `--awaited` is right at once.

**Grants.** A task closed as stopped gives its grant back the usual way
([grants.md](grants.md)). The directory itself stays in the session's permission
context after the clear, until that return takes it.

**Deliveries.** From `clear.asked` until the answer, the worker's wrapper holds new
notices as not yet deliverable, the way a grant waits for an idle session
(`internal/wrap/grants.go`). The hold is bounded by the command's own limit, so a lost
answer holds nothing for long. A notice queued in the harness before that goes first:
its priority `next` runs ahead of the clear's `later`.

**What main sees.** `cleared review-claude: conversation 2f9c01aa, before 8c1f02ab`, and
in `rewake list` `cleared 0m by main-claude`. The old conversation stays on disk and can
be reopened with `/resume`.

## Races

- **A notice in the instant of the check.** The module sees no turn, and a notice already
  in the harness's queue starts one before the clear runs. The clear then runs after that
  turn. The turn's report goes out at its Stop, before the clear, so nothing is lost; but
  a task that turn left `pending` is carried into the new conversation with nobody knowing
  it. The module reports a turn that started between its check and the clear in the
  answer (`after turn <id>`), and main decides.
- **A clear typed at the keyboard** while main's request is on the way: two SessionStart
  `clear` events. The module's ids tell which is main's. The marker names `by` only on
  the id the module answered with.
- **An interrupt, then a clear.** `rewake interrupt` returns once the turn is aborted.
  The turn's end reaches the module as `turn.complete` a moment later. A clear sent in
  between is refused `in a turn`, and main tries again.
- **A report after the clear.** A Stop hook of the turn before the clear, arriving late,
  still carries the old `session_id`. The collector orders events by their start time,
  so it does not move the conversation back.

## The probe

The smallest run that settles what the sources leave open costs no model. Use a private
HOME and state directory in `/tmp`, a stand-in API on a local port as in the earlier
probes, and `CLAUDE_CODE_ENABLE_FUNCTION_HOOKS=1`. Launch with a settings layer that logs
SessionStart, SessionEnd and Stop, a plugin directory with a module that polls a file
from `$.clock.every`, and `--add-dir` to an extra directory. Then:

1. Idle: `$.command.run({command:"clear"})` resolves — record its value — the hooks log
   `clear`, the id changes, and a second request later works, so timers survive.
2. Whether a `/clear` run this way raises `turn.start` and `turn.complete`. If it does,
   the module's `in a turn` check and the collector's activity must ignore that turn.
3. Mid-turn (the stand-in answers after 10 s): the clear runs after the turn; the
   promise's timing.
4. After the clear, a write into the `--add-dir` directory, and into one a hook added
   before it, go through without a prompt.
5. A socket line sent during a turn before the clear reaches the first request of the
   new conversation.

On Codex, one idle run of the terminal through a test gateway, with no turn and so no
quota: the terminal's `/clear` request carries `sessionStartSource: "clear"`, `/new`
none, and the launch's id starts with `startup-thread-start-`.

## Owner choices still open

1. **Tasks owed to the caller** on a clear: closed as `stopped` at once, or left owed
   to be settled by the next report with `threadChanged`. Recommended: stopped.
2. **Tasks owed to other sessions**: refuse the clear, or clear and close them as
   stopped with a word to their senders. Recommended: refuse. Main can see who waits
   and settle it first.
3. **Codex until it can switch its terminal**: ship `rewake clear` for Claude Code now,
   with an honest refusal on Codex, or wait for parity. The owner decided on
   September 24, 2026 that remote control behaves the same on both harnesses.
   Recommended: ship with the refusal. The refusal is the same predictable answer every
   time, and the marker, which is the fix for what went wrong, works on both.
4. **A notice for a change main did not ask for**: yes for a clear, new or resume at the
   worker's keyboard, and the first conversation in the availability notice.
   Recommended as written.
5. **Holding deliveries during a clear**: hold, bounded by the command's limit, or let
   notices go into whichever conversation takes them. Recommended: hold.
