# Stable completion publication

September 18, 2026. The reporting boundary is independent of native RPC. The
backend receives a CompletionHandler with nonblocking Capture and context-aware
Publish functions; since stage 3's S5 also Confirm, which an adapter that can hold an
end open calls in place of Publish ([turn-outcomes.md](turn-outcomes.md#the-confirmation-on-claude-code)). The wrapper owns the mailbox/epoch association and closes it
only after backend event producers and the publisher have stopped.

## Read association before asynchronous delay

Each owed inbox read records a monotonically increasing sequence beside its
message ID in the epoch's waiter record. Reads are serialized by the existing
mailbox lock. A separate durable high watermark is reserved before writing a
waiter so a crash cannot recycle an earlier sequence. The committed sequence is
then published in an eight-byte shared mapping under the same private epoch
folder. This is local Linux file IPC, not a new socket or daemon.

The native reader snapshots that word atomically: no mailbox lock, directory
scan, file open or journal write occurs at capture. The boundary identifies the
mailbox and epoch as well as the sequence. It uses no wall-clock comparison,
file modification time, loaded-root choice or terminal-focus inference.

For native transport, capture occurs when a frame is received, before projection.
The relevant end/idle boundary follows staged terminal results through a delayed
ACK and follows gaps through their grace interval. Same-turn steering reads before
the boundary remain included. A later completion after advisory stopped has its
own final boundary; stopped itself never settles the original wait.

The queue and journal retain that boundary. Publication takes the mailbox lock
and selects only still-owed message IDs whose read sequences are at or below it.
Later reads cannot enter an earlier result, even when journal persistence or the
first receipt preparation was delayed. The original recipient epochs stay with
their waits; room/epoch and live-recipient checks continue to apply. Clearing a
subset also preserves the remaining messages' sequences.

Waiter files retain their existing epoch/since/message fields and add a parallel
sequence column. Synchronous legacy hooks retain their prior behavior. A backend
completion without a captured boundary, or a waiter without a provable sequence,
is refused and retained rather than guessed from the current mailbox. Matching
wrapper/CLI binaries and the planned session restart are required at installation.

## Durable outcome identities

A native turn keeps its normal thread/turn ID. Advisory stopped receipts use a
separate tagged key; final finished/error receipts share the final key, so retries
of each remain idempotent. A final for the same native turn can therefore publish
after stopped and settle the original eligible waits once. Existing stopped-only
legacy receipts migrate to the advisory key instead of suppressing that final.

Anonymous completion-gap counters are local to an observation incarnation. Their
publication ID includes connection and generation before crossing the adapter,
journal and receipt boundary. The gateway uses that same canonical identity for
live duplicate filtering. Normal native turn IDs are not rewritten.

Each prepared receipt still freezes the exact waiter subset and complete report
batch before publication: original text, recipient epochs, message IDs and advisory
threadChanged flags. Retrying cannot absorb a changed payload or new reads.

## One shutdown budget

The complete drain has one five-second budget, not five seconds per attempt.
The budget starts when publication is asked to stop, including if an emit is
already running. Successful emits cannot bypass the deadline by continuing the
loop. Context cancellation reaches mailbox acquisition and publication steps.

A single in-flight call is awaited through that budget. On timeout the journaled
head and remainder stay pending; a late callback cannot dequeue them after the
publisher exits. If a late write completed, the durable receipt makes recovery
idempotent. Disk failures keep their existing diagnostics; this is not a daemon
that retries a dead run forever. The queue lock protects only short in-memory
copies, not JSON encoding, filesystem writes or mailbox operations.

## Evidence

INT-1/2/3 review reproductions pass with race detection, including repeated runs.
INT-4's slow-success reproduction and a blocked in-flight emit both obey the drain
budget and retain pending work. Additional tests cover multiple same-turn reads,
late reads, busy mailboxes, cross-process visibility, counter recovery,
cross-mailbox/epoch rejection, legacy
stopped receipts, stop/error retries, adapter gap scope, delayed native ACKs and
gap grace periods. The full production check suite covers side/primary continuity,
reservation, permission, question, receipt and room regressions as well.

The original integration submission and the independent failing reproductions were
kept unchanged at the time; the package holding them was deleted on September 22,
2026, so what is written here and in the tests is what remains of them. Native helper fixtures were not rerun: their unscoped callbacks do
not exercise this mailbox publisher; these defects have deterministic wire and
cross-package reproductions. Independent re-review passed. By September 19 the
owner had installed the reviewed startup-repaired build and a short task returned
an automatic final report. The subsequent [installed primary/side check](gateway-native-evidence.md#installed-primaryside-delivery-acceptance--september-19-2026)
received a primary result while side stayed visible and another after side closed,
without resume or restart. Side output settled neither task. This accepts those
report paths without attributing every historical delay or loss to one cause.
