# A pending turn end that keeps the turn's text — September 26, 2026

Two losses of one day, both around `rewake pending`
([turn-outcomes.md](../turn-outcomes.md#interim-turn-ends-rewake-pending)):

- **A turn woken by background work closed the task.** A worker ended a turn with the
  mark while its subagents ran. The first subagent to finish woke it; it wrote "waiting
  for two more" and ended that turn without a mark. That turn end was the report, and
  the task was settled with the rest of the work still going.
- **Findings written into a pending turn were lost.** A worker wrote its analysis in the
  turn's answer and ended the turn with `rewake pending "the analysis is in this
  answer"`. The interim message carried the mark's text in place of the turn's, so main
  received that one line.

A read-only review session looked at the mechanism and weighed five options; main chose
two of them the same day.

## What was done

- **(d) The interim message carries the turn's text.** `internal/cli/turn_reports.go`:
  the mark's text is the first line, as before, so the notice's preview does not change;
  a blank line and the turn's own text follow when the turn said anything. One path for
  both harnesses. A failed or stopped turn, and a blocked `--question`, are untouched.
- **(a) The briefing says it for every turn.** The rule in the write and general
  playbooks (`internal/role/playbook.go`) now says it holds for a turn a finished
  subagent or background task woke as well, and that the turn's text goes with the mark,
  so findings can stay in the answer.

## Evidence

- `internal/cli/pending_test.go`: `TestAPendingTurnEndKeepsTheTaskOwed` and
  `TestALatePublishedTurnIsAReportWithItsOwnText` expect the mark's line, a blank line and
  the turn's text, and fail on the old code; `TestAPendingTurnEndWithoutTextIsItsMark`
  keeps a turn that said nothing to the mark's line alone.
- The workflow case `pending-report` requires the interim message to begin with the
  mark's line and carry the turn's text, in both columns, and has a third mutant,
  `pending-text-dropped`, which restores the old replacement and must break only that
  observation.
- The briefing snapshots in `internal/brief/testdata` were regenerated.

## What was not done

- **(e) A Stop hook that asks once, on Claude Code — awaiting the owner's decision.**
  After an interim turn end, a turn that ends without a mark while waits are still open
  would be held once by the Stop hook (`{"decision":"block"}`, with `stop_hook_active`
  guarding the second call) and asked: still waiting — mark it; done — end the turn
  again. It closes the first loss without a marker the worker must remember, at the cost
  of one more model step per episode. Not built: it needs the owner's decision and a live
  check that a block from rewake's settings layer works and what
  `last_assistant_message` holds on the second call. Codex has no such hold through the
  gateway.
- **(b) A mark that lasts until an explicit "done" — rejected.** It reverses the owner's
  decision of September 23, 2026: a forgotten "done" leaves the obligation open for
  ever, a `--question` waits out its `--wait`, and `--owed` and `--awaited` show pending
  indefinitely.
- **(c) A turn not woken by mail does not settle — rejected.** The ordinary final report
  is exactly such a turn, woken by the last subagent to finish, so it would need a
  "done" marker too, with (b)'s faults; and telling a self-started turn from a delivered
  one on Claude Code is not reliable — whether UserPromptSubmit fires for it is not
  verified.

## What stays open

- **(e)**, above, until the owner decides.
- **Someone else's Stop hook that blocks.** `rewake turn-ended` does not look at
  `stop_hook_active`. If a person's own Stop hook blocks the stop, rewake has already
  reported on the first Stop, with the text before the block, and settled the waits; the
  turn's continuation then finds nobody waiting, and main never reads what came after.
  Not observed, found by reading; to be decided with (e).
