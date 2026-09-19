# Native start-or-steer and terminal outcome evidence

September 19, 2026. Owner decision: after failed or interrupted work, wake only for
new messages. Old announced work must never replay; manual continuation or inbox
peek is not required for new mail.

## Evidence

**[Owner native TUI capture, pinned CLI 0.154.0; inspected September 19, 2026]**
Executable SHA-256:
`3188814c35471432d4123203e0eb38e5bddc60226e3d7ddf0e59e649ea140022`.
The preserved `delivery-outcomes-owner-0inuifgr/gateway-metadata.jsonl` capture in
the native delivery proof has these zero-based rows for the selected primary:

- 101: `thread/status/changed` with `systemError`; 105: `turn/completed` with
  `failed`. No later idle for that turn was observed.
- 157: `thread/status/changed` with `idle`; 158: `turn/completed` with `interrupted`.

The native-order precheck reproduces both sequences against the revision-4 archive;
its three race-enabled repetitions fail. These are existing owner-captured metadata
and isolated protocol regressions, not a new live acceptance run or transcript read.

## Native start-or-steer, not a dispatch terminal gate

Final owner clarification, September 19: ready messages reach active work between
tool calls and wake idle work. The orchestrator's interpretation of "between turns"
as a full native turn/completed boundary was wrong. Revision 6 removes that gate.
The observed terminal orders above remain valid outcome evidence, not dispatch
prerequisites. Completion tracking is retained for task reports and causal boundaries.

**[Source rechecked September 19, 2026; snapshot 44b9011, CLI 0.154.0]**
`app-server/src/request_processors/turn_processor.rs:648-677` calls
`start_or_steer_turn` for `turn/start`, handling Started and Steered outcomes.
`core/src/codex_thread.rs:323-334` defines that operation as accepting input without
requiring the caller to inspect thread state. The native operation chooses active
steering or idle start. The wrapper must not add a status-versus-start race, force
interruptions or require a guessed turn ID through a separate steer-only request.
Previously recorded owner same-turn and fresh-thread delivery probes are linked
from [session-owned server research](research.md#session-owned-app-server).

The wrapper submits each ready eligible group through that existing API. An active
turn can ACK multiple notices with the same turn ID; ACK is acceptance, not proof
of model consumption. Each notice keeps its own member IDs, count and preview.
No terminal or inbox overview is required before the next ready dispatch. Input
arriving after dispatch cannot retroactively change an accepted notice.

The removed progress and overview-counter tests encoded the superseded gate. Their
replacements verify prompt active/idle delivery, exact mixed groups, readiness and
ACK overlap, and no old-unread replay. Normal/error/stopped report tests still cover
native terminals before and after ACK, including the owner-observed status orders.
Original revision-5 archives and failure proof remain unchanged. The
[grouped inbox contract](inbox-groups.md) governs readability and dispatch.

## Installed acceptance

[September 19 revision-6 acceptance](inbox-acceptance.md) confirms later mail was read
between the first and second tool calls of an active four-step task. Its original
finished report and a subsequent idle task/report were preserved. This is scoped
live consumption evidence, separate from synthetic ACKs and earlier terminal traces.
