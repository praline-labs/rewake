# Remote control: `rewake compact` and `rewake interrupt`

A main session acts on a running worker without a person at its keyboard: it compacts
the worker's conversation, or interrupts the worker's turn. This is stage 2 of rewake's
function-hooks plugin for Claude Code ([claude-plugin.md](claude-plugin.md) is stage 1,
which only listens). What the harness offers for it is in
[research-claude-actions.md](research-claude-actions.md)
and, for Codex, [research-protocol.md](research-protocol.md#compaction-and-interrupt-on-request).

Built on September 24, 2026 for Claude Code, with unit and module tests and a workflow
case. The Codex side is next ([work-queue.md](work-queue.md#now-stage-2-of-the-plugin-the-rest)).

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
- **Its harness must take control requests** (`harness.Steerable`), and a focus only
  where the harness can pass one for a single compaction (`CompactFocus`). Codex
  implements neither yet, so any request to a Codex session is refused; when part B
  lands, a focus to a Codex session still is (owner decision below).

Every one of these is exit 2 and writes nothing. Then the request is sent and the
command waits for its outcome, within bounds:

| Outcome | Exit | Meaning |
| --- | --- | --- |
| `done` | 0 | compacted — with the token counts before and after when the harness gave them — or the turn is interrupted |
| `refused`, `in a turn` | 1 | a compaction was asked while the worker is in a turn |
| `refused`, `no turn running` | 1 | an interrupt was asked while the worker is idle |
| `refused`, `compaction switched off` | 1 | the worker runs with compaction switched off |
| `refused`, `nothing to compact` | 1 | the conversation is too short to compact |
| `refused`, `not answering` | 1 | nothing took the request within 5 seconds: the plugin is not loaded, or the session is stalled — stopped, or its process frozen |
| `refused`, `cut short` | 1 | the command itself was interrupted before anything took the request; the request is withdrawn and nothing was done |
| `refused`, `no control directory` | 1 | the session was started by an earlier rewake, or its wrapper could not make one: restart it |
| `refused`, `withdrawn before it was taken` | 1 | the session took the request in the instant the command gave up, and did nothing |
| `refused`, `another request in flight` | 1 | another `compact` or `interrupt` is waiting on the same session |
| `failed` | 1 | the harness raised something else, or no outcome came in time |

A refusal carries the host's own error text as its detail and names the next action.
Under `--json` the model is printed either way: `session`, `action`, `focus`,
`outcome`, `reason`, `detail`, `tokensBefore`, `tokensAfter`.

## The control directory

One per run, like the sockets, so a request written for a run that ended never reaches
the next holder of the name:

```
<REWAKE_DIR>/rooms/<room>/control/<name>.<epoch>/     0700
  lock                    flock of the asker; a second asker is refused, not queued
  request.json            {id, action, focus, from}, 0600, put in place by rename
  <id>.taken              written by the served side the moment it picks the request up
  <id>.result             {id, outcome, reason, detail, tokensBefore, tokensAfter}, written once
```

The wrapper makes the directory before the harness starts and removes it when the
session ends, only for a harness that serves it (`harness.Steerable`): a directory
nobody serves would say `not answering` where the true next step is a restart. If it
cannot be made the launch goes on without one, and every request to that run is refused
as `no control directory`. The id is 32 lowercase hex
characters, and the served side refuses any other: it builds file names from it.

The served side — rewake's module in a Claude Code session — can write files but not
remove them, so the asker owns every removal (`internal/control`):

1. Take the lock, and remove everything a dead asker or an expired request left.
2. Write the request.
3. Wait up to the **pickup** limit for `<id>.taken` or `<id>.result`. Nothing: remove
   the request — withdraw it — and only then look for the mark once more. Still no mark:
   `not answering`.
4. Taken: wait up to the **outcome** limit for a result that parses and carries the
   request's id. A file that does not parse yet is one still being written and is read
   again. The request stays in place meanwhile; the served side checks it (below).
5. Remove the request, the taken mark and the result on the way out. No result in time:
   `failed`, saying the request was taken and may still be carried out; its late result
   is cleared by the next request.

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
[research-claude-control.md](research-claude-control.md#a-second-probe-signals-and-hooks-around-an-interruption)),
or the call is terminated otherwise — stops waiting on SIGTERM or SIGINT, withdraws the
request on the way out and reports that the wait was cut short: refused as `cut short`
before a pickup, `failed` after one. So a stalled target that resumes later finds nothing
to carry out. The withdrawal waits for nothing, so it fits in the 1.5 s: a probe of the
same day saw exit 1 in 53 ms with the request removed.

The limits: pickup 5 seconds for both — the module polls four times a second —
outcome 90 seconds for a compaction, a request to the model that stays under the two
minutes an agent's shell call is usually given, and 10 for an interrupt.

## On Claude Code

The wrapper writes the directory's path into the module (`__REWAKE_CONTROL__`, empty
without one). On `session.start` the module starts `$.clock.every(250, …)` once per
load; each tick, unless the previous one is still acting, it asks `$.fs.exists` for the
request — a `$.fs.read` of a missing file is logged by the host as an error — reads and
checks it, skips an id this load has handled or that has a taken mark (a hot reload
runs `session.start` again with an empty memory), writes the mark, checks the request is
still in place, acts, and writes the result.

- **Compact** calls `$.session.compact()`, or `$.session.compact({ instructions })`
  with a focus. The module does not check for a turn: the host refuses mid-turn by
  itself, atomically, and its text becomes `in a turn`; "switched off" becomes
  `compaction switched off`; "Not enough messages to compact" becomes
  `nothing to compact`; anything else is `failed` with the host's text. The result
  carries `tokensBefore` and `tokensAfter` only — never the summary the host returns,
  since transcripts are not read. The compaction is counted by the existing telemetry
  like any other (`completedCompactions`).
- **Interrupt** needs the running turn's id, which only the module knows: it keeps it
  from `turn.start` and `turn.complete` of its own events, those without an `agentId`.
  No turn known: `no turn running` without asking the host. Otherwise it remembers the
  turn and the asker, calls `$.turn.abort({ turnId })`, and passes a host refusal on as
  `no turn running`.

**What an interrupt from main produces.** The aborted turn ends through stage 1's path:
`turn.complete` with reason `aborted`, now with `by` — the asker's name, sent only when
the aborted turn is the one that request asked for. The collector publishes `stopped`
with the text "`<main>` interrupted this turn with rewake interrupt" instead of the
person-at-the-keyboard text, and routing is unchanged: the `stopped` goes only to the
sessions waiting on that worker.

**The line in the next notice.** A plugin abort leaves no trace for the model: the
prompt stays, nothing marks the stop (unlike Codex's `<turn_aborted>`). So after an
interrupt from main, the worker's next rewake notice ends with
"`<main>` interrupted your previous turn with rewake interrupt." Once: the mark is used
up only by a notice that was delivered, and only if no newer interrupt replaced it. Any
turn that starts or ends before a notice lays it aside — the interrupted turn is no
longer the previous one. A person's Esc sets no mark.

## On Codex (part B)

The wrapper, not a module, serves the same directory: it holds the app-server
connection and knows the running turn. It refuses a compaction itself while a turn runs
or a `turn/start` is in flight, because Codex's own `compact()` would abort that turn
instead of refusing; otherwise it sends `thread/compact/start` with the gateway's
manual-compaction mark, and `turn/interrupt` with the running turn's id. Codex records
`<turn_aborted>` in the history itself, so no notice line is added there. The harness
will implement `Steerable` with `CompactFocus` false.

## Known limits

- **An asker killed outright** (SIGKILL) cannot withdraw its request: a target stalled
  at that moment carries it out when it resumes, telling nobody. The next request clears
  what is left.
- **A compaction that outlives 90 seconds** is reported `failed` and may still finish;
  `rewake list` shows the compaction when it does.
- **A hot reload of the module in the middle of a turn loses the tracked turn**: the
  reloaded module has not seen that turn start, so an interrupt answers `no turn
  running` until the next turn. rewake never changes the module's file once the
  session runs; only an edit from outside causes it.
- **Without the module** — `--bare`, function hooks switched off, a harness version
  without them — every request is `not answering`; such a session shows interruptions
  unobserved in `rewake list`.

## Owner decisions

- Only main may call these commands (September 24, 2026).
- A compaction is refused at once while the worker is in a turn; it never waits for
  idle and never happens on its own (September 23, 2026).
- The behaviour is the same and predictable on both harnesses (September 24, 2026).
- A focus for a Codex compaction is refused for now: `thread/compact/start` has no
  field for it, and `compact_prompt` replaces the whole prompt for the whole
  conversation. A proper route and an emulation through the conversation's history are
  to be researched later ([work-queue.md](work-queue.md)) (September 24, 2026).
- After an interrupt from main, the Claude Code worker's next rewake notice carries a
  line saying so, once, so that its model sees roughly what Codex's does
  (September 24, 2026).

## Tests

- `internal/control/control_test.go` — the protocol: a done answer and the cleanup, the
  modes, `not answering` with the request removed, a taken request without an outcome
  and its late result cleared, a partial or foreign answer ignored, a second asker
  refused without writing, no directory, a request taken as the asker gives up, in
  each of the three orders above, a taken request left in place while it is served, and
  an asker cut short withdrawing its request as `cut short`.
- `internal/wrap/control_test.go` — the wrapper makes the directory, private, for a
  harness that serves it, removes it with the session, and makes none for one that
  does not.
- `internal/cli/steer_test.go` — the commands against a fake served side: the outputs,
  the refusals with their next action, `not answering`, and every wrong call refused
  with exit 2 and nothing written; a harness that is not steerable, and one without a
  focus; and a SIGTERM during the pickup, which ends the call as `cut short` with the
  request withdrawn instead of ending the process.
- `internal/harness/claude/plugin_control_test.go` — the module under node against a
  strict host with the research's forms and refusals of `$.clock.every`, `$.fs`,
  `$.session.compact` and `$.turn.abort`: an idle compaction, the host's refusals passed
  on, the interrupter reported, a request carried out once across a reload, a request
  withdrawn or replaced as it is marked left undone, and no polling without a directory.
- `internal/harness/claude/telemetry/interrupter_test.go` and
  `internal/harness/claude/lane_interrupt_test.go` — the stopped text naming main, the
  mark and when it is used up or laid aside, and the notice line.
- The workflow case `claude-steered` — both commands end to end from a main against
  workers whose module runs under node in the fixture: an idle compaction counted by the
  telemetry, a compaction refused mid-turn with the turn going on, an interrupt giving
  main the stopped text and the worker the notice line once, an idle interrupt refused,
  a worker without the module not answering, and a worker's call refused as a wrong call;
  seven product mutants, one per link ([testing-plugin.md](testing-plugin.md#steering-a-session)).
