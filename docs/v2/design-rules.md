# Design: the core rules

The numbered rules 2.0 carries, restated without the 1.x migration (decision 3) and
without a harness. Which 1.x rule each comes from, and why the others went, is in
[revision-rules.md](../archive/v2/revision-rules.md). The full wording of each rule moves into the
2.0 rules documents in stage 3 with its tests; this file fixes what each says. The
overview is in [design.md](design.md).

Groups, each with its own letter so a rule is cited without its document: **E** the
effects of a mail operation (from `mail-bridge-cli.md` 1–8) and **L** what a launch adds
(from `mail-bridge-launch.md`); **T**, every tool transport, and **C**, the channel record,
left with the mail tool on October 18, 2026.

## Effects, E1–E8

Since October 18, 2026 the guarantee rules E1–E8 are not carried: the owner decided to
remove the frozen mechanics they described — the barrier, the unfinished turn journals, the
mailbox stop and the proofs of done and not done — and to replace them with a simple turn
end. A turn end writes each report into its recipient's mailbox unless a letter with that
id is already there, then clears the waits it answered, and writes its journal last; an
operation that meets an unreadable record fails with an error and is retried, and a
report is lost only if a process is killed between two steps, in which case main asks
again. The rules that remain are [rules/effects.md](../rules/effects.md); the turn end is
in [delivery-turn-end.md](../delivery-turn-end.md#the-end-of-a-turn).

### An operator decision on an unknown outcome

Removed on October 18, 2026 with the stop it disposed of: there is no `rewake decide`,
no operator decision record and no authority that verifies one, because no unknown outcome
stops a mailbox any more. A report that did not arrive is asked for again by main. See
[rules/effects.md](../rules/effects.md) and
[delivery-turn-end.md](../delivery-turn-end.md#the-end-of-a-turn).

Gone with the migration: 8-withheld, 8-settle, 8-cutover, 8-origin, the earlier-build
evidence and the run and successor records ([revision-rules.md](../archive/v2/revision-rules.md#mail-bridge-climd-rules-18)).

## Tools and the channel record

The guarantees of every tool transport, T1–T11, and the channel record of a run, C1–C8,
left with the mail tool by the owner's decision of October 18, 2026; their last wording is
archived ([rules-tools.md](../archive/rules-tools.md), [rules-channel.md](../archive/rules-channel.md)).
One build per room, which T10 restated for a tool call, holds for every writer
([rules/state.md](../rules/state.md)).

## Turn outcomes

- **An interim wake must not close a task early.** A turn woken after `rewake pending`
  that ends without a new mark, while its senders still run, is asked once before its
  end closes: the core keeps the answer and hands the session a reason to go on; the
  next end publishes that answer with its own, and every failure of the check falls to
  publishing, the behaviour without it (`core/mail/turn_hold.go`; held by
  `core/mail/turn_hold_test.go` and `test/workflow/pending_confirm_test.go`). The mechanism is the adapter's TurnBoundary
  ([design-api.md](design-api.md#turnboundary)).
- **Failed and interrupted turns** report as in 1.x (`docs/turn-outcomes.md`, "Failed
  turns", "Interim turn ends"), with the reason taken from the adapter's end event.

## What a launch adds, L1–L3

- **L1. The mail is added and nothing else changes.** The person's configuration is
  never edited; what a run needs is passed for that one launch. (`mail-bridge-launch.md`
  rule 1; on Claude Code, one `--plugin-dir`.) A worker of Claude Code also gets rewake's
  worker profile, launch options alone, by the owner's decision of October 19, 2026
  ([worker-profile.md](worker-profile.md)).
- **L2. Diagnostics say where, never what**: no configuration content or argument value
  is printed. (Rule 7.)
- **L3. Nothing persists past the run**: what a launch writes lives in the run's
  directory and goes with it; after a kill, a later launch removes it once the run is
  proven dead ([the qualified guarantee](../archive/v2/stage3-rundir.md#the-qualified-guarantee)).
  (Rule 8.)

`mail-bridge-launch.md` rules 2, 4, 5 and 6 are about injecting an MCP server and checking
its name; they return, redesigned, with the Codex adapter (stage 5).
