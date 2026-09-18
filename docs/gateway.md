# Selected-conversation delivery gateway

September 18, 2026. The wrapper forwards the native TUI's own WebSocket traffic
through a session-owned gateway. There is no observer that discovers loaded roots
or resumes them on the agent's behalf. Native requests and replies pass unchanged;
the gateway projects only bounded routing/control metadata and live completion
text. The terminal retains its normal process and input handling.

## Selection and reservation

A supported primary start/resume request on one TUI connection, followed by its
matching direct-input reply, establishes epoch/connection/generation/thread. Fork
selection uses the additional positive evidence described below.
Requests to switch conversations invalidate the previous generation immediately.
A-B-A is a new generation even though the thread ID repeats. Helper connections,
overview reads, activity, stored timestamps and loaded lists never select a target.
Conflicting primary connections make new delivery unavailable.

`inbox.Server` asks a transport-neutral `Reservation` before making mail readable.
It checks epoch, expiry and question leases, waits outside the mailbox lock for the
native target, then rechecks mail under the lock. The reservation runs through
reserving ledger capacity, recording the delivery thread, linking unread mail,
reading current Git roots and
the native turn ACK. Ordinary lifecycle requests cannot overtake this interval.
Approval replies bypass the admission FIFO, and the upstream reader remains free.
Unread mail can be consumed before the ACK; `read` still wins over a lost notice.
Capacity/maintenance refusals precede readability; an unused reservation returns
its ledger slot. Each reservation permits only one delivery attempt. A failed
reservation exposes no task. An unexpired report whose notice cannot be
admitted remains readable with `reportAvailable`, just like other notice failures.

Resume can authorize a bounded numeric backfill of loaded-thread metadata. New
injection waits outside both the admission FIFO and global gate until that read
workflow closes; it does not revoke an exception while native backfill still needs
it. A native settings update for the target also waits for its response before
roots are read. Readiness is rechecked after acquiring admission. The entire
reservation is bounded to three seconds. A changed target or deadline refuses
before sending; no guessed destination or automatic replay follows.

Main/write task and question deliveries read `thread/read` with includeTurns=false
inside this reservation. Only a single local environment with valid absolute
roots is eligible for additive Git metadata roots. Existing roots, policy,
approvals and profiles are preserved. Failed reads leave the grant unchanged and
add a delivery diagnostic. General, notifications and reports receive no grant.
A timed-out metadata-only read can be abandoned without closing the TUI connection;
mutating-request uncertainty still fences that connection and forbids replay.

## Outcomes and publication

An admitted-work ledger binds positive ACKs to thread/turn under the original
connection/generation. Later routing uncertainty, switching or remaining reads
cannot erase a matching finished/error/stopped outcome. Work admitted before ACK
can stage bounded candidates until the ACK identifies its turn. Manual compaction
is excluded from owed-work outcomes. Stopped is advisory; an explicit later finish
or error may still settle the original wait. Duplicate terminal results are filtered.

Observed active/idle intervals retain the existing 500 ms completion-gap diagnostic.
An interval seen while a primary request awaits its reply is preserved when that
reply validates the thread. This does not reconstruct missed history or manufacture
an outcome from disconnection. Explicit unsubscribe can also exclude a terminal
notification; missing events are not reconstructed from history. Ledger state does
not transfer to another connection.

The native reader captures a causal read boundary and enqueues completions. A
separate publisher journals pending callbacks and their stable read scopes at the epoch-specific socket path plus `.outcomes.json`, then calls the
existing report function. That function uses the mailbox lock and durable turn
receipt to freeze eligible waits, report text, recipients and question IDs before
publication. Later reads cannot enter an older completion, including before its
first receipt is prepared; [publication boundaries](report-publication.md) specify this. A failed publish retries without blocking native traffic or
changing that batch. A callback is not a persisted report until this path succeeds.
A same-epoch backend restart can reload its journal; a new run cannot borrow it.

Shutdown closes and joins gateway event producers before draining publication.
One five-second budget covers successful retries and in-flight publication; a
late callback cannot dequeue retained pending work. Failures keep the journal and
print its location. A journal is recovery evidence, not an announced inbox report. It
does not provide a daemon that retries a dead session. The in-memory queue stops
new transport work on overflow (256 items or 16 MiB) instead of growing indefinitely.
Native callback text is allowed only as live results; no transcript file is read.

## Compatibility and limits

