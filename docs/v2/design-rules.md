# Design: the core rules

The numbered rules 2.0 carries, restated without the 1.x migration (decision 3) and
without a harness. Which 1.x rule each comes from, and why the others went, is in
[revision-rules.md](revision-rules.md). The full wording of each rule moves into the
2.0 rules documents in stage 3 with its tests; this file fixes what each says. The
overview is in [design.md](design.md).

Four groups, each with its own letter so a rule is cited without its document: **E**
the effects of a mail operation (from `mail-bridge-cli.md` 1–8), **T** every tool
transport (from `mail-bridge-server.md` 1–11), **C** the channel record (from
`mail-bridge-channel.md` 1–8), **L** what a launch adds (from `mail-bridge-launch.md`).

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
  every text part names its letter, part, count and byte range. (M1 rule 5.)
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
    its directory, the disk). No command goes around the barrier; this is the whole
    accepted rule, with or without the option below.

### Not accepted: an operator decision on an unknown outcome

An option for the owner (question 4 of [design.md](design.md#the-owners-answers)),
outside E1–E8 until the owner accepts it and it has its own design and review. It is a
**conscious choice under an unknown outcome, not evidence**: a decision proves what an
operator chose, not whether the effect happened. Decided "not done" wrongly, the report
can be published twice; decided "done" wrongly, an obligation closes with nothing
delivered. That is the boundary of the guarantee it would trade away, and the reason it
is not part of E8.

If accepted, the contract is: a command by the room's verified main or the person at a
shell writes an **operator decision** record that names the operation by its identity,
states the delivery fact as **unproven**, and keeps the original records and the stop's
evidence untouched, for good. Any effect taken after it — a publication, a closed
obligation — carries the decision's id and never claims delivery. The decision applies
only where the operation's identity — report, recipient, obligations — is known from
something readable, as 1.x `settle` read them from a readable conversion journal
(`inbox/reconcile.go:285`); a stop whose unreadable record is itself the only source of
that identity has no subject to decide and stays. Before it lands it still needs the
owner's decision, the authority's design (who is verified, how), the rules for a retry
across the decision, and its acceptance tests. Until then nothing lifts a stop without
returning proof. The 1.x migration that `settle` served (`cli/settle.go:28`,
`inbox/conversion_decide.go:209`) does not return either way.

Gone with the migration: 8-withheld, 8-settle, 8-cutover, 8-origin, the earlier-build
evidence and the run and successor records ([revision-rules.md](revision-rules.md#mail-bridge-climd-rules-18)).

## Tools, T1–T11

These bind **every tool transport** — the Claude Code mod, a Codex MCP server, any
next one. The transport is each adapter's choice; the guarantees are not. (Restated
from `mail-bridge-server.md` 1–11 after the second revision review, R1.)

- **T1. A transport decides no mail.** It carries a call to the core and the answer
  back, and keeps nothing a later call needs; what a call did lives in the receipts.
- **T2. A call runs only under a trusted binding.** The binding names the conversation,
  turn and call as the harness itself reported them, the time, the deadline and the
  digest of the normalized arguments. Nothing the model supplies — an argument, a field
  it could shape — is authority on its own. One native call gets one binding. A ticket
  issued by the wrapper after it saw the call natively is how 1.x held this on Codex;
  [design-claude.md](design-claude.md#how-a-tool-call-runs) says how the mod holds it.
- **T3. Every effect happens in the core operation, under its receipt**; a transport
  never repeats, finishes or undoes one, and starts no second operation for a call
  whose first still runs.
- **T4. A call is bound to its operation before its first effect**: a record keyed by
  the native call, whose absence ("no such file") proves the call made no effect. Only
  on that proof may the answer send the same words to the shell.
- **T5. Everything read and written is bounded, in one place each.** A request is
  bounded before it is parsed; every answer passes one encoder and one bound on its
  final bytes; an answer that does not fit is replaced whole, never clipped.
- **T6. A letter is read only on proof that the call's own whole result reached the
  model.** The proof is the harness's record of the result for that native call, equal
  to the answer the core recorded before returning it, direct, successful, and inside
  the harness's **effective** result limit — the limit under which the harness passes a
  result whole rather than a preview. Text that merely contains a letter proves nothing.
  A reading tool whose transport cannot give that proof **refuses before its first
  effect**, and the shell (`rewake inbox`) stays the way to read. A child process that
  marks read on printing is no way round this: under a tool call its output is not yet
  shown to the model (`cli/inbox.go:114-119`; persisted output seen on 2.1.289,
  `.scratch/v2-recon/claude-mods.md:52`).
- **T7. A call's commits stop at its turn's end, and a pending mark speaks only for
  its own turn.** Two conditions, both needed:
  - **Not ended.** A pending mark or a read's acknowledgment checks, under the mailbox
    lock, for an end of the run on record at or after its time, or a turn start
    recorded after it, and in the host for ends it has heard and not yet recorded;
    found, nothing is committed and the letter shows again.
  - **Its own turn, proven.** A pending mark is written only by an attempt proven to
    run in the turn its operation was made in: the attempt that created the operation,
    or a later one whose binding names the same conversation and turn on a transport
    whose turn ids are proven never reused. No absence proves the turn still open —
    of a later turn's start, of an end. An attempt that cannot prove it does not mark:
    the outcome is **unproven**, the mark is finished as not made, and the answer says
    why (`cli/pending_turn.go:10-31,65`: `inOwnTurn`, `markUnproven`). This binds the
    shell, `retry` and every tool transport alike: a `retry` from a later turn never
    writes an earlier turn's mark. Until an adapter proves its turn ids are never
    reused — on Claude Code across an interrupt, a reload, `/clear` and resume (P3,
    P9) — only the first attempt counts there, as 1.x counts it for `prompt_id`.
  The carried test includes a lost end with a `retry` from another turn.
- **T8. An end's boundary is a cut between commits**, taken at the end's own event —
  the clock when no acknowledgment is between its check and its close, or the snapshot
  the acknowledgment takes after its last write under the lock. Each acknowledgment is
  applied once.
- **T9. Every wait has a bound, and nothing waits holding what it waits for**; a wait
  that runs out leaves a defined outcome. A write begun under a held lock is not timed.
- **T10. One build per room.** The wrapper, whatever runs the core for a call, and the
  CLI of every session of a room are one build — one artifact, by the hash of its
  content, not by its version line; every writer holds the room's build lease while it
  acts, and anything of another build is refused before it acts
  ([design-state.md](design-state.md#one-build-per-room)).
- **T11. A failing tool never weakens the mail.** The shell takes the same words under
  the same receipts; the surface, the read boundary and the refusals are the CLI's.

The carried fault and order tests (`bridge/server/fault_test.go`, `order_cut_test.go`,
`order_gen_test.go`, `bridge/endpoint/order_table_test.go`) hold T2–T8; their oracles
become transport-neutral and run against every transport
([design-docs-tests.md](design-docs-tests.md#tests)).

## Turn outcomes

- **An interim wake must not close a task early.** A turn woken after `rewake pending`
  that ends without a new mark, while its senders still run, is asked once before its
  end closes: the core keeps the answer and hands the session a reason to go on; the
  next end publishes that answer with its own, and every failure of the check falls to
  publishing, the behaviour without it (`cli/turn_hold.go:13-24`; held by
  `cli/turn_hold_test.go:80,122,149` and `test/workflow/pending_confirm_test.go:25`, with
  negative controls at 177 and 181). The mechanism is the adapter's TurnBoundary
  ([design-api.md](design-api.md#turnboundary)).
- **Failed and interrupted turns** report as in 1.x (`docs/turn-outcomes.md`, "Failed
  turns", "Interim turn ends"), with the reason taken from the adapter's end event.

## The channel record, C1–C8

Core and harness-free (revision, R3): no branch on a harness id in `core/channel`; what
one adapter needs beyond the neutral events lives in the adapter.

- **C1.** Per channel — tool, shell — one observation with its own evidence and time,
  plus a policy block; the state shown is derived, never stored apart.
- **C2. Connected is not working.** A transport's hello proves it reached the host; only
  a call that met its binding proves the tool carried one.
- **C3. Silence proves nothing.** No hello, no call, no shell command are not failures;
  a wait names what was not observed, never what did not happen.
- **C4.** Each channel has one kind of evidence: the tool's, a call bound (T2); the
  shell's, a mail operation that wrote under a lock or failed to reach it.
- **C5.** Events fold by when they happened, not by when they arrived.
- **C6.** A notice states an action first and fits the preview.
- **C7. A policy refusal is not routed around** (the owner, October 4, 2026) — a managed
  policy that keeps mods off included.
- **C8.** Nothing is relaunched, granted or approved by the channel.

## What a launch adds, L1–L3

- **L1. The mail is added and nothing else changes.** The person's configuration is
  never edited; what a run needs is passed for that one launch. (`mail-bridge-launch.md`
  rule 1; on Claude Code, one `--plugin-dir`.)
- **L2. Diagnostics say where, never what**: no configuration content or argument value
  is printed. (Rule 7.)
- **L3. Nothing persists past the run**: what a launch writes lives in the run's
  directory and goes with it. (Rule 8.)

`mail-bridge-launch.md` rules 2, 4, 5 and 6 are about injecting an MCP server and checking
its name; they return, redesigned, with the Codex adapter (stage 5). Rule 5's guarantee
— the effective limits decide a read — is T6.

## Grants

The three steps of the unforgeable grant carry over unchanged (`docs/grants-authority.md`,
"The scheme": at send, at delivery, when main does not answer), in `core/grant`
with the wrapper's side in `host`. Applying a grant is the Permissions capability.
