# A task sent into a long Codex compaction — September 26, 2026

A live run on Codex 0.155.1 lost a task for good: main compacted a worker whose context
was 85% full and sent it its next task, the compaction ran about 104 seconds, and at
80 seconds from the request the mark's bound ended both the hold of deliveries and
main's wait for the end. The task went into the running compaction, the server refused
it with `ActiveTurnNotSteerable { turn_kind: Compact }`, and the delivery failed with
"not retried automatically"; main was told the compaction had failed, and was told
nothing when it succeeded ([research-codex.md](../research-codex.md#a-delivery-during-a-long-compaction)).

## Why it was lost

- **One bound for two waits.** `markHoldLimit`, 80 seconds from the request in
  `internal/harness/codex/gateway/steer.go`, bounded the hold in `dropStale`
  (`activity.go`) whether or not the compaction's turn had been seen: a compaction
  visibly running was released the same as one the server never started.
- **Any refusal of a delivery was final.** `DeliverChecked`
  (`internal/harness/codex/server_delivery.go`) made every error of the native request
  `failed`; the server's refusal for a compaction running, which takes nothing, was
  treated like any other.
- **The wait ended as a failure.** `waitEnded` answered main `failed` whatever it had
  seen, main's wrapper sent the letter from that outcome at once, and the compaction's
  later end reached no one: the mark was finished, and the end had nowhere to go.

## What was done

Both ways of keeping the task, since either alone leaves a gap: the hold covers what
the gateway sees, and the refusal covers what it does not — a compaction lost sight of,
or one running past any bound.

- **Two bounds.** A mark not yet tied to the compaction's turn is held 80 seconds from
  the request, as before: that bound is for a server that never starts the compaction.
  Once the compaction's item ties it, the bound is 10 minutes (`compactionRunLimit`),
  several times the longest compaction seen, so the hold lasts to the turn's end.
  Both are build values (`builtCompactionStart`, `builtCompactionRun`); a resume whose
  snapshot shows the compaction no longer running releases a tied hold early.
- **The refusal is a wait.** `Deliver` (`gateway/reservation.go`) takes the server's
  `ActiveTurnNotSteerable { turn_kind: Compact }` for `ErrCompacting`, and
  `DeliverChecked` makes it `pending`: nothing was taken, and the message is tried again
  every two seconds until the compaction ends. The same refusal for a review still fails.
- **An honest outcome at the bound.** A tied mark whose wait ends answers main
  `started` — "it may still be running, and its end is reported when seen" — not
  `failed`; an untied one is still `failed`, its turn not seen. The mark stays, and
  when its turn ends the gateway records the end as a second outcome of main's request
  (`recordLateEnds`), with the tokens.
- **main's letter waits past `started`.** `closeRequest` (`internal/wrap/compaction_letters.go`)
  takes an outcome of `started` for none yet and waits for the end; `letterBound` went
  from 5 to 15 minutes to stay past the 10-minute wait, and a bound letter after a
  `started` outcome says what the worker said.
- **The fixture refuses as the server does.** The shim answers a `turn/start` during its
  compaction with the 0.155.1 refusal and logs the refusal and the compaction's end. The
  new workflow case `codex-compact-hold` runs a compaction past the start bound and one
  past both, with four product mutants
  ([testing-plugin.md](../testing-plugin.md#a-long-compaction-on-codex)).
- Documents: [remote-control-codex.md](../remote-control-codex.md),
  [remote-control.md](../remote-control.md#known-limits),
  [remote-control-letter.md](../remote-control-letter.md),
  [delivery.md](../delivery.md), [research-codex.md](../research-codex.md),
  [traps.md](../traps.md), the testing documents and the two shortened waits in
  [testing-pool.md](../testing-pool.md#waits-the-suite-shortens).

## Evidence

- Unit tests: `internal/harness/codex/gateway/compact_hold_test.go` — a tied mark holds
  past its start bound to its end; a compaction ending past the running bound answers
  `started` and is recorded with its tokens and asker when it ends; the Compact refusal
  is `ErrCompacting` and the Review one is not.
  `internal/wrap/compaction_letters_running_test.go` — no letter for `started`, the
  letter from the end; the bound letter after `started`.
- `codex-compact-hold` alone: one pass in 22 s; slow's task went with no refusal,
  slower's was refused twice before the compaction's end and then went.
- Its four mutants each broke only the observation they aim at: `hold-ends-at-start`,
  `compaction-refusal-final`, `late-end-unrecorded`, `running-taken-for-an-outcome`.
- The full workflow suite with the new case: 122 cases, 119 pass and 3 unsupported for
  a named capability, 3m48s. Its first run failed the shape case: the hook that ends
  the shim's compaction was counted as an event; the check now skips it.

## What stays open

- **A review refuses input the same way** (`turn_kind: Review`), and that refusal still
  fails the message: nothing holds a delivery during a review, and a review's end is not
  tracked as a compaction's is.
- **The refusal is recognized by its text**, which is the Debug form of a Rust enum in
  0.155.1, and the same in 0.157.1 (`turn_processor.rs:675`, `turn_input.rs:659`,
  `error_code.rs:6`, read by the Codex-side acceptance); a later Codex that words it
  otherwise makes it final again. The fixture's
  refusal carries the 0.155.1 text, so the suite does not notice such a change.
- **The 10-minute bound is a guess from two points**, 4–9 seconds on an empty
  conversation and 104 seconds on a full one. Past it the task goes, is refused and
  waits — no longer lost — but main's wait has ended as `started`.
- **A compaction that ends while the terminal is away** releases its hold on the
  terminal's return, but main learns its end only at the bound: the resume asks for no
  turns, so nothing says how it ended
  ([remote-control-codex.md](../remote-control-codex.md)).

## Review

The Claude-side review and the Codex-side acceptance of the commit accepted neither,
for two defects, one gap and three documents; fixed in the next commit.

- **A silent server held the terminal for 10 minutes** (regression, confirmed by a probe:
  with a 200 ms start bound and a 3 s running bound, the terminal's `thread/read` reached
  the server after 2.5 s). The command's context became the 10-minute wait for the end,
  and the same context bounded `thread/compact/start` in `callReserved`, which holds the
  admission gate every request of the terminal's waits for. The request now has its own
  context bounded by the start bound (`steer.go`, `g.markLimit()`), as the 80-second
  bound did before. `TestASilentCompactionRequestHoldsTheTerminalOnlyToTheStartBound`
  goes red without it.
- **A late `started` hid the end** (found by both, reproduced deterministically).
  `server_steer.go` publishes `later()`'s answer after the connection's lock is let go,
  and the compaction's end, recorded under that lock by `recordLateEnds`, can come
  first: the outcomes read `[done, started]` or `[failed, started]`, and `halves` took the
  last, so main waited for its bound. Now a `started` after a final outcome of the same
  request is dropped by `CompactionEnded` (`telemetry.go`), and `halves`
  (`internal/wrap/compaction_letters.go`) lets no `started` stand over a final outcome in
  either order. `TestALateStartedDoesNotStandOverTheEnd` (the review's reproduction),
  `TestAStartedAfterAFinalOutcomeIsDropped` and `TestAStartedRecordedAfterTheEndDoesNotHideIt`
  go red without the two changes.
- **A resume ends the hold, not the wait.** Finishing the wait from the resume's reply
  was not simple: the terminal's resume asks for no turns, so the reply carries no
  status of the compaction's turn. Written down as a limit instead, above.
- Documents: the letter's bound for a Claude Code compaction is 15 minutes, not "several";
  the terminal's `/compact` sets a mark and holds deliveries the same way, and a repeated
  one is refused for as long as main's runs; the request's own bound. The limits of
  [remote-control-codex.md](../remote-control-codex.md) moved to
  [remote-control-codex-limits.md](../remote-control-codex-limits.md) when it passed 400
  lines.
