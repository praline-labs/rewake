# Remote control: `rewake compact` and `rewake interrupt`

A main session acts on a running worker without a person at its keyboard: it compacts
the worker's conversation, or interrupts the worker's turn. It was built as stage 2 of
rewake's function-hooks plugin for Claude Code ([claude-plugin.md](archive/1.x/claude-plugin.md)
is stage 1, which only listened). What the harness offered for it, for Codex, is in
[research-protocol.md](archive/research-protocol.md#compaction-and-interrupt-on-request).

Built on September 24, 2026 for Claude Code, with unit and module tests and a workflow
case, and the same day for Codex (part B), where the wrapper serves the requests itself
([remote-control-codex.md](archive/1.x/remote-control-codex.md)).

**The same directory carries `rewake answer`** since October 18, 2026: main's answer to a
Claude Code worker's permission request or question, the action `answer`, which the worker's
wrapper reads against the open request and gives its mod
([permission-requests.md](permission-requests.md#the-path)).

**pi serves both verbs since October 18, 2026**, including compaction focus, through its
per-run extension and native APIs ([research-pi.md](research-pi.md#telemetry-and-remote-control)).
The wrapper serves the control directory with `control.Serve` and carries each request
over the extension socket. Manual compaction refuses an active turn; it answers `started`
when the native preparation hook is seen or `requested` after one second, and records the
final result with native before/estimated-after counts for main's letter. Automatic
compaction being disabled does not disable this manual request. Interrupt waits for the
matching aborted settled event, names the initiator in the stopped report, and appends
that actor to native history without starting a turn. A missing acknowledgment leaves an
open outcome and is not repeated.

**Claude Code serves both verbs through the mod again since October 18, 2026**, including
focus. The wrapper carries a request on the heartbeat's answer; the mod refuses compaction
during a turn, answers `requested`, then reports the native result for main's letter.
Interrupt waits for the matching aborted turn and records its requester in the stopped report
and native history. Capabilities follow the mod's admitted hello and heartbeat. Live acceptance
is pending ([design](v2/design-claude.md#control), [checked scope](research-claude-parity.md)).
The fixture also serves `control.Serve` and the shared command and letter workflow case
(`fixture-steered`, [testing-cases.md](testing-cases.md#a-stop-and-steering-a-session)).
The historical Claude Code observations below describe the 1.x module.

**Accepted live** on September 24, 2026, on Claude Code 2.1.280 with the installed
rewake built from `e65b865`, in the owner's sessions started through their launch
aliases. main-claude ran `rewake compact review-claude "<focus>"` on the idle reviewer:
exit 0, "compacted review-claude: 129381 tokens before, 5949 after". It then ran
`rewake interrupt write-claude` 8 s into a task: exit 0, and main received the stopped
report "main-claude interrupted this turn with rewake interrupt" at 14:20:27. The
interrupted turn had got as far as its first command. write-claude's next notice ended
with "main-claude interrupted your previous turn with rewake interrupt."; the notice
before it did not. The same run found two things, both fixed afterwards and described
below: `rewake inbox --awaited` listed the interrupted task as stopped by a person, and
main was sent a compaction notice for a compaction it had asked for itself.

**Accepted live on Codex** on September 25, 2026, on Codex CLI 0.155.1 with rewake built
from the working tree, by review-codex in a private state directory: a compaction of an
idle session, one refused as `in a turn` during a turn, an interrupt reported as stopped
by main, a delivery held pending during a compaction and delivered after it, a focus
refused with exit 2, and an ordinary task after them.

**Non-blocking since September 25, 2026** (owner decision below): `rewake compact`
returns once the compaction has started — or answers `requested` when its start was not
seen in time — and never waits for its end. The end reaches main as a letter from the
worker, a `notify` that owes nothing, with the token counts and the session's count of
compactions, or the refusal or failure that ended it ([below](#the-letter)). The
roadmap entry is [2026-09-25-compact-non-blocking.md](roadmap/2026-09-25-compact-non-blocking.md).

## The commands

```
rewake compact <session> [focus] [--json]
rewake interrupt <session> [--json]
```

Both are in the command table under "STEER A SESSION", with the usual help, examples
and refusals. The call is checked whole before anything is written:

- **Only a session of role main may call them.** The caller is the run named by
  `REWAKE_SESSION` and `REWAKE_EPOCH`, as for main's telemetry in `rewake list`; a
  shell outside rewake, a run that is not the registered one, or a session of another
  role is a wrong call.
- **The target** is a running session of the same room, not the caller itself; an
  unknown name gets the same refusal as `send`, with the live names.
- **Its harness must offer the verb** (`Offer.Control`, [code.md](code.md#the-adapter-api)),
  and a focus only where the harness can pass one for a single compaction
  (`CompactFocus`); each verb is offered on its own since S10. The fixture and pi
  harnesses and Claude Code take both requests and a focus. In
  1.x both harnesses took them and Codex took no focus, so a focus to a Codex session
  was refused (owner decision below).

Every one of these is exit 2 and writes nothing. Then **its run must have Control live**
— the record's live set, which the run's readiness exchange sets and a lost connection
withdraws at once ([design-api.md](v2/design-api.md#two-levels-offered-and-live)): a
target whose harness offers the verb but whose run has not proven Control, or has lost
it, is refused exit 1, nothing asked (S11, October 11, 2026). The offer says the harness
can be steered; only the live set says this run can. The design's `clear` verb has no
command in this build, so it has no refusal yet. Then the request is sent and the
command waits for its answer, within bounds — for a compaction, the answer to the
request, not the compaction's end:

| Outcome | Exit | Meaning |
| --- | --- | --- |
| `started` | 0 | the compaction has started; its end comes to main as a letter |
| `requested` | 0 | the session took the request, and the compaction's start was not seen within the adapter's bound (one second on pi, three in the 1.x module): it may still start, and its letter comes either way |
| `done` | 0 | the turn is interrupted |
| `refused`, `in a turn` | 1 | a compaction was asked while the worker is in a turn |
| `refused`, `no turn running` | 1 | an interrupt was asked while the worker is idle |
| `refused`, `compaction switched off` | 1 | the worker runs with compaction switched off |
| `refused`, `nothing to compact` | 1 | the conversation is too short to compact: on Claude Code by the host's measure, on Codex one that has run no turn |
| `refused`, `remote conversation` | 1 | the Claude Code session is a thin client, whose conversation lives on a remote session: compact that one |
| `refused`, `not answering` | 1 | nothing took the request within 5 seconds: the served side is not running, or the session is stalled — stopped, or its process frozen |
| `refused`, `cut short` | 1 | the command itself was interrupted before anything took the request; the request is withdrawn and nothing was done |
| `refused`, `no control directory` | 1 | the session was started by an earlier rewake, or its wrapper could not make one: restart it |
| `refused`, `withdrawn before it was taken` | 1 | the session took the request in the instant the command gave up, and did nothing |
| `refused`, `another request in flight` | 1 | another `compact` or `interrupt` is waiting on the same session |
| `failed` | 1 | the harness raised something else: final, like a refusal, and no letter follows; or, marked `open`, the request went out and no outcome came — the command's wait ran out or was cut short once the request was taken, or the served side lost the request on its way — and for a compaction, which may still run, its letter comes |

A refusal carries the host's own error text as its detail and names the next action.
Under `--json` the model is printed either way: `session`, `action`, `focus`,
`outcome`, `reason`, `detail`, and `tokensBefore`, `tokensAfter` where an answer
carries them. An answer's `open` tells a failure that leaves the outcome undecided from a
final one; a served side of an earlier rewake never sets it, so its failures are final. A refusal the host gives at once — mid-turn, switched off, too short when
it comes before the start is seen — is the command's answer, and no letter follows.

### The letter

When a compaction the command answered `started`, `requested` or an open `failed`
ends, main gets a letter from the worker, of kind `notify`, with
its outcome; main's own wrapper owes it. How it is made and why it always comes:
[remote-control-letter.md](remote-control-letter.md).

## The control directory

One per run, like the sockets, so a request written for a run that ended never reaches
the next holder of the name:

```
<REWAKE_DIR>/v2/rooms/<room>/control/<name>.<epoch>/  0700
  lock                    flock of the asker; a second asker is refused, not queued
  request.json            {id, action, focus, from, conversation}, 0600, put in place by rename
  <id>.taken              written by the served side the moment it picks the request up
  <id>.started            written once the compaction's start is seen; in 1.x by Claude Code's collector, when its PreCompact arrived
  <id>.result             {id, outcome, reason, detail, tokensBefore, tokensAfter}, written once
```

The wrapper makes the directory before the harness starts and removes it when the
session ends, only for a harness that offers a Control verb (`Offer.Control`): a directory
nobody serves would say `not answering` where the true next step is a restart. If it
cannot be made, or the path found there is not a private directory of this user — a
symlink, another user's, one open to the group or the world, which the wrapper refuses
rather than repairs — the launch goes on without one, and every request to that run is
refused as `no control directory`. The id is 32 lowercase hex
characters, and the served side refuses any other: it builds file names from it.

The served side — in 1.x rewake's module in a Claude Code session, and in this build the
fixture's, pi's and Claude Code's runs through `control.Serve` — can write files but not remove them, so the asker owns every removal
(`internal/core/control`):

1. Take the lock, and remove everything a dead asker or an expired request left.
2. Write the request.
3. Wait up to the **pickup** limit for `<id>.taken` or `<id>.result`. Nothing: remove
   the request — withdraw it — and only then look for the mark once more. Still no mark:
   `not answering`.
4. Taken: wait up to the **outcome** limit for a result that parses and carries the
   request's id. A file that does not parse yet is one still being written and is read
   again. The request stays in place meanwhile; the served side checks it (below).
5. Remove the request, the taken mark and the result on the way out. No result in time:
   `failed`, marked `open`, saying the request was taken and may still be carried out;
   its late result is cleared by the next request.

**Giving up is honest in every order.** The served side, having read a request, first
writes its mark and then checks that the request is still in place: carried out only if
it is, answered `withdrawn before it was taken` if it is gone or replaced by another.
The asker, giving up, first withdraws the request and then looks for the mark. Of the
two, the one that looks second sees what the other did first:

- the served side checked before the withdrawal: it carries the request out, and the
  asker, looking after its own withdrawal, finds the mark and waits for the outcome;
- it marked before the asker's last look but checked after the withdrawal: it does
  nothing, and the asker waits for and reports `withdrawn before it was taken`;
- it marked after the asker's last look: the asker reports `not answering`, and the
  served side, finding the request gone, does nothing either; what it writes is cleared
  by the next request.

So the command never reports `not answering` for a request that was carried out. It
adds no limit of its own: the check is one more read before acting. Found in review on
September 24, 2026, when the asker still removed the request at pickup and a module that
had read it in the same instant acted after `not answering` was reported.

**An asker cut short** — an Esc or Ctrl+C on main ends its Bash tool's command with
SIGTERM, and SIGKILL 1.5 s later if it is still alive (seen live and read in the binary,
Claude Code 2.1.280, September 24, 2026,
[research.md](research.md#interrupting-a-turn-commands-and-the-stop-hook)),
or the call is terminated otherwise — stops waiting on SIGTERM or SIGINT, withdraws the
request on the way out and reports that the wait was cut short: refused as `cut short`
before a pickup, `failed` after one. So a stalled target that resumes later finds nothing
to carry out. The withdrawal waits for nothing, so it fits in the 1.5 s: a probe of the
same day saw exit 1 in 53 ms with the request removed.

The limits: pickup 5 seconds — the module polled four times a second — and outcome 10
seconds. A compaction is answered once it has started, which the served side waits up
to 3 seconds to see; its end is waited for apart from the command, by the served side,
and reaches main as the letter. Until September 25, 2026
the command waited up to 90 seconds for the end.

## On Claude Code

Until S9, October 11, 2026, rewake's plugin module served the directory inside the
session ([claude-plugin.md](archive/1.x/claude-plugin.md) has the module): the wrapper
wrote the directory's path into it, and it polled for a request four times a second.
A compaction called the host's `$.session.compact()`, with the host's own refusals
mapped to `in a turn`, `compaction switched off`, `remote conversation` and `nothing
to compact` (read in the 2.1.280 binary); the telemetry collector marked its start on
`PreCompact` and counted its end on `PostCompact`, and the module told rewake who had
asked so main's notice could be skipped. An interrupt aborted the turn the module had
seen start, and its `turn.complete` with reason `aborted` carried the asker's name to
the collector, which published the `stopped`. The module, the collector and the hooks
they read left with S9; the 1.x text of this section is in the history at `98acc3b`.

What stays, on every harness that serves the directory:

**What an interrupt from main produces.** A `stopped` with the text "`<main>`
interrupted this turn with rewake interrupt" instead of the person-at-the-keyboard
text, and routing unchanged: the `stopped` goes only to the sessions waiting on that
worker. main's `rewake inbox --awaited` shows the task as `stopped:` followed by the
stop's own text, so it names who stopped the turn: the person at the keyboard, or the
main.

**Main gets a letter instead of a notice of its own compaction.** main's wrapper sends a
notice "Rewake: context compacted (compaction N)." for each compaction of a worker. It
skips a compaction whose cue names this main as the asker, because the letter carries
the tokens and the count. A compaction the worker makes itself, or one another main
asked for, keeps its notice.

**The line in the next notice** was Claude Code's: a plugin abort left no trace for the
model, so after an interrupt from main the worker's next rewake notice ended with
"`<main>` interrupted your previous turn with rewake interrupt.", once. The collector
that kept the mark left in S9, and no adapter of this build adds the line; the fixture's
program ends the turn saying who interrupted it.

## On Codex

In 1.x the wrapper served the directory itself, through the gateway that held the
app-server connection ([remote-control-codex.md](archive/1.x/remote-control-codex.md)),
and the directory carried one action that was not main's, `accept`
([delivery-conversation.md](archive/1.x/delivery-conversation.md)). Both left with the
Codex adapter in S8.

## Known limits

- **An asker killed outright** (SIGKILL) cannot withdraw its request: a target stalled
  at that moment carries it out when it resumes, telling nobody. The next request clears
  what is left.
- **A compaction on Codex that outlives 10 minutes** — its mark's running bound — is
  answered `started`, which is no outcome yet: main's letter waits for its end and comes
  from it. One still not ended at main's bound gets the bound letter saying it may still
  be running. One whose turn is not seen within 80 seconds of the request ends in a
  `failed` letter, and may still start. A compaction anywhere with no word of it within
  15 minutes gets main's bound letter, and its later end gives no second letter.
- **A worker whose wrapper ends just after its compaction**, before its snapshot is next
  written — within a quarter second — takes the outcome with it: the 3 seconds main's
  wrapper waits after a departure cover a snapshot published late, not one never
  published. The letter then says the worker left before the compaction ended, while
  `rewake list` of that run would have shown it compacted.

## Owner decisions

- Only main may call these commands (September 24, 2026).
- A compaction is refused at once while the worker is in a turn; it never waits for
  idle and never happens on its own (September 23, 2026).
- The behaviour is the same and predictable on both harnesses (September 24, 2026).
- A focus for a Codex compaction is refused for now: `thread/compact/start` has no
  field for it, and `compact_prompt` replaces the whole prompt for the whole
  conversation. A proper route and an emulation through the conversation's history are
  to be researched later ([work-queue.md](archive/work-queue.md)) (September 24, 2026).
- After an interrupt from main, the Claude Code worker's next rewake notice carries a
  line saying so, once, so that its model sees roughly what Codex's does
  (September 24, 2026). The line left with the plugin in S9, October 11, 2026; stage 4
  decides it again for Claude Code's adapter.
- On Codex no such line is added: Codex puts `<turn_aborted>` into the model's history
  itself, and the line on Claude Code exists to match it. The two harnesses differ here
  on purpose (September 24, 2026).
- On Codex a conversation that has run no turn is refused as `nothing to compact`
  before anything is sent, as Claude Code answers, rather than compacted at the cost of
  a model call (September 24, 2026).
- On Codex the terminal's `/compact` while main's compaction runs is refused by the
  gateway with an error the terminal shows, not queued behind it: a second compaction
  right after is pointless, and a refusal is predictable (September 24, 2026).
- When main asked for a compaction itself with `rewake compact`, it gets no "context
  compacted" notice for it. What main needs comes instead with the outcome — the tokens
  before and after, and the session's count of compactions with this one — since the
  decision below in the letter rather than in the command's answer. Compactions the
  worker makes itself, and ones anyone else asked for, keep their notice
  (September 24, 2026).
- `rewake compact` does not block: it returns as soon as the request's outcome is known
  — refused at once, or the compaction started — and the result comes to main later as
  a letter, the way `send` returns at once and the report comes later. Waiting for the
  end, up to 80–90 seconds, stalled the orchestrator on one shell call. The behaviour is
  the same on both harnesses; `rewake interrupt` stays as it is (September 24, 2026).

## Tests

What proves each part — the unit tests and the workflow cases — is listed in [remote-control-tests.md](remote-control-tests.md).
