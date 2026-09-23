# Turn outcomes other than a report

What reaches the sessions waiting on a turn when that turn does not simply finish: it
failed, a person stopped it, or the session marked it as not the end of the work. The
ordinary path — a turn end, its `finished` report, and how waits are recorded and
settled — is in [delivery.md](delivery.md#the-end-of-a-turn).

## Failed turns

After a failed or interrupted turn, a session is woken only by new mail: what was
already announced is never announced again, and new mail needs neither a manual
continuation nor a peek to reach it. Owner decision, September 19, 2026; the evidence
is in [native-terminal-progress.md](native-terminal-progress.md). This holds for the
keyboard stops below as well.

The hidden hook emits error with the harness reason unchanged. Received empty
completion after read work produces an error with empty text. Successful turns
still produce finished. Error is in the kind catalog but has no send flag;
`send --error` is refused. It creates no reply obligation.

Errors go to all live waiters for this run; with none, they go to the room's
main. A failing main, or a room with no main, retains the error in the failing
session's own unread mailbox without announcing it to itself. Identified turns
are idempotent; callbacks without turn ids are separate invocations. Delivery uses failed task-notification status on the socket path and a red circle on
the server path. A waiting question consumes the error and exits 1.

Failure observation depends on the harness providing a callback. The current
server backend observes terminal failures directly. Hooks on other transports
still depend on the harness providing a callback.

## Interim turn ends (`rewake pending`)

Owner decision, September 23, 2026: a turn end stays the report by default. A session
about to end a turn that has not finished the work — background work still running —
runs `rewake pending "<what it waits for>"` first. That turn end then reaches every
waiter as a `pending` message carrying that text, instead of the report; it owes
nothing, settles nothing, and the waits stay open. The command itself answers one line,
`Rewake: marked pending; at this turn's end <senders> will read that the work goes on.`
The next turn end without a mark is the report, as usual. A forgotten `pending` gives the behaviour without it, never worse;
a `finished` marker the worker had to remember would have left the obligation open for
ever when forgotten, which is why it was not chosen.

- **The mark** is `inbox/<name>/pending/mark.json`, changed only under the mailbox lock.
  It belongs to the turn it was made in and to no other, and the tie is time on the
  machine's boot clock (`CLOCK_BOOTTIME`, `internal/boottime`), one clock for every
  process that a wall-clock jump after a WSL suspend does not move back. The mark records
  when the `rewake pending` process started; a turn end honors it only if that lies
  between the ending turn's start and end. A mark older than the turn's start was made in
  an earlier turn whose end never reached rewake — an Esc interruption, a hook whose
  payload never came, a mailbox lock not taken in time — and is removed unused, so the
  next turn end is the report. A mark newer than the turn's end was made in a later turn
  while this one was still being published, and is left for it. A turn whose start is not
  known cannot be tied to the mark and ends as a report. Every failure falls to that side:
  the behaviour without `pending`. A second `pending` in one turn replaces the text.
- **The turn's start and end.** Codex: the gateway stamps `turn/started` and
  `turn/completed` as it sees them and carries both with the completion, so a turn
  published late — queued, or retried when the mailbox was busy — cannot take a mark made
  in the turn after it. Claude Code: the end is the moment the Stop hook's `rewake
  turn-ended` started; the start is the latest UserPromptSubmit, which fires for typed
  input and for mail delivered through the inbox socket alike. That hook runs in the
  background and must not wait, so it takes no lock: `rewake observe` records its own
  process start as a file of its own, named by the reading and never overwritten, in the
  directory `sock/<name>.<epoch>.obs.turn/`, and the start read back is the largest name
  there — two hooks at once cannot leave the older reading on top. A late hook of the
  current turn therefore records an earlier time than the mark's and cannot clear it; a
  turn started after the mark always records a later one. Every turn end that reaches
  `rewake turn-ended` also records its own end there, so the next turn is known to start
  after it even without a UserPromptSubmit. Without any reading — the telemetry hooks are
  not running — `rewake pending` is refused on Claude Code.
- **The gap that remains.** If a turn end with a mark is lost — an Esc, a stdin or lock
  timeout — and Claude Code then starts a turn by itself, without UserPromptSubmit, and
  that turn's end is the first one heard, its final answer leaves as an interim message
  and the report does not come. Not verified live: whether Claude Code fires
  UserPromptSubmit for a turn it starts by itself, to report a background task. Recording
  every heard turn end narrows the gap to that one sequence.
- **Both harnesses** take the same path from there: the Stop hook through `rewake
  turn-ended`, and Codex's completion from the gateway, into the one function that
  prepares a turn's reports, which asks for the mark once per turn end.
- **Only a normal finish is softened.** A failed or stopped turn reports `error` or
  `stopped` as it would have; the mark is used up by it all the same.
- **The message** has its own report id (the wait's id plus `-pending-<turn>`), like
  `stopped`, so the report that follows is not taken for a copy. A blocked `--question`
  does not take it for its answer: it is a turn outcome for being kept readable when
  its notice fails (`inbox.IsReport`), but not one that settles (`inbox.Settles`), and
  a question waiting in the sender's session leaves it to be announced. The notice reads
  `Rewake: <session> pending, …` with the text as its preview; Claude Code draws it as
  status `running`, which it has no color for, and Codex marks it with ⏳.
- **Refused** outside a session, for main — whose turns are reported to nobody — without
  text, and when no read task or question in this run waits for a report; each refusal
  says why.

## Keyboard stops

A stopped outcome is advisory and hook-only. It goes to the current waiters,
otherwise to main, using the text "the person at the keyboard stopped this turn".
It owes no reply and has its own report id, separate from the eventual result.
Its separate advisory turn receipt keeps the original waits intact. A finished
or error outcome for the same native turn uses its final receipt and can settle
those waits; retries of either outcome remain idempotent. Human continuation can then
publish finished with the same inReplyTo and settle those waits. Main waits for
the person instead of resending. A waiting question prints stopped and exits 1;
the later result remains an ordinary inbox report. Socket notices use killed
status; server-delivered text notices use a yellow circle.

Answer receipts store the report id in received/<question id>. Only an exact
match confirms that report; stopped does not acknowledge a later finished with
the same inReplyTo. Empty legacy receipts are unqualified and do not confirm a
new outcome. Their existing reference-based retention and expiry still apply.
