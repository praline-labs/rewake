# Live checks of messaging on Codex, and Claude Code probes — September 26, 2026

The day's changes — actions on a sent message, the pending text, the compaction hold,
remote interrupt — had passed their workflow cases against fixtures. This entry records
what real harnesses did with them the same evening. The evidence — controller sources,
terminal output, RPC metadata from a passive relay, mailbox snapshots — is kept outside
the repository; no account files and no native transcripts are part of it.

## Codex 0.155.1

Two real sessions, a subject and a sender, both on gpt-6-luna at effort low, confirmed
in their launch arguments, TUI and telemetry. A private room, state directory and
workspace; room `default` and its sessions were not touched. The TUI was driven through
a pseudo-terminal; the relay forwarded frames unchanged and logged methods, ids, item
kinds, errors and token counters only. The quota indicator moved from 100% to 97% left
over the three runs.

**Run 1, blocked.** The subject was launched from inside the reviewer's own Codex
sandbox, and every command it tried failed before starting: the nested sandbox could
not open its mount registry lock ([traps.md](../traps.md#a-codex-session-started-inside-codexs-sandbox-runs-no-command)).
No scenario message was sent. On main's decision the next runs passed
`--sandbox danger-full-access` to the subject sessions only, for that launch.

**Run 2, rewake build 0ee310e.**

| Check | Result | What was seen |
| --- | --- | --- |
| A group of notes, a task and a `--to` addendum during one active turn | PASS | three notes as one grouped notice, the task and the addendum each steered into the running turn — every reply named the same turn; all five ids read in it; one `finished` naming both task ids |
| `withdraw` and `edit` while the worker waits, before it reads | PASS | the withdrawn task left `--awaited` at once, its action was not done, no report named it; the edit's replacement was read and reported alone, the original's action not done |
| A/B: an addendum after the worker switched conversations | PASS | the TUI refuses `/new` while a task runs (`'/new' is disabled while a task is in progress.`); after a `pending` turn end `/new` went through, the addendum reached conversation B, and one report with `threadChanged: true` closed both ids |
| A delivery at the start of a keyboard turn | PARTIAL | mailbox first, keyboard steered 38 ms after `turn/started`; keyboard first, delivery 12.6 ms after `turn/started` and 16.7 ms after the start's reply: each read and reported once, nothing lost or doubled. The window before the server acknowledges the start was not hit, and no turns were spent trying |

**Run 3, rewake build fa5ece0.**

| Check | Result | What was seen |
| --- | --- | --- |
| A task sent into a running compaction | PASS | `rewake compact` answered `started`; the task was sent 2.96 s before the compaction ended, held, and went as `turn/start` 29.5 ms after its end; one letter to main, `Rewake: compacted subject-codex: 16913 tokens before, 4734 after (compaction 1)`; one report, the compaction's turn not counted as it |
| `rewake interrupt` during a `sleep 20` | PASS for the turn | `turn/completed` `interrupted` 27 ms after `turn/interrupt`; one `stopped`, `sender-codex interrupted this turn with rewake interrupt`, no `finished`; the task stayed `stopped` in `--awaited` |

The interrupt's side finding: the `sleep 20` ran on after the turn ended and wrote its
end marker 20 seconds after it started. An interrupt on Codex 0.155.1 does not stop a
command the turn already started
([remote-control-codex-limits.md](../remote-control-codex-limits.md)).

**Not checked by these runs.** The compaction was short, 3.6 seconds, so the 80-second
bound for a compaction not yet seen, the 10-minute bound for one seen running, a
`pending` delivery retried after the server's `ActiveTurnNotSteerable { turn_kind:
Compact }`, and a letter for a compaction that outlived the wait were not reached; they
rest on the workflow case `codex-compact-hold`. A keyboard interruption on Codex, a
refusal for a stale target, and the start window above are not covered either.

## Claude Code 2.1.280

Probes in a private HOME, the model API played by a stand-in on the loopback address.
The stand-in streams text and calls no tools.

- **A Stop hook that blocks** — the live check the Stop-hook confirmation was waiting
  for ([pending-text](2026-09-26-pending-text.md#what-was-not-done)). From a layer given
  with `--settings`, `{"decision":"block"}` holds the turn and the model is called
  again; the second Stop carries `stop_hook_active: true`, and `last_assistant_message`
  holds only the continuation's text. The ninth block in a row is overridden and the
  turn ends; `CLAUDE_CODE_STOP_HOOK_BLOCK_CAP` moves that limit. An Esc during the
  continuation fires neither Stop nor StopFailure
  ([research-claude-actions.md](../research-claude-actions.md#holding-a-turns-end-from-the-stop-hook)).
- **`rewake claude -w probe`** launches, registers and takes deliveries; Claude Code runs
  in `.claude/worktrees/probe` while `rewake list` and the session record show the
  launch directory, and `/exit` removes the worktree and its branch
  ([research-launch.md](../research-launch.md#a-worktree-at-launch)). The report at the
  turn's end was not checked: a model that calls no tools never reads the message, and
  nothing is owed.
- **Two traps**: the first start in a fresh HOME cloned the official plugin marketplace
  over SSH, and `REWAKE_ROOM` exported for a launch put a session in `default`
  ([traps.md](../traps.md#a-probe-in-a-fresh-home-cloned-the-plugin-marketplace-over-ssh)).

## What stays open

- The `cwd` of a `-w` session: the record keeps the launch directory
  ([design.md](../design.md#session-record)). Recorded, not fixed.
- The Stop-hook confirmation (e) still waits for the owner's decision; what it needed
  to know from a live run is now known. Two answers bear on its design: the harness
  gives up after eight blocks in a row, and the second Stop's `last_assistant_message`
  lacks the reply written before the block.
- The live gaps named above: the long compaction paths, a keyboard interruption on
  Codex, the unacknowledged start window, a Claude Code report from a `-w` session with
  a model that reads its mail.
