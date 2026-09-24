# Remote control: `rewake compact` and `rewake interrupt`

A main session acts on a running worker without a person at its keyboard: it compacts
the worker's conversation, or interrupts the worker's turn. This is stage 2 of rewake's
function-hooks plugin for Claude Code ([claude-plugin.md](claude-plugin.md) is stage 1,
which only listens). What the harness offers for it is in
[research-claude-actions.md](research-claude-actions.md)
and, for Codex, [research-protocol.md](research-protocol.md#compaction-and-interrupt-on-request).

Built on September 24, 2026 for Claude Code, with unit and module tests and a workflow
case, and the same day for Codex (part B), where the wrapper serves the requests itself
([remote-control-codex.md](remote-control-codex.md)).

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
refused with exit 2, and an ordinary task after them. The run scenario by scenario is in
[the roadmap entry](roadmap/2026-09-24-remote-control-codex.md).

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
  where the harness can pass one for a single compaction (`CompactFocus`). Both
  harnesses take the requests; Codex takes no focus, so a focus to a Codex session is
  refused here, before anything is sent (owner decision below).

Every one of these is exit 2 and writes nothing. Then the request is sent and the
command waits for its outcome, within bounds:

| Outcome | Exit | Meaning |
| --- | --- | --- |
| `done` | 0 | compacted — with the token counts before and after when the harness gave them, and the session's count of compactions with this one — or the turn is interrupted |
| `refused`, `in a turn` | 1 | a compaction was asked while the worker is in a turn |
| `refused`, `no turn running` | 1 | an interrupt was asked while the worker is idle |
| `refused`, `compaction switched off` | 1 | the worker runs with compaction switched off |
| `refused`, `nothing to compact` | 1 | the conversation is too short to compact: on Claude Code by the host's measure, on Codex one that has run no turn |
| `refused`, `remote conversation` | 1 | the Claude Code session is a thin client, whose conversation lives on a remote session: compact that one |
| `refused`, `not answering` | 1 | nothing took the request within 5 seconds: the plugin is not loaded, or the session is stalled — stopped, or its process frozen; on Codex, the wrapper is stalled |
| `refused`, `cut short` | 1 | the command itself was interrupted before anything took the request; the request is withdrawn and nothing was done |
| `refused`, `no control directory` | 1 | the session was started by an earlier rewake, or its wrapper could not make one: restart it |
| `refused`, `withdrawn before it was taken` | 1 | the session took the request in the instant the command gave up, and did nothing |
| `refused`, `another request in flight` | 1 | another `compact` or `interrupt` is waiting on the same session |
| `failed` | 1 | the harness raised something else, or no outcome came in time |

A refusal carries the host's own error text as its detail and names the next action.
Under `--json` the model is printed either way: `session`, `action`, `focus`,
`outcome`, `reason`, `detail`, `tokensBefore`, `tokensAfter`, `compaction`. For a
compaction that is done, the command waits up to 3 seconds for the session's telemetry
to count it and prints the count as the compaction notice would, "(compaction N)". If
the telemetry has not counted it by then, the line says so and points to `rewake list`.

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

The served side — rewake's module in a Claude Code session, the wrapper itself on Codex
— can write files but not remove them, so the asker owns every removal
(`internal/control`):

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
  itself, atomically, and its text becomes `in a turn`, as does "a turn is in flight",
  its refusal while another compaction runs (seen live during a typed `/compact`), and
  "an external turn is driving the conversation"; "switched off" becomes `compaction
  switched off`; "a thin client's conversation lives on the remote session" becomes
  `remote conversation`; "Not enough messages to compact" becomes `nothing to compact`;
  anything else is `failed` with the host's text. The table holds every refusal the
  host has before a compaction starts (read in the 2.1.280 binary), so a `failed` is an
  error after the start — the summary's request, or applying it. The
  result carries `tokensBefore` and `tokensAfter` only — never the summary the host
  returns, since transcripts are not read. The compaction is counted by the existing
  telemetry like any other (`completedCompactions`).
- **Who asked for a compaction.** Before it asks the host, while it knows no turn is
  running, the module tells rewake who asked — `compact.asked` with the request id and
  the asker — and waits until that report is sent: `$.process.run` resolves only once
  `rewake observe` has ended (read in the 2.1.280 binary). Datagrams reach the
  collector in the order they were sent, so the mark is there before any hook of that
  compaction, and the next `PostCompact` takes it up. A refusal or failure is reported
  as `compact.refused` and sent before the answer is written; it lays the mark aside.
  It says whether the host had started the compaction — "Not enough messages" and a
  `failed` come after `PreCompact`, the other refusals before the host starts — so the
  collector
  ends the listing's `compacting` for it, or lets its `PreCompact`, a background hook
  that may come after the refusal, start nothing. Mid-turn no mark is sent: the host
  refuses that compaction, and the turn may compact on its own.
- **The module's `session.compact` handler** never sees the compaction its own call
  raised: the host skips a plugin's handlers for an event that plugin's code raised,
  as re-entry (seen live and read in the binary, 2.1.280). It sees every other one,
  with the compaction itself beneath `next(e)`, and uses only its end.
- **What the mark guarantees.** The collector gives it to the next `PostCompact` it
  receives, whichever compaction that belongs to. The hooks run as separate processes,
  so a compaction's `PostCompact` can arrive a moment after its end. After the end of
  a compaction the module did not ask for — a person's `/compact`, an automatic one —
  it waits a second before it sends the mark and asks the host for the main's, so that
  compaction's `PostCompact` is in first. Within a second of the other's end the request waits rather than being
  refused; while the other still runs, the host refuses it as `in a turn`.
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
sessions waiting on that worker. main's `rewake inbox --awaited` shows the task as
`stopped:` followed by the stop's own text, so it names who stopped the turn: the person
at the keyboard, or the main.

**Main gets no notice of its own compaction.** main's wrapper sends a notice
"Rewake: context compacted (compaction N)." for each compaction of a worker. It skips a
compaction whose cue names this main as the asker, because the command's answer already
carries the tokens and the count. A compaction the worker makes itself, or one another
main asked for, keeps its notice.

**The line in the next notice.** A plugin abort leaves no trace for the model: the
prompt stays, nothing marks the stop (unlike Codex's `<turn_aborted>`). So after an
interrupt from main, the worker's next rewake notice ends with
"`<main>` interrupted your previous turn with rewake interrupt." Once: the mark is used
up only by a notice that was delivered, and only if no newer interrupt replaced it. Any
turn that starts or ends before a notice lays it aside — the interrupted turn is no
longer the previous one. A person's Esc sets no mark.

## On Codex

The wrapper serves the directory itself, through the gateway that holds the
app-server connection: [remote-control-codex.md](remote-control-codex.md).

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
- **A `PostCompact` later than a second** after the end of a compaction the module did
  not ask for, arriving after the mark of a main's compaction, is taken for that one:
  the worker's compaction counts as main's, and main gets no notice of it, while main's
  own gets a notice and `rewake compact` prints the other compaction's number. The
  wait narrows this to a hook process that took more than a second to start and send;
  nothing ties a `PostCompact` to its compaction by identity, since the hook carries
  none.
- **A compaction whose `PostCompact` never reaches the collector** — a hook that
  failed — leaves its mark in place. The session's next compaction then takes the mark
  and counts as main's, so main gets no notice of it. The command, whose request it was,
  has stopped waiting by then and reports no count.
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
  compacted" notice for it. The command's answer carries what main needs instead: the
  tokens before and after, and the session's count of compactions with this one.
  Compactions the worker makes itself, and ones anyone else asked for, keep their notice
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
  a done compaction with its count from the telemetry and without one that has not
  come,
  the refusals with their next action, `not answering`, and every wrong call refused
  with exit 2 and nothing written; a harness that is not steerable, and one without a
  focus; and a SIGTERM during the pickup, which ends the call as `cut short` with the
  request withdrawn instead of ending the process.
- `internal/harness/claude/plugin_control_test.go` and `plugin_compact_test.go` — the module under node against a
  strict host with the research's forms and refusals of `$.clock.every`,
  `$.clock.sleep`, `$.fs`,
  `$.session.compact` and `$.turn.abort`: an idle compaction, the host's refusals passed
  on, "in flight", "an external turn" and the thin client's among them, and an error
  after the start counted as started, the asker reported and sent before the host compacts, a
  refusal reported and sent before the answer with whether the host had started, no
  mark sent mid-turn, a second waited out after the end of a compaction the module did
  not ask for, the host itself held to the re-entry rule, the interrupter reported, a request carried out once across a reload, a request
  withdrawn or replaced as it is marked left undone, and no polling without a directory.
- `internal/harness/claude/telemetry/interrupter_test.go` and
  `internal/harness/claude/lane_interrupt_test.go` — the stopped text naming main, the
  mark and when it is used up or laid aside, and the notice line;
  `internal/harness/claude/telemetry/compaction_asked_test.go` — the mark decoded, taken
  up by the next compaction and laid aside by its refusal, which ends `compacting` for
  a started compaction, also when its `PreCompact` comes after the refusal, and leaves
  another compaction's alone.
- `internal/wrap/session_notices_test.go` — no notice of a compaction this main asked
  for, and one each for a compaction another main asked for and one nobody asked for.
- The workflow case `claude-steered` — both commands end to end from a main against
  workers whose module runs under node in the fixture: an idle compaction counted by the
  telemetry, with its count in the answer and no notice of it to main, a compaction
  refused mid-turn with the turn going on, an interrupt giving main the stopped text,
  also in its awaited list, and the worker the notice line once, an idle interrupt
  refused, a worker without the module not answering, and a worker's call refused as a
  wrong call; eleven product mutants, one per link
  ([testing-plugin.md](testing-plugin.md#steering-a-session)).
- `internal/control/serve_test.go` — the served side the Codex wrapper runs: an answer
  written once with the request's id, a request carried out once across a second look
  and a fresh server, one withdrawn or replaced as it is marked left undone, and a
  foreign id not taken.
- `internal/harness/codex/gateway/steer_test.go` and `steer_guard_test.go` — a
  compaction on request marked manual, never published as work, with its tokens and
  counted as the asker's; refused without a word to the server while a turn, a
  `turn/start` or `review/start` in flight, the terminal's `/compact` or a working
  status runs, and between a `turn/start` reply and its `turn/started`, the terminal's
  and a delivery's; refused as `nothing to compact` on a new conversation with no turn,
  and sent on a resumed one; a server's refusal leaving no mark, an unanswered request
  keeping it for a late compaction, and a mark whose compaction never starts ceasing
  to hold at its bound while main's next compaction stays refused; a turn acknowledged
  before a `/resume` to another thread keeping the conversation uncertain until a later
  turn is answered; the terminal's `/compact` answered by the gateway while main's
  runs; a delivery waiting out main's compaction and going after it; an interrupt
  naming main in the stopped outcome, of a turn the terminal started and of one a
  delivery started, and refused idle, during a compaction and when the server says no
  turn is active. `internal/harness/codex/server_steer_test.go` — the wrapper serves
  the run's directory, answers what it cannot carry out, and passes a reservation
  refused for a compaction to the inbox as one to try again;
  `internal/inbox/reservation_test.go` keeps such a message pending and delivers it
  on the next pass. `activity_test.go` holds the record of what a conversation does
  to the frames that set and clear it: a compaction refused while an inline review is
  acknowledged and sent past a detached one; an interrupt naming the turn a
  `turn/start` reply named before its `turn/started`, and the one a resume's snapshot
  shows running; a delivery that queued on the gate behind main's compaction staying
  pending; a compaction left behind by a `/resume` not ended by an idle resume — still
  holding when its start was seen, lost sight of when only answered — and held until its
  end when it comes back running; a reply naming a turn already ended holding nothing.
  `accepted_test.go` holds main's rule for work the server accepted: an operation
  without its end — a review, started or not, a turn through `systemError` or through
  its items — leaving the conversation uncertain through a resume, a status and time;
  the answer to a later turn ending that; a compaction answered but not started
  outliving an idle resume; a mark holding deliveries only up to its bound, with main's
  answer saying why; an unbound mark taking no other turn; an interrupt learning the
  turn's id from an item event; a late reply from an earlier selection naming no turn. `mark_test.go` holds how the mark
  finds its turn: an ordinary turn compacting inside itself never taken for it, also
  when the proof it is work comes after its compaction item; a tied mark never moving;
  an operation past the record's capacity leaving the conversation uncertain; main's
  wait ending the hold and its answer saying whether the turn was seen. `sight_test.go`
  holds the three safety properties against attribution: a mark lost sight of when the
  terminal leaves before its turn — main answered at once, deliveries going, the
  conversation uncertain, also through a goal's turn failing at its compaction; a lost
  compaction's turn settling nothing, whether its item, only its end or a run of unknown
  id comes back, or the terminal's own compaction came between, while a goal's turn
  shown to be work by its item and a turn of the terminal's still report; an untied mark leaving no author on
  the turn's compaction; main's wait without a reply ending the hold; a compaction that
  ended as the wait did answered by its end; and a turn whose end came before its reply
  still reporting, the terminal's and a delivery's. `proof_test.go` holds the
  publication rule: a compaction only advisory when stopped before its item by Esc or
  its hook, and never published when it runs after a goal's turn took its mark or ends
  on the next connection; a turn of the terminal's and a delivery's reporting in every
  order of the reply and the turn's events, with no item or with the reply last; and a
  turn named on one connection reporting when it ends on the next. `advisory_test.go`
  holds the advisory report of a turn without proof — a goal's failing before its item,
  one whose reply was lost with the connection — none for a compaction shown by its
  item, the turn's own outcome after a late proof, a stop included, under an identity
  apart from the advisory's, and no outcome dropped when the connection ends or later
  ones push it out. `reconnect_uncertainty_test.go` holds an inline review, or a turn
  whose reply was lost, keeping main's compaction refused after a reconnect until its
  end, a later answered turn or a delivered message, and the refusal texts naming
  their ways out. `internal/cli/gap_advisory_test.go` holds the advisory reports at the
  waiter, and a proven stop after an advisory reported with its times, taking the
  pending mark.
- The workflow case `codex-steered` — the same commands on the Codex column, the shim
  answering `thread/compact/start` and `turn/interrupt` as the server does, aborting a
  held turn for a compaction sent mid-turn: a focus refused with exit 2 and nothing
  compacted, an idle compaction with its tokens and count and no notice to main, a
  compaction refused mid-turn with the turn going on, an interrupt giving main the
  stopped text, also in its awaited list, and the worker's next notice no line, an idle
  interrupt refused, and a worker's call refused; five product mutants
  ([testing-plugin.md](testing-plugin.md#steering-a-session)). The shape case checks the
  shim's reply and events for both requests against the schema.
