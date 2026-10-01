# Stage M2 of the mail tool: the server, built and accepted

The second of the three stages of [mail-bridge.md](../mail-bridge.md) — the MCP server
that runs the mail tool outside the sandbox — was built on October 1, 2026 and accepted
the same day on the Codex side after four acceptance rounds, against rules agreed after
four review passes of the design ([mail-bridge-server.md](../mail-bridge-server.md),
[mail-bridge-turns.md](../mail-bridge-turns.md),
[mail-bridge-checks.md](../mail-bridge-checks.md)). No harness starts the server yet:
its injection at launch is stage M3.

## What was built

- **The server** (`internal/bridge/server`, the internal command `bridge-serve`): an
  MCP loop over stdio with bounded frames and ids, one encoder that holds every message
  to the result cap, four calls at a time, and one child per call that runs the CLI
  words and is never restarted; a child that died is answered from its call's binding.
- **The wrapper's endpoint** (`internal/bridge/endpoint`, run by `internal/wrap/mailtool.go`):
  hellos checked for uid, capability, descent from the harness and the same build;
  one ticket per native call, issued only while its turn is open and confirmed once;
  the acknowledgment of a read from a used ticket with its binding; and the gate that
  orders acknowledgments against the end the wrapper captures.
- **The adapters**: the Codex gateway follows the primary thread's tool calls and
  captures its turn ends through the gate; Claude Code's collector captures an
  interruption through it, and the hook `bridge-hook` reports the tool's calls.
- **What stage 1 gained**: a call's binding (`receipts/<run>/calls/<call>`), a pending
  mark judged against the turn ends on record, a tool read acknowledged under the gate,
  and one file seam for the inbox's reads, renames and removals.

## How it was checked

The tests and what each proves are in
[mail-bridge-checks.md](../mail-bridge-checks.md#how-the-checks-are-built). The fault
test takes its 331 cases from the logs of 13 clean scenarios. The order test is generated
in three spaces: one call taken apart into its steps against its turn's end taken apart
(182 orders and 60 with the result lost, 274 cases), the call table under pressure and
age (1,050 orders), and two turns (180 orders as 390 cases). Twenty-two mutants of the
main guarantees were each killed by a test in the build — among them the gate letting an acknowledgment write after the end,
a capture that samples again, a second ticket for one call, a ticket of a closed turn,
an acknowledgment without a binding or for a changed answer, the result cap, the hello
checks and the adapters taking another thread. The uid check was not mutated: one user
cannot test it.

The reviewers' probes of the four acceptance rounds that the tests did not already
cover became tests of their own: the 2 s wait for an observation, the last ticket a run
remembers issued once, no ticket while a capture waits for a writer, a completion whose
binding cannot be read spent, an outlived call naming its retry, a reader that resumes
in time, and a lost answer of an effect found again rather than repeated.

One defect of stage 1 was found and fixed on the way: a call whose final receipt could
not be written after its effect was done answered that the step "was not taken"; it now
says the step was done and names the `retry` that records it.

## The design passes

Four review passes of the rules came before the code
([what each found](../mail-bridge-checks.md#what-the-first-review-found)). The first
accepted the transport without mail state, the ticket only after a native observation,
the result cap and the stage boundary, and found seven defects — among them a boundary
sampled after the end's waits, a Stop taken for the end of its turn, waits without a
total bound, and evidence by bytes contained rather than by the whole answer — and the
rules were rewritten. The second found a turn start taken as written before the next
turn's calls, and one budget promised for both the lock wait and the whole
acknowledgment; the third, a boundary that could take in the next holder's read, and a
pending mark that needed the wrapper's memory. The fourth accepted the rules, and the
build began.

## The acceptance rounds

- **First.** Three defects reproduced by the reviewer's probes: a ticket issued between
  a turn's capture and `turn/completed` read above the frozen boundary; a repeated
  completion retried an acknowledgment its budget had dropped; a used call forgotten by
  the full table got a second ticket. A ticket is now stamped under the gate's mutex and
  only while no end was noted since its call was heard (on Codex, since its turn
  started); a completion is spent when first heard; the calls given a ticket are kept
  for the run, up to 65,536. The order generator was taken apart into each call's and
  each end's steps, with a generated space for the table, so that a mutant putting back
  each defect is killed by the generated tests; the check of what precedes the binding
  became an exact list, and the doc's claim that the gate proves the acknowledgment's
  budget was corrected.
- **Second.** At the end of stdin the server waited without a bound for a child whose
  output something held past its kill. Once stdin ends, a killed child is waited for one
  second more, its call answers that its outcome is unknown, and the server exits
  without it (`child_bound_test.go`, six cases).
- **Third.** A client that kept stdout open without reading it stopped the server before
  it read the end of stdin, and a blocked answer held the wait for the calls. stdin is
  now read apart from the dispatch, up to 16 frames ahead, and output stuck for 2 s once
  stdin ended, or while input backs up behind it, ends the server as one killed
  outright; an answer that never left reads nothing (`stdout_bound_test.go`).
- **Fourth.** Accepted: no new defect in the transitions checked; the reviewer's last
  probes — a reader that resumes before the bound, and a lost answer of a published
  effect — pass and are now tests of the tree.

Each fix of a round came with a mutant that puts the defect back and is killed by the
generated or seam tests, eleven in all over the four rounds, beside the twenty-two of
the build.

## What stays for stage M3

- Injecting the server at launch, keeping every other server and setting of the harness.
- The refusal of a launch when the person already has a server named `rewake`.
- The channel record — whether the tool or the shell carries a session's mail — with its
  notices and display, moved to stage 3 by main's decision of October 1, 2026; stage 2
  keeps the server's outcomes (ran, refused, transport failed) as defined results.
- The live checks on both harnesses
  ([mail-bridge-checks.md](../mail-bridge-checks.md#left-to-stage-3-and-live)).

## What stays open

- **The window before the capture on Codex**: a turn the app-server starts before the
  gateway reads the last `turn/completed` can commit a read inside that end's boundary;
  older than stage 2, queued in [work-queue.md](../work-queue.md#now-after-100).
- **The run's limit of 65,536 tickets**: past it every new call of the run is refused,
  rather than a used call forgotten and served twice.
- **Backpressure on a client that never reads**: while stdin is open and nothing backs
  up, an answer waits on a full stdout without a bound of its own, holding its slot;
  at worst new calls answer "busy".
- **Cases not generated**, with the reason for each, are listed at the end of
  [How the checks are built](../mail-bridge-checks.md#how-the-checks-are-built).