Fresh startup and ordinary new/resume keep native arguments and configuration.
The launcher owns the upstream socket (`.up`) and gateway socket for the run;
startup initializes and closes a probe client without selecting any thread. The
TUI uses the original socket path. Backend exit terminates the TUI, and cleanup
reaps only wrapper-owned children. Role/name, room, epoch and permission contracts
are unchanged. Native version differences still warn.

The first installed integration failed live startup. [The transport repair](startup-transport.md)
replaces eight-slot burst-sensitive queues with item/byte bounds and adds private
close diagnostics. A subsequent owner log confirmed the 4 MiB size guard caused
repeated closes. The 128 MiB message / 144 MiB queue repair passed independent
review and owner fresh/resume checks in the usual environment; see the acceptance record below.

The native reconnect workflow uses a settings-preserving resume without roots.
On a new initialized connection, its explicit numeric resume may correlate with
the last unambiguous closed primary's thread ID. A matching direct-input ACK is
still required and establishes a new connection/generation. This preserve path
replays its cached view without ordinary resume backfill, so it does not arm that
read exception. Failed attempts retain
only this bounded ID anchor, not closed clients or subscriptions. Unknown targets,
an uncertain predecessor or a competing live primary cannot use that shortcut.
No resume is injected and no input is replayed. Events missed during a disconnected
interval cannot be recovered from this evidence.

Settings updates, naming, metadata and memory-mode changes do not select a new
conversation. Policy updates and additive grants remain ordered. Other unclassified
selection/control workflows fail closed; normal native frames still pass through.
See the source and acceptance boundary in [native evidence](gateway-native-evidence.md).

### Fork and side are different workflows

Owner decision, September 18: the registered address belongs to the **primary**.
The person can ask a side question while primary work continues. New messages and
its finished report must still work; a side answer must not settle primary waits,
and closing side must not require resume/new. Separate side addressing is out of scope.

The native thread/fork response alone is insufficient: both ordinary and side forks
create a child with that API. CLI fork is authorized by the wrapper's parsed launch
mode, on one initialized connection and parent, consumed once after binding. This
context cannot come from an option value or a prompt after `--`.

An in-session fork of the accepted primary suspends new admission while its kind is
unresolved. A matching direct-input child reply plus a successful typed unsubscribe
reply for the old accepted parent confirms ordinary lineage replacement. Child
hydration and optional naming may occur in between. A changed intent, failed or
unknown acknowledgement, wrong parent, closed child or contradictory control cannot
promote the candidate. Generation fences and retained outcomes still apply.

Alternatively, a matching thread/inject_items request and successful reply for that
child identify the native side setup. The still-live previously accepted parent
remains primary; the side child is tracked only to exclude its scoped activity from
selection/reporting. No prompt content or focus inference is used. Side input and
its completions never gain the primary admission ledger or consume its waits.
Primary messages remain admissible while side is open. A parent read followed by
side interrupt/unsubscribe closes the side without changing primary authority.
Only one proven side child is retained; a new primary intent clears that context.

Until the shared fork sequence is disambiguated, reservation may wait within its
ordinary finite budget. Unclassified/contradictory workflows still refuse new work;
already accepted outcomes survive. This does not add addressing for arbitrary
cached/subagent views, nor treat local rendering as the selection authority.

## Regression coverage

The reviewed protocol's 52 tests remain, including the original delivery/backfill
outcome-retention reproducer. Its deliberately unreserved injection helper is now
test-only; production injection requires a Reservation. New tests cover readiness
waits, approvals, expiry, A-B-A, native settings ACKs, reconnect cleanup and an
active/idle interval observed before the accepted-intent reply.

Old observer/discovery implementation-specific tests are replaced by these semantic
checks: no auxiliary target selection or attachment, no stale reconnect target,
closed-connection cleanup, no old-generation injection, retained completion/gap
observations and native connection usability. Git role/root/config tests and the
existing question/receipt/room/epoch suites remain. A cross-package integration
check joins gateway ACK, readable inbox, waiter recording, post-switch completion,
durable report retries, threadChanged and readability after a failed report notice.
Async publication has separate blocked-reader and journal-retry regressions. A
working-primary/side regression verifies a new primary notice while side is visible,
no side result settling its question, main finished publication before side close,
and unchanged primary binding after close. A native-server fake-TUI run separately
checks main completion after close, with the original admitted turn retained.

The isolated prototype's owner-run terminal acceptance is not production peer
acceptance. By September 19, independent integration/report/startup reviews passed,
the owner accepted fresh/resume startup with the usual configuration, and the new
build was installed with all three sessions restarted. A short task returned its
automatic final report. Main delivery and completion while side remains open still
need the full owner-run check; broader peer acceptance is not inferred.
