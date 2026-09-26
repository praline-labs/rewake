# Remote control: the tests

What proves `rewake compact` and `rewake interrupt`, part by part: the design is in
[remote-control.md](remote-control.md), the Codex side in
[remote-control-codex.md](remote-control-codex.md), and how the workflow cases are
built in [testing-plugin.md](testing-plugin.md#steering-a-session).

- `internal/control/control_test.go` — the protocol: a done answer and the cleanup, the
  modes, `not answering` with the request removed, a taken request without an outcome
  answered as an open failure and its late result cleared, a partial or foreign answer ignored, a second asker
  refused without writing, no directory, a request taken as the asker gives up, in
  each of the three orders above, a taken request left in place while it is served, and
  an asker cut short withdrawing its request as `cut short`.
- `internal/wrap/control_test.go` — the wrapper makes the directory, private, for a
  harness that serves it, removes it with the session, and makes none for one that
  does not.
- `internal/cli/steer_test.go` — the commands against a fake served side: the outputs,
  a compaction answered started and one answered requested, each ending the command
  and leaving main's record of it, made before the request is written, a request taken
  and unanswered keeping it and promising the letter, a command that could not leave the
  record promising no letter, the refusals and a final `failed`, which leave none, while
  an open `failed` keeps it, the record held while the command asks and free once it
  has ended, with their next action, `not answering`, and every wrong call refused
  with exit 2 and nothing written; a harness that is not steerable, and one without a
  focus; and a SIGTERM during the pickup, which ends the call as `cut short` with the
  request withdrawn instead of ending the process.
- `internal/harness/claude/plugin_control_test.go` and `plugin_compact_test.go`, with the host in `plugin_control_host_test.go` — the module under node against a
  strict host with the research's forms and refusals of `$.clock.every`,
  `$.clock.sleep`, `$.fs`,
  `$.session.compact` and `$.turn.abort`: an idle compaction answered started and its
  end told with the counts, an answer written before the end of a compaction that takes
  a while — on the collector's start mark — with a second request refused meanwhile, a
  start not seen within the bound answered requested, a failure after the answer told
  as failed, a host call failed before its start was seen answered as a final failure,
  the host's refusals passed
  on, "in flight", "an external turn" and the thin client's among them, and an error
  after the start counted as started, the asker reported and sent before the host compacts, a
  refusal reported and sent before the answer with whether the host had started, no
  mark sent mid-turn, a second waited out after the end of a compaction the module did
  not ask for, the host itself held to the re-entry rule, the interrupter reported, a request carried out once across a reload, a request
  withdrawn or replaced as it is marked left undone, an answer the host failed to write
  still ending the compaction and freeing the next request, a final answer — a refusal
  mid-turn, a failed host call — told as the outcome after it is written, and no polling without a
  directory.
- `internal/harness/claude/telemetry/interrupter_test.go` and
  `internal/harness/claude/lane_interrupt_test.go` — the stopped text naming main, the
  mark and when it is used up or laid aside, and the notice line;
  `internal/harness/claude/telemetry/compaction_asked_test.go` — the mark decoded, taken
  up by the next compaction and laid aside by its refusal, which ends `compacting` for
  a started compaction, also when its `PreCompact` comes after the refusal, and leaves
  another compaction's alone; `compaction_ended_test.go` — `compact.ended` decoded, its
  detail cut, an outcome a compaction cannot have refused, the outcome kept in the
  snapshot, and the collector's start mark written for the `PreCompact` of the
  compaction a main asked for and for no other.
- `internal/wrap/session_notices_test.go` — no notice of a compaction this main asked
  for, and one each for a compaction another main asked for and one nobody asked for;
  `compaction_letters_test.go` — one letter, a note from the worker owing nothing, with
  the tokens and the count; a refusal and a failure as letters at once, another main's
  outcome not sent; a half waiting for the other only a moment, and going at once when
  the worker leaves; every letter owed by main's record of the request, which goes with
  it: a worker gone with no word of the compaction as a failed letter only after a
  moment in which its last snapshot is read again, and one that published its outcome
  late getting that outcome's letter, no word within the bound as the bound's letter, and a record an
  earlier run of main left closed by the next; `compaction_letters_hold_test.go` — a record
  its command holds left alone while its worker leaves, and a command killed with
  SIGKILL letting go of its record, which is then closed. `internal/state/held_test.go`
  — the hold taken only once the writer lets go, and refused on a file removed or
  replaced between the open and the lock; `internal/control/pending_test.go` — the
  record read again under the hold, and a closed one not held.
- The workflow case `claude-steered` — both commands end to end from a main against
  workers whose module runs under node in the fixture: an idle compaction that takes
  five seconds answered started within three, before the telemetry counts it, then its
  letter to main with the tokens and the count, the count in the telemetry and no notice
  of it to main, a second compaction whose worker leaves before its end still ending
  in a failed letter to main, a compaction
  refused mid-turn with the turn going on, an interrupt giving main the stopped text,
  also in its awaited list, and the worker the notice line once, an idle interrupt
  refused, a worker without the module not answering, and a worker's call refused as a
  wrong call; fourteen product mutants, one per link
  ([testing-plugin.md](testing-plugin.md#steering-a-session)).
- `internal/control/serve_test.go` — the served side the Codex wrapper runs: an answer
  written once with the request's id, a request carried out once across a second look
  and a fresh server, one withdrawn or replaced as it is marked left undone and handed
  to the served side once its answer is written, and a foreign id not taken.
- `internal/harness/codex/gateway/compact_start_test.go` — a compaction answered
  started when its item ties the mark, before its turn ends, its end with the tokens
  waited for apart and kept in the snapshot; one whose start is not seen answered
  requested, saying whether the server had taken the request, and so is one the gateway
  lost sight of before its item tied the mark; a server's refusal
  answered at once with nothing left to wait for, a final failure recorded as the
  outcome, and a connection
  ending after the request went out answered as an open one.
- `internal/harness/codex/gateway/steer_test.go` and `steer_guard_test.go` — a
  compaction on request marked manual, never published as work, with its tokens and
  counted as the asker's; refused without a word to the server while a turn, a
  `turn/start` or `review/start` in flight, the terminal's `/compact` or a working
  status runs, and between a `turn/start` reply and its `turn/started`, the terminal's
  and a delivery's; refused as `nothing to compact` on a new conversation with no turn,
  and sent on a resumed one; a server's refusal leaving no mark, an unanswered request
  keeping it for a late compaction and answered as an open failure, and a mark whose compaction never starts ceasing
  to hold at its bound while main's next compaction stays refused; a turn acknowledged
  before a `/resume` to another thread keeping the conversation uncertain until a later
  turn is answered; the terminal's `/compact` answered by the gateway while main's
  runs; a delivery waiting out main's compaction and going after it; an interrupt
  naming main in the stopped outcome, of a turn the terminal started and of one a
  delivery started, and refused idle, during a compaction and when the server says no
  turn is active. `internal/harness/codex/server_steer_test.go` — the wrapper serves
  the run's directory, answers what it cannot carry out, records a compaction
  withdrawn before it was taken as its outcome, and passes a reservation
  refused for a compaction to the inbox as one to try again;
  `internal/inbox/reservation_test.go` keeps such a message pending and delivers it
  on the next pass. `activity_test.go` holds the record of what a conversation does
  to the frames that set and clear it: a compaction refused while an inline review is
  acknowledged and sent past a detached one; an interrupt naming the turn a
  `turn/start` reply named before its `turn/started`, and the one a resume's snapshot
  shows running; a delivery that queued on the gate behind main's compaction staying
  pending; a compaction left behind by a `/resume` not ended by an idle resume — still
  holding when its start was seen, lost sight of when only answered — and held until its
  end when it comes back running; a reply naming a turn already ended holding nothing.
  `accepted_test.go` holds main's rule for work the server accepted: an operation
  without its end — a review, started or not, a turn through `systemError` or through
  its items — leaving the conversation uncertain through a resume, a status and time;
  the answer to a later turn ending that; a compaction answered but not started
  outliving an idle resume; a mark holding deliveries only up to its bound, with main's
  answer saying why; an unbound mark taking no other turn; an interrupt learning the
  turn's id from an item event; a late reply from an earlier selection naming no turn. `mark_test.go` holds how the mark
  finds its turn: an ordinary turn compacting inside itself never taken for it, also
  when the proof it is work comes after its compaction item; a tied mark never moving;
  an operation past the record's capacity leaving the conversation uncertain; main's
  wait ending the hold and its answer saying whether the turn was seen. `sight_test.go`
  holds the three safety properties against attribution: a mark lost sight of when the
  terminal leaves before its turn — main answered at once, deliveries going, the
  conversation uncertain, also through a goal's turn failing at its compaction; a lost
  compaction's turn settling nothing, whether its item, only its end or a run of unknown
  id comes back, or the terminal's own compaction came between, while a goal's turn
  shown to be work by its item and a turn of the terminal's still report; an untied mark leaving no author on
  the turn's compaction; main's wait without a reply ending the hold; a compaction that
  ended as the wait did answered by its end; and a turn whose end came before its reply
  still reporting, the terminal's and a delivery's. `proof_test.go` holds the
  publication rule: a compaction only advisory when stopped before its item by Esc or
  its hook, and never published when it runs after a goal's turn took its mark or ends
  on the next connection; a turn of the terminal's and a delivery's reporting in every
  order of the reply and the turn's events, with no item or with the reply last; and a
  turn named on one connection reporting when it ends on the next. `advisory_test.go`
  holds the advisory report of a turn without proof — a goal's failing before its item,
  one whose reply was lost with the connection — none for a compaction shown by its
  item, the turn's own outcome after a late proof, a stop included, under an identity
  apart from the advisory's, and no outcome dropped when the connection ends or later
  ones push it out. `reconnect_uncertainty_test.go` holds an inline review, or a turn
  whose reply was lost, keeping main's compaction refused after a reconnect until its
  end, a later answered turn or a delivered message, and the refusal texts naming
  their ways out. `internal/cli/gap_advisory_test.go` holds the advisory reports at the
  waiter, and a proven stop after an advisory reported with its times, taking the
  pending mark.
- The workflow case `codex-steered` — the same commands on the Codex column, the shim
  answering `thread/compact/start` and `turn/interrupt` as the server does, aborting a
  held turn for a compaction sent mid-turn: a focus refused with exit 2 and nothing
  compacted, an idle compaction answered started before its end, then its letter with
  its tokens and count and no notice to main, a
  compaction refused mid-turn with the turn going on, an interrupt giving main the
  stopped text, also in its awaited list, and the worker's next notice no line, an idle
  interrupt refused, and a worker's call refused; seven product mutants
  ([testing-plugin.md](testing-plugin.md#steering-a-session)). The shape case checks the
  shim's reply and events for both requests against the schema.
- The workflow case `codex-compact-hold` — a task sent right after main's compaction of
  a Codex worker whose compaction runs long, the shim refusing input meanwhile as the
  server does: past the mark's start bound the hold lasts to the compaction's end and
  the server is never asked; past its running bound the task is refused, waits and goes
  after the end, and main's letter comes from the late end with its tokens; four product
  mutants ([testing-plugin.md](testing-plugin.md#a-long-compaction-on-codex)).
  `internal/harness/codex/gateway/compact_hold_test.go` holds the same at the gateway,
  and `internal/wrap/compaction_letters_running_test.go` the letter waiting past an
  outcome of `started`.
