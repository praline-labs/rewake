# Startup transport repair

September 18, 2026. The installed integration failed the owner's ordinary and
resumed terminal startup: repeated reconnects, unavailable conversation and
read-only/input-paused indications. Earlier native checks exercised the prototype
or a production-library helper, not the complete installed wrapper. That
installation was not successful live acceptance.

## Reproduced close cause

Both transport readers used nonblocking channels with eight slots. A ninth queued
request or response immediately closed the connection. Scheduling delay, an
admission holding the request gate, or a busy terminal writer could therefore
turn a short startup burst into transport failure. Reconnection retained that
trigger; routing became unavailable as a consequence of connection loss.

An isolated run of the exact installed binary reproduces a disconnect during 56
concurrent startup metadata requests. A diagnostic-only build, retaining the old
queues, identifies `tui-to-server / request-queue-full`. The same full wrapper
sequence passes after the repair. Two deterministic socket regressions fail on
the reviewed pre-repair source: requests queued behind admission, and responses
queued behind a busy terminal writer. Both pass after the change.

The owner also ran the diagnostic-only build through the full wrapper and real
terminal in fresh isolation: ten seconds without reconnect/read-only/flicker,
then `/quit`, exit zero, no model requests, and only peer EOF after exit. Thus
clean native startup alone does not reproduce the reported working-environment
incident. Queue overflow was a proven defect, but attribution to the original
incident remained unconfirmed until the subsequent working-environment size log
described below. The isolated pass alone did not fix that live incident.

## Bounds and diagnostics

The initial queue repair permitted 256 queued messages and 16 MiB of retained frame data,
including the message being processed. Both bounds apply: maximum-size history
frames cannot consume the item limit times the frame limit. Requests stay in
order behind admission; approval replies still bypass that queue. Readers do not
wait on queue capacity. Sustained overload still closes the connection, with a
diagnostic, rather than blocking approvals or growing without bound. The existing
4 MiB individual-message limit remained and caused the subsequent failure below.

The adjacent private `<socket>.gateway.log` records the first close cause per
connection: local connection/generation counters, direction, stage, allowlisted
error category, frame size when known, and sampled queue counts. These counts
can change between overflow and recording. No RPC bodies, method values,
thread/request identifiers, histories, configuration values, completion text or
secrets are logged. Unknown errors become a fixed category. `<socket>.up.log`
remains native server stderr.

## Verification boundary

Regressions cover request/response bursts, FIFO ordering, approval bypass with
occupied admission, both queue budgets, released byte capacity, and redacted
first-cause diagnostics. Routing, primary/side, receipts and read-scope tests remain.

The full-wrapper fixture runs the command entry point, registration, read clock,
native backend, startup probe, gateway, terminal child and cleanup. Its automatic
terminal child is a synthetic RPC client; owner mode uses the real native terminal
through the same wrapper. Both have fresh home/state, isolated mount/PID/network
namespaces and a local model endpoint. Automatic startup starts no model turns
and checks registry/socket cleanup. The native executable is pinned in the repair
evidence; source inspection is not asserted to match that executable exactly.

A passing synthetic-client run is not terminal or registered-peer acceptance.
The later owner acceptance record below distinguishes proven startup/report smoke
behavior from the remaining main/side check. This repair does not infer focus,
replay work, change owner configuration or recover missed events.

## Confirmed native message-size failure and repair

September 18 follow-up: the owner tested the queue-repaired binary with their
usual configuration, a fresh conversation, separate state and no task prompt.
Its gateway log records nine consecutive server-to-terminal `read` failures with
`websocket message too large`, then peer EOF at final exit. The server stderr
log is empty. This identifies the 4 MiB gateway size guard as the immediate cause
of those disconnects; neither the exact message size nor its contents were
inspected or inferred. The clean isolated terminal pass did not cover that size.

Owner decision: keep the native 128 MiB message ceiling, independently allow
neighboring service messages in the queue, and test the exact limit and one byte
over. The gateway now applies 128 MiB consistently to individual data frames,
assembled fragmented messages, metadata projection and writes. Each queue keeps
256 slots and a separate 144 MiB capacity budget: one maximum native message plus
16 MiB of adjacent traffic. The writer's in-flight message and spare slice capacity
count until released. Sustained overload still refuses; nothing is dropped,
truncated, replayed or silently exempted from accounting.

Reads fill the assembly buffer directly, with bounded geometric growth up to
128 MiB. A fragment does not need a second full payload allocation. Writes keep
one gate/deadline across header and body; masked output uses 64 KiB scratch and
unmasked output uses the existing buffer. Opaque JSON strings use quote scanning
rather than per-byte traversal for every metadata lookup. These avoid multiplying
large-message copies; the existing message, item and connection limits remain
finite. The active reader and temporary old/new buffers during growth are outside
queued bytes, so 144 MiB is a queue capacity bound, not a total process RSS bound.

Size refusals now log only `sizeStage`, `messageBytes` and `limitBytes`, including
an advertised frame size or the accumulated fragmented size. Header overflow
refuses before allocating or reading its payload. Unknown payloads and private
configuration/history remain absent from diagnostics.

The local reference source at revision `44b9011611e1f4213ef34bd51b33476475803a94`
sets both remote-client frame and message limits to 128 MiB in
`app-server-client/src/remote.rs`. Its Unix server uses the native library's
acceptor defaults, which are not changed here. This source checkout is not claimed
to be the exact installed binary. Native tests use the previously pinned 0.154.0
executable; generated 18 MiB instructions produce a native startup/config reply
above both the old frame and old queue limits, without a model request. The old
queue-repaired wrapper disconnects with the same size error; the new complete
wrapper passes and verifies all generated bytes plus a following service reply.

Boundary tests cover masked unfragmented and unmasked fragmented messages exactly
128 MiB, metadata projection, both queue budgets with 16 MiB neighbors, capacity
release, and refusal at 128 MiB + 1 byte in reads, assembled fragments, projection
and writes. Independent review subsequently passed, including additional masked
fragment, concurrent/partial write, cancellation and escaped-string regressions.
All five checks independently passed on the same 240-file production tree.


## Owner acceptance and installation — September 19, 2026

The owner accepted fresh startup and resume of an existing conversation with the
usual configuration and separate state, using the reviewed size-repaired binary
`b3837e9beb08f0574c79c3b1b2500e11ae41d811910b4a885e2d65017a383bc6`.
Both were stable without flicker, reconnect or read-only errors. Preserved fresh/
resume assessments and gateway logs show only peer EOF at exit; no task was sent
during these startup checks. This closes the observed startup failure for those
owner-tested paths, not every native workflow.

The owner then installed the reviewed build and restarted all three sessions.
A short reviewer task returned the automatic final report `NEW-REVIEW-OK`.
The complete main-delivery/main-finished check while side stays open is still
pending. Side output must not settle main waits. No claim of that acceptance is
made from the startup checks or single short-task report.
