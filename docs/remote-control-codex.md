# Remote control on Codex

How a Codex session serves `rewake compact` and `rewake interrupt`. The commands, the
control directory and the decisions they share with Claude Code are in
[remote-control.md](remote-control.md); what the server offers for them, and what the
live run showed, in
[research-protocol.md](research-protocol.md#compaction-and-interrupt-on-request).

Codex has no plugin, and the wrapper already holds the app-server connection and knows
the running turn, so the wrapper serves the directory: `control.Serve` polls it every
100 ms from the moment the app-server is up and follows the same steps as the module —
mark taken, check the request is still in place, act, write the answer once. It acts
through the gateway (`internal/harness/codex/gateway/steer.go`), which bounds a
compaction to 80 seconds and an interrupt to 8, under the asker's own limits.

- **Compact** takes the gateway's admission gate, so no request of the terminal's is
  admitted meanwhile, and refuses as `in a turn` when the conversation is busy: a
  compaction marked, a turn the record below says runs, the terminal's `turn/start`,
  `turn/steer`, `review/start` or `thread/compact/start` in flight, or a delivery being
  admitted, or an operation the server accepted whose end has not been read — the
  last as "the gateway cannot tell whether an earlier operation has finished; compact
  from the TUI, or after the next message delivered to this session or turn typed at
  the terminal". Those are the answered turns that clear it; a goal's turn is answered to
  no request and does not. Codex would not refuse: its `compact()` aborts the running turn and
  compacts in its place, so the refusal has to be rewake's, decided before anything is
  sent.
- **Nothing to compact.** A conversation this connection started with `thread/start`
  that has run no turn since is refused as `nothing to compact`, as Claude Code
  refuses a conversation too short: Codex would compact it and spend a model call on
  nothing. A resumed conversation is always sent — the terminal may resume without its
  turns (`excludeTurns`), so an empty list proves nothing.
- **Otherwise** it marks the compaction manual, as the terminal's `/compact` is, so its
  turn is never taken for work, settles nothing and reports nothing, and sends
  `thread/compact/start {threadId}`. The compaction runs as a turn of its own; the
  answer comes when that turn completes. `tokensBefore` is the context the telemetry
  last saw, `tokensAfter` the `last.totalTokens` of the compaction turn's
  `thread/tokenUsage/updated`; with no usage seen before the answer carries neither.
- **When the request fails.** A refusal from the server is `failed` with its text, and
  the mark is laid aside; so it is when the request was never written. Any other
  failure — no reply within the wait — leaves the server free to start the compaction
  still, so the mark stays and its turn, if it starts, is not taken for work; the answer
  says it may still start, and the hold ends with it, as with any wait that ends first.
- **The mark's bound.** A mark holds deliveries, and main's wait, for 80 seconds from
  the request, or until main's wait ends if that comes first — main's answer then says
  whether the compaction's turn was seen at all, and a compaction that ended as the wait
  did is answered by its end. Past it the hold ends — deliveries go, main's answer is `failed` with the
  reason — but the mark does not: a compaction that starts late is still not taken for
  work, and main's next compaction stays refused until the operation is seen to end.
  80, not less: a compaction is a model call over the whole conversation, 4 to 9 seconds
  live on a nearly empty one and longer on a long one; the server refuses a delivery
  sent while it runs; and main's own wait is 80 seconds, so one bound ends both.
- **The terminal's `/compact` while main's runs** never reaches the server, which would
  abort main's compaction for it and pass the mark to its own: main's answer would fail
  and its turn be published as work. The gateway answers the terminal itself with a
  JSON-RPC error (`-32600`, "rewake: `<main>` asked for a compaction with rewake compact
  and it is running; try again once it ends"); a second compaction right after the first
  would compact nothing more. Once main's has ended, or its hold has, the terminal's
  passes again.
- **A delivery during a compaction** waits for it. While any compaction of the
  conversation is marked — main's, or the terminal's `/compact` — the wrapper sends no
  work into it. A message that comes meanwhile is not refused: the reservation waits
  its three seconds for the compaction's end — also when it queued on the gate before
  the compaction took it, and learns of the mark only when its wait ends — and if the
  compaction has not ended the message stays `pending` ("the conversation cannot take a message yet: a compaction of the
  conversation is running") and is tried again every two seconds, going once the
  compaction's turn has completed, once main's answer has come, or once the mark's bound
  has passed. So compacting a worker and then sending it its
  task at once is an ordinary order; `rewake send` may answer exit 3 for it, and the
  message goes a few seconds later.
- **Who asked for a compaction.** The manual mark carries the request id and the asker,
  and the telemetry puts them on the compaction it counts, so main's wrapper sends no
  notice of it and the command reads its count — the same fields the Claude Code
  module's `compact.asked` fills. The telemetry counts a compaction when its item
  completes, and writes the asker only when the turn the mark is still tied to ends: a
  tie undone before that — the turn shown to be work — leaves no author to take back.
- **Interrupt** sends `turn/interrupt {threadId, turnId}` with the id of the turn the
  record below names — also one only acknowledged in a reply of the current selection,
  known from a resume's snapshot, or named by any event of it. A turn running whose id
  no event has named yet, after a resume without turns, is `failed` with a hint to ask
  again. No turn running, the compaction's own turn, or an unnamed one while a mark still
  holds, is `no turn running` without asking the server; the server's own refusal of a turn that ended meanwhile ("no active turn to
  interrupt", "expected active turn id … but found …") is the same answer. The
  stopped outcome of that turn names the asker, as on Claude Code, whether a delivery
  started the turn or the terminal did; a person's Esc afterwards is the person's again.
- **No notice line.** Codex puts `<turn_aborted>` into the model's history itself, so
  the worker's next notice carries nothing more; the interrupt command's answer says
  which of the two a harness does (`InterruptTrace`).
- **A focus** never reaches the wrapper: the command refuses it with exit 2. A wrapper
  that received one anyway answers `failed` rather than compact without it.

## What the gateway knows a conversation is doing

Nothing the server sends proves a conversation free. It answers `turn/start`,
`turn/steer`, `review/start` and `thread/compact/start` once it has queued the work, not
once the work has started: the request ends when the submission is on the session's
channel (`core/src/session/mod.rs`, `submit_with_id`; `submit_core_op` in
`turn_processor.rs` and `thread_processor.rs`). The reply and the turn's events travel
different paths — the reply from the request's handler, the events from the
conversation's listener (`bespoke_event_handling.rs`) — so a reply may come before the
turn's `turn/started` or after its `turn/completed`. And the listener may answer a
resume before it takes the next queued submission, so a snapshot showing the
conversation idle says nothing about work accepted but not yet started.

What does prove something, read in the reference tree:

- **An end.** Only `turn/completed` naming the conversation and the turn, with status
  `completed`, `failed` or `interrupted`, says an operation has ended
  (`bespoke_event_handling.rs`, `handle_turn_complete` and `handle_turn_interrupted`).
  An inline review ends with the same frame though no `turn/started` precedes it
  (`core/src/review.rs`; `core/src/tasks/mod.rs`); its `exitedReviewMode` item is not
  the end. An idle or `systemError` status, an `error`, an item's completion, a
  snapshot's turn status and elapsed time are not ends: `systemError` comes, for one,
  from a failed `memoryMode/set` while the turn goes on (`handlers.rs`), and a resume
  keeps it (`thread_status.rs`). And an accepted operation may never end at all: a
  review that fails before its task starts sends only `error` (`handlers.rs`).
- **A later successful answer.** The session runs its submissions' handlers one at a
  time, in order (`handlers.rs`, `submission_loop`; the app-server also serializes
  them per conversation, `request_serialization.rs`), and answers `turn/start` and
  `turn/steer` only after routing them (`session/mod.rs`). So once a `turn/start` or
  `turn/steer` the gateway itself sent after an operation, on the same connection and
  conversation, is answered with success, that operation's handler has run: it has
  started or been refused, and cannot first start after. Only a success says so: input
  into a running compaction or review is refused (`core/src/session/turn_input.rs`,
  `ActiveTurnNotSteerable`, an error reply from `turn_processor.rs`), so a refused
  answer may mean the operation is running — as a live run showed for a compaction
  ([research-protocol.md](research-protocol.md#compaction-and-interrupt-on-request)). A later `turn/started` proves nothing of the kind — a
  handler spawns its turn and returns (`tasks/mod.rs`, `spawn_task`), turns start
  outside the queue, and an active goal starts one on an idle conversation by itself
  (`ext/goal`, `start_turn_if_idle`) — and neither does the reply to a compaction or a
  review, which is sent once the work is queued.

What part B guarantees, decided by main on September 24, 2026, and what acceptance
is judged by — three safety properties:

1. **rewake never sends `thread/compact/start` into a running or accepted turn.** Main's
   compaction is refused while anything runs or while an accepted operation's end is
   unread (the rule below).
2. **No outcome is ever silently dropped.** A turn shown to be work — a delivery's, the
   terminal's `turn/start` or `turn/steer`, a review, a goal's turn that does anything —
   reports its outcome as it is, also when its end came before its proof, and on
   whichever connection it ends. A turn without proof, and a run whose turn was never
   named, report as advisory. Only a turn its `contextCompaction` item shows to be a
   compaction reports nothing: it is no outcome of anyone's task. Restated by main on
   September 25, 2026, in round 8; before, it read "a work turn's report is never lost
   or withheld".
3. **A task is never settled by a compaction's turn.** Only a turn with proof of work
   settles, and a compaction's turn gives none (the publication rule,
   [codex-publication.md](codex-publication.md)).

Everything about *which* compaction is main's — main's answer, the telemetry's author,
the mark's turn — is attribution: best effort, stated below with its limits, never at
the cost of the three.

**The publication rule**, decided by main on September 25, 2026: an outcome is
published as it is only for a turn with proof of work, one without is reported as
advisory, and a turn shown by its item to be a compaction reports nothing. The rule,
the gap, and why (2) and (3) hold are in [codex-publication.md](codex-publication.md).

So the rule, decided by main on September 24, 2026, replacing the one of the round
before:

- **Uncertain is a state of its own.** An operation the server accepted — a
  `turn/start`, `turn/steer`, inline `review/start` or `thread/compact/start`, the
  terminal's, a delivery's or main's — whose end has not been read leaves the
  conversation uncertain for main's compaction, which is refused with the explanation
  above and never sent. Three things clear it: that operation's end, by the predicate
  above; a successful answer to a `turn/start` or `turn/steer` sent after it; and, for a
  compaction, the server refusing its request or the request never being written. No
  time does; no status does. The record belongs to the gateway, not to a connection:
  the server keeps an operation it queued when the terminal disconnects and runs it
  later — a disconnect drops the subscription and defers unloading the conversation
  (`thread_processor.rs`, `thread_lifecycle.rs`), and a queued review is not cancelled
  (`handlers.rs`) — so what was open on one connection stays open on the next; decided
  by main on September 25, 2026, in round 8. There it clears by a later answered turn,
  and by its own end when its turn is known. A compaction's operation knows its turn
  only through the mark, which ends with the connection, so its end on the next
  connection does not clear it (Known limits). A request that may start a turn and whose
  answer the ending connection never read is recorded as open then, with no turn, since
  the server may have queued it: only a later answered turn clears it. An operation past
  what the record holds — 64 open in one conversation — keeps that conversation
  uncertain until a turn sent after it is answered. One with open operations in 64
  conversations already leaves every conversation uncertain for the gateway's life, and
  the refusal says so: "the gateway lost count of the operations it accepted, with some
  open in 64 conversations, and cannot tell whether any has finished until this
  session's wrapper ends; compact from the TUI".
- **Deliveries are not held by uncertainty.** Only a compaction mark holds them, and
  only up to its bound.
- **The mark is tied to its turn only by that turn's `contextCompaction` item**
  (`item/started` or `item/completed`), only while it is tied to none, never to a turn
  known to be work — one a `turn/start`, `turn/steer` or `review/start` reply named, or
  one with any other item — and only while the event stream has been continuous since
  the request: no change of selection between. A tied mark never moves to another
  turn. The mark ends with its turn's end, by the predicate.
- **Losing sight.** When the selection changes before the mark was tied — a `/resume`
  away and back, or any request that resets the selection, a resume of the same
  conversation included, since rewake does not tell them apart — the server may
  have sent the compaction's item while the conversation was not selected, so a turn
  seen after the gap is not known to be it. The mark never ties then. Main's wait ends
  `failed` with "the gateway lost sight of the compaction (the terminal left the
  conversation before it started)", the hold ends, and the compaction's operation stays
  open, so the conversation stays uncertain by the rule. Whichever turn seen later is
  the compaction, it settles nothing: it has no proof of work.
- **For an interrupt, name only a turn seen in the current selection** as the running
  one, and learn its id from any event of the conversation that carries one when the
  turn runs with an unknown id. A reply to a request of an earlier selection never
  names it.

The gateway keeps three things (`internal/harness/codex/gateway/activity.go`,
`operations.go`):

- **The running turn of the selection** (`activity`): whether a turn runs and its id.
  It goes with the selection — every change of the selected conversation resets it —
  because after the terminal leaves a conversation (`/resume` of another is allowed
  mid-turn, `tui/src/slash_command.rs`, and unsubscribes from the first,
  `tui/src/app/thread_routing.rs`) the server sends this connection none of its events
  (`thread_processor.rs`), and an id read before may name a turn long ended.
- **The open operations of each conversation** (`operations`, `threadTurns`): every
  operation accepted whose end has not been read, keyed by the order it was sent in —
  numbered across all connections — with its turn when one is known; and the last
  turns read to their end. These outlive the selection and the connection.
- **The compaction mark** (`admittedWork.manual`, `mark.go`), set when the request is sent.

| frame | what it does |
|---|---|
| a `thread/compact/start` sent, main's or the terminal's | the mark set, holding; an operation open with no turn yet |
| the reply to `turn/start` or `turn/steer` naming T, the terminal's or a delivery's | every operation of the conversation sent before it closed; T open unless already read to its end; of the current selection, also T running |
| the reply to `review/start` naming R, when its `reviewThreadId` is the conversation it was asked for | R open; of the current selection, R running. A detached review answers with a conversation of its own (`turn_processor.rs`, `start_inline_review`, `start_detached_review`) and holds nothing here |
| a `contextCompaction` item of turn C, the mark tied to none, C not known to be work | the mark tied to C, and its open operation named C |
| a successful reply naming T, or an item of T other than `contextCompaction` | T known to be work, on every connection; a mark tied to T untied, its open operation named no turn again; T's outcome published, also one read before |
| `turn/started` T | T running, announced |
| any other event naming T while a turn runs with an unknown id | T running |
| `thread/status/changed` active | a turn running, its id unknown until an event names it |
| `thread/status/changed` idle | the running turn cleared only when its id is unknown, since the status goes idle before `turn/completed`; nothing open closed |
| `turn/completed` T, status `completed`, `failed` or `interrupted` | T's operation closed, T remembered as ended, its mark ended; the running turn cleared when it is T or unknown |
| the reply to a selection | what runs: the snapshot's last turn in progress — no `turn/started` follows for it — or one of unknown id when the status is active and the reply has no turns; nothing open closed |
| the server refusing a compaction's request, or the request never written | the mark and its open operation gone |
| the mark's bound passing, or main's wait ending first | the mark stops holding; main's answer `failed` with the reason |
| a change of selection, the mark tied to no turn | the mark lost: it never ties, stops holding, main's answer `failed` with the reason; its operation stays open |
| the connection ending with a `turn/start`, `turn/steer`, `review/start` or delivery unanswered | an operation open on its conversation with no turn |
| an outcome waiting half a second for its proof | its advisory report published, none for a turn its `contextCompaction` item shows to be a compaction; the outcome still waits |

A resume without turns is the terminal's ordinary one: with paginated history it sets
`excludeTurns` (`tui/src/app_server_session/rollout_history.rs`), and the server puts
the running turn in the reply only when turns are included or an initial page is asked
for (`thread_lifecycle.rs`, `handle_pending_thread_resume_request`). Until an event of
that turn comes, `rewake interrupt` is `failed` with a hint to ask again.

Why the item ties the mark, and what keeps an ordinary turn from it — attribution only,
since what is published does not read the mark. An ordinary turn
compacts inside itself with the same item: before its input is recorded when the
context is already full (`core/src/session/turn.rs`, `run_pre_sampling_compact`, so
the item may be the turn's first), or midway (`run_auto_compact`). What tells the two
apart, from the source:

- a manual compaction's turn sends no item but its `contextCompaction`: the local, the
  remote and the token-budget paths each emit that one item and nothing else
  (`core/src/compact.rs`, `compact_remote_v2.rs`, `compact_token_budget.rs`), and the
  compaction hooks come as `hook/started` and `hook/completed`, not items
  (`hook_runtime.rs`, `bespoke_event_handling.rs`). A live run showed the same: the
  item first and alone in its turn
  ([research-protocol.md](research-protocol.md#compaction-and-interrupt-on-request)).
  A turn with any other item is work;
- no successful reply names a manual compaction's turn: `thread/compact/start` answers
  `{}` (`thread_processor.rs`, `thread_compact_start_inner`), and input into a running
  compaction is refused (`turn_input.rs`). A turn a reply named is work.

Either proof may come after the item — a reply travels apart from the events, and an
auto-compaction at a turn's start comes before the turn's input — so a mark tied to a
turn later shown to be work is untied, and waits for its own compaction's item again.
That is not a move: the tie was wrong, and a manual compaction's turn never gives
either proof. The mark goes when its turn completes, which is main's answer, or when
the server refuses the request; otherwise with the connection. A reply may even come
after the turn's end, the mark having ended with it; the turn still reports, by the
publication rule.

Sources: reference tree `e29eceb75`, read September 24, 2026, by write-claude, and by
review-codex for main's question on what proves an operation's end.

What the live run showed — the order of events, a compaction of an empty conversation,
what the terminal shows and does meanwhile — is in
[research-protocol.md](research-protocol.md#compaction-and-interrupt-on-request).

## Known limits

- **A goal's turn that compacts first, in a continuous stream, before main's compaction
  starts.** An active goal starts a turn on an idle conversation by itself (`ext/goal`,
  `runtime.rs`), and a turn whose context is full compacts before its input
  (`core/src/session/turn.rs`, `run_pre_sampling_compact`; `compact.rs`), with the same
  item main's compaction sends; its hooks come as `hook/*` (`hook_runtime.rs`), not
  items. If that item comes before the item of main's compaction, the mark ties to the
  goal's turn, and if the turn then ends with no other item — a pre-turn compaction that
  fails, say — main is told `done` or `failed` by that turn, the conversation's
  uncertainty about main's compaction clears early, and the telemetry puts main's name
  on the goal's compaction. And main's next compaction may be sent while main's first is
  still queued or running, and replace it: property (1) holds here for work only, since
  what the second compaction would abort is a compaction. Neither compaction's turn is
  published, nor the goal's: each has no proof of work, and its item shows it to be a
  compaction. Until September 25, 2026 the mark was what kept a turn from publication,
  and main's first compaction, left with no mark, was published as a finished turn. A
  turn that goes on to do anything shows another item, is untied and reports.
- **A turn whose reply comes after its end.** A turn of the terminal's or a delivery's
  that compacts first and ends — before the reply naming it reaches the gateway — while
  a mark waits for its compaction's item is taken for that compaction: main may get a
  false answer, and the telemetry puts main's name on that turn's compaction. The turn's
  report is still published when the reply comes, and the reply closes every operation
  sent before it anyway.
- **A compaction request left without a reply** keeps its mark in case the server
  starts it late; it takes a server silent for the whole 80-second wait. The hold ends
  with main's answer: deliveries go, and so does the terminal's `/compact`. A turn the
  terminal starts meanwhile is work, as it should be: only the compaction's own item
  ties the mark.
- **A compaction lost sight of.** After the terminal left the conversation before the
  compaction's turn was seen, the compaction ends unseen or is any later turn, so main's
  compaction on that conversation stays refused as uncertain until a `turn/start` or
  `turn/steer` sent after it is answered — in practice, the next turn. A delivery sent
  while the compaction still runs is refused by the server and fails, as it does past
  the bound.
- **A turn without proof of work is only advisory.** A goal's turn that fails before any
  item, or a turn whose reply the gateway never read, the connection having closed
  between the request and its reply, reports as advisory: the task it did stays owed
  until a later turn finishes. So does an outcome whose proof comes after 16 later
  outcomes on the same connection waited for theirs. A goal's turn that fails in its
  pre-turn compaction reports nothing: its only item is `contextCompaction`, which takes
  it for a compaction. A compaction stopped before its item reports as advisory, noise
  for the waiters, since nothing shows what it was. The proofs of the last 1024 turns
  are kept. A gap is reported as advisory too.
- **A mark whose compaction ends unseen after its turn was seen** — the terminal left the
  conversation by `/resume` of another while it ran — lives until the connection ends:
  the server sends this connection nothing more of it, and neither a resume nor a status
  is its end. It holds deliveries only up to its bound; main's compaction on that
  conversation stays refused as uncertain until a later turn is answered.
- **Work accepted and lost.** A turn or review the server accepted and whose end the
  gateway never reads — on this connection or on any after it — keeps the conversation
  uncertain for main's compaction until the next turn is answered, though the terminal
  finds it idle. So does a request whose answer was lost with its connection, whether
  or not the server took it. A false refusal, by the rule above; the refusal says to
  compact from the TUI, or after the next message delivered or turn typed at the
  terminal.
- **A compaction whose item and end come on the next connection** keeps its operation
  open there. The operation is named by the mark once the item ties it, and the mark
  ends with the connection it was set on, so on the next one the compaction's item names
  nothing and its end closes nothing: main's compaction stays refused as uncertain until
  a later turn is answered. A false refusal, safe for (1). Tying an operation with no
  turn to the first `contextCompaction` item seen would end it, and was decided against
  on September 25, 2026: that item may be a goal's turn's own compaction, the race
  above, and the operation would then close while main's compaction is still queued.
- **A running turn whose id is unknown.** After a resume without turns the reply says a
  turn runs but not which; `rewake interrupt` is then `failed` with a hint to ask
  again, until an event of that turn names it — usually within moments, since a
  running turn streams.
