---
description: Main steering a running worker — rewake compact and rewake interrupt, their checks and outcomes, the control directory they share with rewake answer, how each harness serves it, and the letter that brings a compaction's end to main.
---

# Remote control: `rewake compact` and `rewake interrupt`

A main session acts on a running worker without a person at its keyboard: it compacts the
worker's conversation, or interrupts the worker's turn. The same control directory carries
`rewake answer`, main's answer to a worker's permission request or question, which the
worker's wrapper reads against the open request and gives its mod
([permission-requests.md](permission-requests.md#the-path)).

`rewake compact` does not block: it returns once the compaction has started, or answers
`requested` when its start was not seen in time, and never waits for its end. The end reaches
main later as a letter from the worker ([below](#the-letter)), the way `send` returns at once
and the report comes later. `rewake interrupt` returns once the turn is interrupted.

## The commands

```
rewake compact <session> [focus] [--json]
rewake interrupt <session> [--json]
```

Both are in the command table under "STEER A SESSION", with the usual help, examples and
refusals. The call is checked whole before anything is written:

- **Only a session of role main may call them.** The caller is the run named by
  `REWAKE_SESSION` and `REWAKE_EPOCH`, as for main's telemetry in `rewake list`; a shell
  outside rewake, a run that is not the registered one, or a session of another role is a
  wrong call.
- **The target** is a running session of the same room, not the caller itself; an unknown
  name gets the same refusal as `send`, with the live names.
- **Its harness must offer the verb** (`Offer.Control`, [code.md](code.md#the-adapter-api)),
  and a focus only where the harness can pass one for a single compaction
  (`CompactFocus`); each verb is offered on its own. Claude Code, pi and the fixture take
  both requests and a focus.

Every one of these is exit 2 and writes nothing. Then **its run must have Control live** —
the record's live set, which the run's readiness exchange sets and a lost connection
withdraws at once ([design-api.md](v2/design-api.md#two-levels-offered-and-live)): a target
whose harness offers the verb but whose run has not proven Control, or has lost it, is
refused exit 1, nothing asked. The offer says the harness can be steered; only the live set
says this run can. Then the request is sent and the command waits for its answer, within
bounds — for a compaction, the answer to the request, not the compaction's end:

| Outcome | Exit | Meaning |
| --- | --- | --- |
| `started` | 0 | the compaction has started; its end comes to main as a letter |
| `requested` | 0 | the session took the request and its start was not seen within the served side's bound — at once on Claude Code, one second on pi: it may still start, and its letter comes either way |
| `done` | 0 | the turn is interrupted |
| `refused`, `in a turn` | 1 | a compaction was asked while the worker is in a turn |
| `refused`, `no turn running` | 1 | an interrupt was asked while the worker is idle |
| `refused`, `compaction switched off` | 1 | the worker runs with compaction switched off |
| `refused`, `nothing to compact` | 1 | the conversation is too short to compact |
| `refused`, `remote conversation` | 1 | the session is a thin client, whose conversation lives on a remote session: compact that one |
| `refused`, `not answering` | 1 | nothing took the request within 5 seconds: the served side is not running, or the session is stalled — stopped, or its process frozen |
| `refused`, `cut short` | 1 | the command itself was interrupted before anything took the request; the request is withdrawn and nothing was done |
| `refused`, `no control directory` | 1 | the session's wrapper could not make one: restart it |
| `refused`, `withdrawn before it was taken` | 1 | the session took the request in the instant the command gave up, and did nothing |
| `refused`, `another request in flight` | 1 | another `compact` or `interrupt` is waiting on the same session |
| `failed` | 1 | the harness raised something else: final, like a refusal, and no letter follows; or, marked `open`, the request went out and no outcome came — the command's wait ran out or was cut short once the request was taken, or the served side lost the request on its way — and for a compaction, which may still run, its letter comes |

A refusal carries the host's own error text as its detail and names the next action. Under
`--json` the model is printed either way: `session`, `action`, `focus`, `outcome`, `reason`,
`detail`, and `tokensBefore`, `tokensAfter` where an answer carries them. An answer's `open`
tells a failure that leaves the outcome undecided from a final one. A refusal the host gives
at once — mid-turn, too short when it comes before the start is seen — is the command's
answer, and no letter follows.

## The control directory

One per run, like the sockets, so a request written for a run that ended never reaches the
next holder of the name:

```
<REWAKE_DIR>/v2/rooms/<room>/control/<name>.<epoch>/  0700
  lock                    flock of the asker; a second asker is refused, not queued
  request.json            {id, action, focus, from, ask, choices}, 0600, put in place by rename
  <id>.taken              written by the served side the moment it picks the request up
  <id>.result             {id, outcome, reason, detail, open, tokensBefore, tokensAfter}, written once
```

The wrapper makes the directory before the harness starts and removes it when the session
ends, only for a harness that offers a Control verb: a directory nobody serves would say `not
answering` where the true next step is a restart. If it cannot be made, or the path found
there is not a private directory of this user — a symlink, another user's, one open to the
group or the world, which the wrapper refuses rather than repairs — the launch goes on
without one, and every request to that run is refused as `no control directory`. The id is
32 lowercase hex characters, and the served side refuses any other: it builds file names from
it.

The served side may be able to write files but not remove them, so the asker owns every
removal (`internal/core/control`):

1. Take the lock, and remove everything a dead asker or an expired request left.
2. Write the request.
3. Wait up to the **pickup** limit, 5 seconds, for `<id>.taken` or `<id>.result`. Nothing:
   remove the request — withdraw it — and only then look for the mark once more. Still no
   mark: `not answering`.
4. Taken: wait up to the **outcome** limit, 10 seconds, for a result that parses and
   carries the request's id. A file that does not parse yet is one still being written and
   is read again. The request stays in place meanwhile; the served side checks it (below).
5. Remove the request, the taken mark and the result on the way out. No result in time:
   `failed`, marked `open`, saying the request was taken and may still be carried out; its
   late result is cleared by the next request.

**Giving up is honest in every order.** The served side, having read a request, first writes
its mark and then checks that the request is still in place: carried out only if it is,
answered `withdrawn before it was taken` if it is gone or replaced by another. The asker,
giving up, first withdraws the request and then looks for the mark. Of the two, the one that
looks second sees what the other did first:

- the served side checked before the withdrawal: it carries the request out, and the asker,
  looking after its own withdrawal, finds the mark and waits for the outcome;
- it marked before the asker's last look but checked after the withdrawal: it does nothing,
  and the asker waits for and reports `withdrawn before it was taken`;
- it marked after the asker's last look: the asker reports `not answering`, and the served
  side, finding the request gone, does nothing either; what it writes is cleared by the next
  request.

So the command never reports `not answering` for a request that was carried out, and adds no
limit of its own: the check is one more read before acting.

**An asker cut short** — an Esc or Ctrl+C on main ends its Bash tool's command with SIGTERM,
and SIGKILL 1.5 s later if it is still alive
([research.md](research.md#interrupting-a-turn-commands-and-the-stop-hook)), or the call is
terminated otherwise — stops waiting on SIGTERM or SIGINT, withdraws the request on the way
out and reports that the wait was cut short: refused as `cut short` before a pickup, `failed`
after one. So a stalled target that resumes later finds nothing to carry out. The withdrawal
waits for nothing, so it fits in the 1.5 s.

## The served side

Every adapter serves the directory through `control.Serve` in its wrapper, which carries each
request to the harness:

- **Claude Code**: the wrapper hands the request to the mod on its heartbeat's answer
  ([design-claude.md](v2/design-claude.md#control)). The mod refuses a compaction during a
  turn, answers `requested`, runs the native compaction with the focus, and reports its
  result — tokens before and after, or the native reason it was skipped — for main's letter.
  A request the mod does not acknowledge within 5 seconds is `not answering` if it was never
  handed over, an open `failed` if it was.
- **pi**: the per-run extension carries it over its socket
  ([research-pi.md](research-pi.md#telemetry-and-remote-control)). A manual compaction
  refuses an active turn, answers `started` when the native preparation hook is seen or
  `requested` after one second, and records the final result with native before and
  estimated-after counts. Automatic compaction being disabled does not disable this manual
  request. A missing acknowledgment leaves an open outcome and is not repeated.
- **The fixture** serves both verbs through its program, and the shared command and letter
  workflow case (`fixture-steered`,
  [testing-cases.md](testing-cases.md#a-stop-and-steering-a-session)).

An interrupt waits for the matching aborted turn — a subagent's end cannot settle it — and
records its requester: the `stopped` report says "`<main>` interrupted this turn with rewake
interrupt" instead of the person-at-the-keyboard text, and routing is unchanged: it goes only
to the sessions waiting on that worker. The same words go into the harness's own history
without starting a turn. Main's `rewake inbox --awaited` shows the task as `stopped:` followed
by the stop's own text, so it names who stopped the turn.

Main's wrapper sends a notice "Rewake: context compacted (compaction N)." for each compaction
of a worker, and skips one whose cue names this main as the asker: the letter carries the
tokens and the count instead. A compaction the worker makes itself, or one another main asked
for, keeps its notice.

## The letter

When a compaction the command answered `started`, `requested` or an open `failed` ends, main's
wrapper puts a letter into main's inbox, from the worker, of kind `notify`:

- "Rewake: compacted `<worker>`: N tokens before, M after (compaction K)." — the counts when
  the harness gave them, and the session's count of compactions with this one;
- "Rewake: `<worker>` refused the compaction you asked for: `<reason>` (`<detail>`). Nothing
  was compacted." — a refusal after the answer, such as a conversation too short once the
  host had started;
- "Rewake: the compaction of `<worker>` you asked for failed (`<detail>`)." — the host's error
  after the start.

The letter reuses the note kind the compaction notice has, with its `compaction` metadata
when the count is known. A note owes nothing (`mail.Owed` is false for it), a blocking
`--question` takes only a message in reply to it, and the letter replies to nothing, so
nothing that tracks reports treats it as one.

**Where it comes from.** The served side records how the compaction ended in the worker's
telemetry snapshot (`compactionOutcomes`: request, asker, outcome, reason, detail, tokens,
time; the last 16 within a 3 KiB budget, each detail cut to 300 bytes, so the snapshot stays
under its 16 KiB limit with 64 compaction events beside them — a snapshot over it would not
be saved at all). Claude Code's mod, pi's extension and the fixture's program all record it
through their adapters. A final answer — a refusal, a final failure — is recorded as the
outcome too, after it is written: a command whose wait was cut short once the request was
taken holds an open answer, and its letter then comes from this at once rather than at the
bound. Main's wrapper reads each worker's snapshot once a second for the compaction notices;
it pairs the outcome with the compaction the telemetry counted under the same request and
sends one letter for the pair. A done outcome waits up to 3 seconds for its count, and a
count for its outcome; after that the letter goes with what came and says the telemetry has
not counted it. A refusal or failure has no count to wait for. A worker that leaves sends
what it has at once. The letter's id is derived from both runs and the request, so a scan
repeated after a failed write sends it once.

**Who owes it.** Main's side, not the worker's: the worker's side may never record an
outcome — a worker killed mid-compaction, a served side that lost its wait. So `rewake
compact` leaves a record in main's state (`<REWAKE_DIR>/v2/rooms/<room>/letters/<main>/<id>.json`:
the request, the worker's registry record, main's run, the time) before it writes the
request, with the request's id chosen beforehand; a command killed after the worker took the
request leaves it too. The command removes the record itself on every answer main already
has in full — a refusal, a final `failed`, the request never written. Only an answer that
leaves the outcome open keeps it and promises the letter: `started`, `requested`, and a
`failed` marked `open`.

The command holds an exclusive `flock` on the record from before it gets its name — the file
is locked, then renamed into place — until it has decided whether the record stays, and
main's wrapper leaves a held record alone; otherwise a worker leaving during the pickup could
get a "left before the compaction ended" letter while the command answered a final `not
answering`. The kernel drops the lock when the command dies however it dies. A lock counts
only when the name still leads to the locked file, and the record is read again under it: the
command may remove the record and let go between the wrapper's open and its lock.

Main's own wrapper closes each record kept exactly once, by the first of:

- the outcome in the worker's snapshot, as said above — the snapshot stays readable after the
  worker has gone;
- the worker's run gone with neither the outcome nor the count seen 3 seconds after its
  departure was seen — its last snapshot may come a moment later, and is read again
  meanwhile: "Rewake: the compaction of `<worker>` you asked for failed (`<worker>` left
  before the compaction ended: `<reason>`)." It comes after the notice that the worker has
  gone;
- **15 minutes** from the request with neither seen while the worker lives on: "Rewake: no
  outcome of the compaction of `<worker>` you asked for was seen within 15m0s; rewake list
  shows whether it compacted." An outcome of `started` is no outcome: the record waits on for
  the compaction's end, which the worker records as a second outcome of the same request. If
  the bound comes first, the letter says what the worker said: "… had no outcome within 15m0s
  (`<detail>`); rewake list shows whether it compacted."

The record goes once its letter is in main's inbox; a letter whose write failed is retried
with the same id. So the promise in the answer holds: a letter comes, whether it compacts or
not. A command that could not leave the record says so instead of promising — "rewake could
not note it for its letter (`<error>`), so none comes: rewake list shows whether it
compacted" — with the error under `--json` as `noLetter`.

The letter lands in main's inbox as any letter does, and is announced as one. The records
are kept by main's name, not its run: a letter to an ended run is never shown
([delivery.md](delivery.md#reading-rewake-inbox)), so a record main's run left behind is
closed by the next run of that main when it starts, to the new run, and the letter adds "You
asked for it in an earlier run of this session."

## Known limits

- **An asker killed outright** (SIGKILL) cannot withdraw its request: a target stalled at
  that moment carries it out when it resumes, telling nobody. The next request clears what
  is left.
- **A worker whose wrapper ends just after its compaction**, before its snapshot is next
  written — within a quarter second — takes the outcome with it: the 3 seconds main's wrapper
  waits after a departure cover a snapshot published late, not one never published. The
  letter then says the worker left before the compaction ended, while `rewake list` of that
  run would have shown it compacted.
- A compaction with no word of it within 15 minutes gets main's bound letter, and its later
  end gives no second letter.

## Owner decisions

- Only main may call these commands (September 24, 2026).
- A compaction is refused at once while the worker is in a turn; it never waits for idle and
  never happens on its own (September 23, 2026).
- The behaviour is the same and predictable on every harness (September 24, 2026).
- When main asked for a compaction itself, it gets no "context compacted" notice for it: the
  tokens before and after and the session's count come with the outcome, in the letter.
  Compactions the worker makes itself, and ones anyone else asked for, keep their notice
  (September 24, 2026).
- `rewake compact` does not block: it returns as soon as the request's outcome is known, and
  the result comes to main later as a letter; `rewake interrupt` stays as it is (September
  24, 2026).
