# The Stop hook asks once after a pending turn end — September 26, 2026

Option (e) of [the pending-text record](2026-09-26-pending-text.md#what-was-not-done),
approved by the owner on September 26, 2026. Three tasks were closed early that day the
same way: a worker ended a turn with `rewake pending`, a finished subagent woke it, and it
ended that turn without the mark; the turn end was the report. The briefing asks for the
mark on every such turn, but that is a rule the model has to remember. The owner's
decisions of September 23 stand: a turn end is the report by default, and there is no
mark that lasts until an explicit "done".

## What was done

- **The hold** (`internal/cli/turn_hold.go`). `rewake turn-ended` holds a turn end on
  Claude Code when it is a Stop with `stop_hook_active: false`, the run's last published
  turn end was interim, a sender whose session runs still waits, the turn made no mark,
  and no answer is kept from an earlier hold. It then publishes nothing, keeps
  `last_assistant_message`, and prints `{"decision":"block","reason":"…"}` quoting the
  pending line and naming the senders. The rules are in
  [turn-outcomes.md](../turn-outcomes.md#the-confirmation-on-claude-code).
- **The records** (`internal/inbox/confirm.go`): `interim.json`, written by an interim
  turn end and removed by a finished or failed one, and `kept.json`, the held answer;
  both beside the pending mark, under the mailbox lock, tied to the run's epoch.
  `inbox.MarkedWithin` asks what `TakePending` would answer without taking the mark.
- **The five amendments main set, from review-claude's reading and the writer's:**
  1. The held answer goes into the report: the next turn end heard puts it before the
     continuation, or after the line of a failure, a stop or a new pending mark
     (`internal/cli/turn_reports.go`). Without it main would receive only what the model
     said after the hold, since the second Stop carries nothing else.
  2. `stop_hook_active: true` is never held, nor is any turn end while an answer is kept:
     one hold per turn. The interim record belongs to the epoch and goes with a finished
     or failed end.
  3. A held end records no turn start (`internal/cli/turnended.go`), so a mark made in the
     continuation stays within its turn.
  4. An Esc after a hold: the plugin's `stopped` carries its line and then the kept
     answer. Where the plugin did not load, the kept answer goes with the next turn end
     heard, which is not held again.
  5. Someone else's blocking Stop hook: left as it is, and written down in
     turn-outcomes.md — rewake reports on the first call and the continuation's text
     reaches nobody; the hooks of one event run side by side, and nothing tells rewake
     another hook will block.
- **The live facts** it rests on, probed that day on Claude Code 2.1.280 in a private
  HOME with a stand-in API, are in
  [research-claude-control.md](../research-claude-control.md#a-stop-hook-that-holds-the-turn).
- Codex is untouched: its turn end cannot be held through the gateway, and there (a) and
  (d) remain.

## Evidence

- `internal/cli/turn_hold_test.go`: the hold and its report with the held answer first;
  a continuation that marks pending, and the next hold asking about the new line; a stop
  and a failure after a hold carrying the answer; an answer kept and never continued
  going out with the next end, not held again; and no hold after a report, with a mark
  in the turn, on a failure, on `stop_hook_active: true` or without it, from Codex, or
  with nobody waiting. The hold is checked to leave the turn's start where it was.
- `internal/inbox/confirm_test.go`: the records belong to their run; `MarkedWithin`
  agrees with `TakePending` and takes nothing.
- The Claude Code fixture answers a block as the harness does — asks its model again and
  runs the Stop hooks with `stop_hook_active: true` and the new answer alone, up to eight
  times — and can mark pending in the continuation (`RW_SHIM_PENDING_ON_HOLD`).
- The workflow case `pending-confirm`, Claude Code column, with the mutants
  `pending-unconfirmed` (never holds; breaks all three observations) and
  `confirm-answer-dropped` (the continuation alone; breaks the two about the messages).
  `pending-report` still passes in both columns, its Claude Code report now coming after
  one hold.

## What stays open

- Not run live with rewake itself: the probe used its own hook. A live run of `rewake
  claude` with a worker that forgets the mark would show the model's answer to the
  reason's wording.
- The listing shows a held session idle until its next event
  ([turn-outcomes.md](../turn-outcomes.md#the-confirmation-on-claude-code)).
