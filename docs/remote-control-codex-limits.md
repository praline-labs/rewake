# Remote control on Codex: known limits

Split out of [remote-control-codex.md](remote-control-codex.md) on September 26, 2026,
when that file passed the project's 400-line limit. That document says how a Codex
session serves main's compaction and interrupt; this one lists where the gateway's view
falls short of the server's, what a false answer costs, and why each is accepted. The
properties numbered below are the three safety properties stated there; the code the
limits refer to is in `internal/harness/codex/gateway`.

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
  false answer or letter, and the telemetry puts main's name on that turn's compaction. The turn's
  report is still published when the reply comes, and the reply closes every operation
  sent before it anyway.
- **A compaction request left without a reply** keeps its mark in case the server
  starts it late; it takes a server silent for the whole 80-second wait. The hold ends
  with main's letter: deliveries go, and so does the terminal's `/compact`. A turn the
  terminal starts meanwhile is work, as it should be: only the compaction's own item
  ties the mark.
- **A compaction lost sight of.** After the terminal left the conversation before the
  compaction's turn was seen, the compaction ends unseen or is any later turn, so main's
  compaction on that conversation stays refused as uncertain until a `turn/start` or
  `turn/steer` sent after it is answered — in practice, the next turn. A delivery sent
  while the compaction still runs is refused by the server and stays `pending`, tried
  again until the compaction has ended, as it is past the bound.
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
  is its end. It holds deliveries only up to its running bound; main's compaction on that
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
