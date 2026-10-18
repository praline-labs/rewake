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

- **E1. An operation whose effect is unknown is never discarded and never bypassed.**
  A receipt goes only when its effect is proven absent or proven done; a step not
  recorded as done is no proof it was not done; age decides nothing — who continues an
  operation is decided by its scope or its receipt. (M1 rule 1.)
- **E2. A lock is removed only by whoever holds it**, and a call that took a lock checks
  the file at the path is still its own. (M1 rule 2.)
- **E3. Every check that decides an effect runs inside the critical section, right
  before the effect, and again on a retry**, which is a new call with its own deadline
  and transport and the same limits. (M1 rule 3.)
- **E4. A read's completion is one durable fact every channel shares.** A letter leaves
  `unread/` only by being read once a part of it was shown, and never comes back; a late
  acknowledgment finds it read and marks nothing. (M1 rule 4.)
- **E5. The size bound holds on the final encoded bytes of every answer, in one place**;
  every text part names its letter, part, count and byte range. (M1 rule 5.) Since
  October 18, 2026 no answer is cut: the bound and its parts were the mail tool's, and the
  shell prints a read whole.
- **E6. A check has three outcomes: found, proven absent, unknown.** Only "no such file"
  proves absence; an unknown stops the effect and leaves the operation where a retry
  takes it up. Readers that only inform fold an unknown into their most cautious answer.
  (M1 rule 6.)
- **E7. Every effect has an immutable identity and a proven scope, and recovery advances
  it only from durable evidence tied to that identity.** (M1 rule 7, clauses identity,
  clock, journal and scope.)
  - A turn end is one operation — publish its reports, take the kept answer they carry,
    clear the waits they answer, record whether the end was interim — named by its run
    and its event, never by an attempt; everything it writes is named from that.
  - Its scope is fixed once, before its first effect, from evidence the event carries.
    The read boundary is a position on the run's read clock, which numbers every read
    and every hold; a hold reserves its position, keeps the answer, then commits the
    clock, and a position is never issued twice.
  - The turn's window on the boot clock picks the pending marks that make it interim,
    opening after the turn's start or the latest earlier end a journal records,
    whichever is later; no end removes a pending mark, which lives as long as its run.
  - Every 2.0 TurnBoundary names its event (a turn id), so the 1.x case of an end heard
    once with no event goes; an end that names an event and carries no boundary still
    takes no effect on any attempt, and its waits stay owed.
  - What the operation will do is written down first (the journal), and nothing but the
    journal publishes. A stop after an interruption goes on taking its held answer (the
    owner, September 30, 2026).
- **E8. Each effect is proven done, proven not done, or unknown; an unknown stops every
  change to its mailbox until evidence settles it.** (M1 rule 8, clauses evidence,
  stop, proof, retention and reconcile.)
  - Done is proven only by evidence written after the effect under the operation's
    name: the journal's entry, the recipient's `published` mark, the letter in the
    recipient's mailbox, a completed journal. Not done is proven only where the protocol
    makes absence visible: a report with no mark, an `intent` mark and no letter, a kept
    answer or interim record still in place. Everything else is unknown.
  - An unknown stops the mailbox: records kept, nothing published, cleared, adopted or
    read; the stop is a record (`stopped`) naming its exact cause and path. Letters from
    others still arrive, recovery writes its own records, main is told once.
  - What a stop is decided by is read before any effect: the barrier and every changing
    call walk the mailbox against one list of record kinds, then run their effects as a
    plan that writes nothing, through the one file seam both passes use; only a plan that
    meets no unknown lets the same code run for real.
  - A stop the plan found goes once the plan finds its cause gone; one only an effect
    met goes once a barrier has run every effect through.
  - The barrier runs under the mailbox lock before a turn end reads a wait: plan; collect
    the obligations proven publications closed; decide every other report — superseded,
    published, or unknown and stopping — and complete the journal. (The 1.x steps 2, 3
    and 5 — conversion, successor, held — go.)
  - Proof lives as long as anything could replay it; a journal is kept while its run may
    retry it, an unfinished one for good, and nothing that names no run is swept by age.
  - **A stop is lifted only by returning evidence, never by removing its cause.**
    Removing a record whose effect is unknown does not make the effect absent: the next
    plan would read "no such file", stop seeing the cause, and publish without the
    evidence E1 asks for. So the stop and every record it names stay until evidence
    returns — in the ordinary case by fixing what made the record unreadable (its mode,
    its directory, the disk). No command goes around the barrier; the operator decision
    below neither removes a record nor counts as evidence.

### An operator decision on an unknown outcome

Accepted by the owner on October 5, 2026 (answer 4 of
[design.md](design.md#the-owners-answers)), as a bounded extension beside E1–E8, not a
part of E8. It is a **conscious choice under an unknown outcome, not evidence**: a
decision proves what an operator chose, not whether the effect happened. Decided "did
not arrive" wrongly, the report is published twice; decided "arrived" wrongly, an
obligation closes with nothing delivered. That is the boundary of the guarantee it
trades away, and the reason it is kept apart from E8's proofs.

The contract: a command by the report's recipient — main, who can check whether the
report arrived — or the person at a shell records "arrived" or "did not arrive" for an
operation whose outcome is unknown, as an **operator decision** record that names the
operation by its identity, states the delivery fact as **unproven**, and keeps the
original records and the stop's evidence untouched, for good. "Did not arrive" sends the
report again; "arrived" closes it. Any effect taken after it — a publication, a closed
obligation — carries the decision's id and never claims delivery. The decision applies
only where the operation's identity — report, recipient, obligations — is known from
something readable, as 1.x `settle` read them from a readable conversion journal
(`inbox/reconcile.go:285`); a stop whose unreadable record is itself the only source of
that identity has no subject to decide and stays. The authority (who is verified, and
how), the rules for a retry across the decision, and its tests are designed in stage 3's
rules; until they land, nothing lifts a stop without returning proof. The 1.x migration
that `settle` served (`cli/settle.go:28`, `inbox/conversion_decide.go:209`) does not
return.

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
  rule 1; on Claude Code, one `--plugin-dir`.)
- **L2. Diagnostics say where, never what**: no configuration content or argument value
  is printed. (Rule 7.)
- **L3. Nothing persists past the run**: what a launch writes lives in the run's
  directory and goes with it; after a kill, a later launch removes it once the run is
  proven dead ([the qualified guarantee](../archive/v2/stage3-rundir.md#the-qualified-guarantee)).
  (Rule 8.)

`mail-bridge-launch.md` rules 2, 4, 5 and 6 are about injecting an MCP server and checking
its name; they return, redesigned, with the Codex adapter (stage 5).
