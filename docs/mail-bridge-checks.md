# Building and checking the server of the mail tool

The third part of the stage 2 design, beside [mail-bridge-server.md](archive-1.x/mail-bridge-server.md)
(the rules and one call) and [mail-bridge-turns.md](mail-bridge-turns.md) (after the
answer): what the build changes in stage 1, how it is tested without a live harness,
what stage 3 must still show live, and what the reviews of the rules found and what
meets each finding now. **Built and accepted October 1, 2026**, with the tests below
([how the checks are built](#how-the-checks-are-built)); the live checks come with stage 3, whose
code is built and accepted.

## What stage 2 changes in stage 1

- **The call's binding.** `lockOperation` writes `receipts/<epoch>/calls/<call>` right
  after it holds the record, under the tool only; the sweep removes a binding with its
  record. `receipt` gains the lookup with three outcomes.
- **Later turns.** `journaled` runs `unresolvedBefore` for a tool call too, once
  `receipt.Begin` finds no record under the call's own key; today it skips the check
  whenever a scope is set (`internal/cli/journal.go`). The same words then stop at an
  unfinished or unknown operation of an earlier turn, as the shell's do.
- **The pending mark.** Only an attempt in the operation's own turn marks: the first, or
  a tool call whose ticket names the turn in the key. Under the mailbox lock, before the
  mark, the outcomes of
  [the pending table](mail-bridge-turns.md#a-pending-mark-at-its-turns-end); a replay to
  a call not shown to be in that turn says this turn is not marked, with exit 1. The
  shell keeps the same rule, so a shell `retry` of an unmarked step no longer marks.
- **The read's answer.** `receipt.Shown` gains `Answer`, the digest of the whole text
  the call prints, and the read's stdout is exactly that text.
- **The acknowledgment.** `AcknowledgeRead` takes one budget for both locks, used only
  before its check; checks the evidence's result against `Answer`; and, under the
  mailbox lock, refuses once an end at or after the call's `CalledBoot` is on record or
  noted, before it changes anything, the receipt included. Past its check it registers,
  and before it releases the mailbox lock it closes: a snapshot of the read clock into
  its own slot, on a failed write too. `bridge.Exposure` gains the result.
- **The end's capture.** The Codex gateway and the collector's stop after an Esc note
  the end under the observer's mutex; with an acknowledgment registered, the boundary is
  that one's closing snapshot, and without one the clock sampled inside the mutex.
  The pending mark reads no note: the journal is its cut.
- **The ticket.** `bridge.Ticket` gains its nonce.
- **Writes through one place.** Receipts, bindings and their removals go through the
  state package's write functions, which get one test hook (below); a test fails when
  `internal/receipt` writes or removes a file by any other way.

## Testing without a live harness

- **A fake MCP client over stdio** drove the real `rewake bridge-serve`, built once per
  test binary, until the server left in S8; its tests are kept in
  [archive/1.x/codex](../archive/1.x/codex/README.md). It works against a real endpoint in a temporary state directory, with
  observations fed by the test through each adapter's own parser.
- **Recorded events from probe 2**, cut to the fields used, are the adapters' parser
  fixtures: Codex `item/started`, `item/completed` and `turn/completed`, and Claude Code
  PreToolUse, PostToolUse, PostToolUseFailure and Stop. The acknowledgment test takes
  its results from them, so the bytes the harness recorded are the bytes compared.
- **The fault test, over every path that holds a record.** The paths are listed once,
  as scenarios: a new heads-up, a new pending mark, a new read; the same words joining
  each; `retry` of a heads-up, a pending mark and a read; `inbox --next` of a read; the
  same words from a later turn. A clean run of each records the order of its durable
  writes through the state package's hook, and the steps are learned from that log, not
  from a list: a write added to a path is in the next run. Then, per step:
  - the child crashes there, from a test-only hook in the write path;
  - the write fails instead (an I/O error);
  - the lookups the server and the observer make afterwards — the binding, the record,
    the journals — fail in turn, as unknown, the way stage 1's plan test fails reads.

  Each run asserts the call's answer is its row of the failure table — never "nothing
  ran" once a step after the binding was taken — and that the same words, then
  `rewake retry`, reach exactly one effect: one heads-up, one pending mark or none with
  an honest answer, and a letter read once, only on its call's answer. In every clean
  log, the binding comes before every write that is an effect; only the child's step
  after the wrapper confirmed its ticket, `receipt.Begin`'s record and key index, and
  that record dropped when another call's index landed first may precede it. The same scenarios kill the server instead of the
  child at each step of a call's path (2 to 8).
- **The order test.** Interleavings generated from the observer's one list of event
  kinds, over two calls in two turns: observation, ticket request, confirmation, the
  child's exit, completion, a read and a pending mark from the shell in the next turn
  (each a commit under the mailbox lock), the end, the end's journal; with Stop held by
  stage 1, Stop blocked by another hook, the plugin's end and the hook's for one turn, a
  lost and a repeated completion, a server restart asking for the same ticket again, and
  each wait running out of its bound. It asserts:
  - after the end is noted, no acknowledgment that has not passed its check may begin
    writing; the acknowledgment already registered is included through its closing
    snapshot, and no read by a subsequent mailbox holder, from the tool or the shell,
    can extend that boundary;
  - no mark after the end's journal;
  - one ticket per native call;
  - no wait outlives its bound, and none holds what its event needs;
  - a pending mark's answer says what its turn's end saw.

  Two cases are written out, each with the tool and the shell. **An Esc nobody heard**:
  a pending step of T saved, its child dead before the mark, T ended by an Esc with no
  plugin; T+1's UserPromptSubmit held back, then a `retry` of the step, then the start
  written and T+1's end; and again with the start's write failing. The `retry` must
  answer not marked, and T+1's report must not be interim by that step. **An
  acknowledgment cut midway**: stopped after both locks and its check, and between two
  `MarkRead` of one answer; its budget then spent, the end captured, the write released.
  The boundary must follow the last commit, the parts and waits must agree with it, a
  shell read of the next turn tried meanwhile must commit above it, and a second
  acknowledgment arriving after the note must write nothing. **A capture paused after it
  chose the writing acknowledgment**: that acknowledgment closes and releases the mailbox,
  a shell read commits, then the capture resumes; the boundary must hold the
  acknowledgment and not the read, and again with the acknowledgment's write failing
  midway. **A pending mark between capture and journal**, from a child and from the
  shell as separate processes: the journal's write held after the capture, the mark
  taking the mailbox lock before it and after it; the end must take the first and the
  second must answer not marked.

  Stage 1's window before the capture — a turn the server starts on its own before the
  gateway reads the last completion — is a scenario of its own, recorded as open and
  not counted against these assertions.

  An event kind added to the list is in the next run. A child past its deadline is held
  by the fault build after its confirmation; one whose kill does not end it is played by
  a process of its own session that keeps the child's stdout open, which is what the
  server sees of a child that did not die.
- **The evidence test**: a short body equal to a line the server writes; two letters
  with one body; a replacement that kept the body; `--json` with escapes; a result kept
  as a preview; a full completion where the model was shown a shortened result; a
  result marked `isError` by the server. None acknowledges a part.
- **The bound test** fuzzes the whole envelope through the encoder — ids at and past
  their bound, frames at and past theirs, stdout and stderr in ASCII, Cyrillic, emoji,
  escapes and dense tokens. Every message written must fit, and a test fails when any
  code writes to stdout except through the encoder.

## How the checks are built

The server's tests are in `internal/bridge/server`, the endpoint's in
`internal/bridge/endpoint`, and the adapters' beside each adapter. The test binary builds
rewake with the `rewakefault` tag, whose `REWAKE_FAULT` (`internal/state/fault_build.go`)
logs, ends, fails or holds a server, child or shell process at a durable step; the test's
own process plays the wrapper, and its writes go through the same seam in-process
(`wrapper_fault_test.go`).

- **The fault test** (`fault_test.go`, its scenarios in `fault_scenarios_test.go` and
  `fault_variants_test.go`): a read, the next part of a read, a heads-up and a pending
  mark, and for all but the next part the same words again, their `retry` and the same
  words a turn later. Each clean log gives the cases: the child ended before each of its
  durable steps, with and without the binding readable to the server; the child's write
  failed at each; the server ended before each of its steps; each directory the child or
  the acknowledgment reads made unreadable; and the acknowledgment's write failed at each.
  Each case asserts its answer is one the failure table allows — "nothing ran" only with
  no binding and no effect, a named `retry` only of the call's own binding — that a
  letter is read only on an answer that arrived whole, and that clean calls of the same
  turn then reach exactly one effect.
- **The order test**, generated in three spaces, each from one list of events:
  - `order_call_test.go` takes one call of T apart — its observation, the server's
    request, the child's confirmation of the ticket, the child's exit, the result
    recorded and recorded again — against T's end taken apart into its capture,
    `turn/completed` reaching the table, and the journal. The server is held before it
    starts the child and the child after its confirmation, so each step lands where the
    order puts it; the first acknowledgment lands, runs out of its budget on a held
    receipt lock, or fails its first write, and the result may be lost. It checks that a
    ticket is issued exactly when the call was heard and asked for before the capture,
    that no acknowledgment writes after the note, that a repeated result changes
    nothing, the boundary, and what T's end reported.
  - `order_gen_test.go` generates every order over two turns — T's call, its completion
    (once, lost, or repeated), T's end and its journal, a shell read and a tool read in
    T+1, a restarted server asking for T's call — and checks the boundary, who read the
    letter and what T's end reported.
  - `endpoint/order_table_test.go` orders one call's observation, request and
    confirmation against the table filling with other calls, the call aging past its
    life, the harness reporting it again and a second server asking for it: the call
    gets at most one ticket.
  `order_test.go` and `order_cut_test.go` write out the capture during a writing
  acknowledgment, one call across two servers, an acknowledgment cut midway (its writes
  landing or failing) and an Esc nobody heard; `order_marks_test.go` holds a child and a
  shell pending mark before and inside the mailbox lock while the end is captured and
  journaled. `endpoint/gate_test.go` proves the capture never samples again after it
  waited; `endpoint/tickets_end_test.go` covers a capture on Codex before the turn's
  completion and an interruption on Claude Code against tickets, a capture still waiting
  for a writer, and the run's limit on the calls it remembers, whose last ticket two
  calls race for and which stays spent past the table. `child_bound_test.go` holds a child past its deadline: killed
  with its group at the hard bound, and, when something of it holds its output past the
  kill, keeping its slot while the server answers another call; each with stdin open,
  ending before the kill and ending after it, where the server answers the outliving
  call as unknown a second past the kill or the EOF and exits without it; a call that
  bound its operation first names that operation's `retry`, which reads the letter.
  `stdout_bound_test.go` gives the server a one-page stdout the client does not drain:
  ping replies fill it, with stdin ending or left open, and a call's answer waits on it,
  with stdin ending before the write, during it, or left open; the server ends within
  its 2 s, the answer that never left reads nothing and the letter goes to a later
  call, and with stdin open and nothing backed up the answer lands whole once read; a
  reader that resumes within the 2 s gets every reply in order; and a send whose answer
  never left is found again by the same words and its `retry`, its heads-up still one.
- **The evidence test** (`evidence_test.go`, where a completion whose binding cannot be read is spent all the same), **the bound test** (`bound_test.go`, two
  fuzz targets whose seeds run with the checks), the protocol and its limits
  (`mcp_test.go`, `encoder_test.go`), the flows (`flow_test.go`), and the endpoint's
  tickets, hellos and adapters against the recorded fixtures
  (`endpoint/*_test.go`, `gateway/bridge_test.go`, `telemetry/plugin_test.go`).

Not generated, and why: Stop held, Stop blocked by another hook, and the plugin's end
beside the hook's are stage 1's ends, which stage 2 reads only through the journals the
generated orders write; a capture held between choosing an acknowledgment and taking its
snapshot has no seam that pauses it inside the wrapper, and is proved on the gate instead
(`gate_test.go`, which takes no lock and has no budget: the acknowledgment's budget is
the generated call orders' and the cut acknowledgment's, held past it); the Esc case with the start's
write failing needs Claude Code's prompt hook, and waits for stage 3.

## Left to stage 3, and live

- **What stage 3 injects:** the server entry and its `env`; Codex's `tool_timeout_sec`,
  `default_tools_approval_mode` and `omit_tools_from`; Claude Code's `--mcp-config`
  layer and `--allowedTools`; the synchronous PreToolUse and PostToolUse hooks for
  `mcp__rewake__rewake`, beside the person's; the refusal of an occupied name.
- **Live acceptance**, on both harnesses: the order of observation and call over many
  calls; the order of completion and end; a nested agent's call, and which hook field
  names it on Claude Code — tool reads open there only after this; the recorded result's
  bytes against the server's (a trailing newline, a re-encoding); `/clear` and `/resume`
  in an interactive Claude Code; Codex parallel tool calls; a pending mark after a
  timeout and after an Esc; that a `prompt_id` is never reused for a later prompt, Esc
  included, before a later Claude Code call may mark; how long a capture waits for a
  writing acknowledgment;
  server death under `--remote`; the notices of a failing channel.
- **Lower configured output limits.** Codex's limits are read from its effective
  configuration at launch and at each admitted thread; Claude Code's
  `MAX_MCP_OUTPUT_TOKENS` and `MCP_TOOL_TIMEOUT` are taken per call by the hook, and
  acknowledge a read only once the gate G8 shows them to be the values the harness
  applies ([mail-bridge-launch.md](archive-1.x/mail-bridge-launch.md#claude-code)). A limit not proven safe
  leaves that read unacknowledged; the cap is never raised to fit.

## What the first review found

The first review of the rules (October 1, 2026) accepted the transport without mail
state, authorization only after a native observation, the bounded wait for it, the
one-time ticket and single build, the result cap without clipping, and the stage
boundary; and found seven defects. What meets each now:

1. **The absence of an index under the call's own key proved nothing for `retry` and
   `--next`**, which never open a record under that key. The binding is written by every
   path that holds a record, before its first step (rule 4), and the fault test covers
   each path.
2. **A boundary sampled after the end's waits could take in a read of the next turn.**
   The end waits for no call that has not begun to write; its boundary is taken at its
   event, and a commit that meets the end commits nothing (rules 7 and 8).
3. **A Stop is not always the end of its turn.** A Stop that stage 1 holds writes no
   journal and the turn goes on under the same identity; one another hook blocks after
   ours published goes on under a new one. The correlation needs no UserPromptSubmit,
   and reads from a possibly nested agent are refused until stage 3 shows how one is
   named.
4. **A process gone does not settle its operation.** Nothing waits for a process any
   more. A pending mark is decided under the lock against the journals: marked, or
   proven not marked once its turn's end is on record.
5. **The end's waits had no total bound.** Every wait has a bound and an outcome in the
   table of waits, but one: a capture waits for an acknowledgment already writing, as
   stage 1 leaves every write under a held lock untimed.
6. **Containing a letter's bytes did not prove that letter's part was shown.** The
   evidence is the whole answer the call recorded before printing it, by digest, and
   every answer the server substitutes is an error.
7. **An id longer than the bound could not be answered within it.** Frames and ids are
   bounded before anything runs, and the id's room is inside the framing.

The corrections to the failure table are in place: an unobserved completion proves only
that parts were recorded as about to be shown; a result over our bound proves no
exposure, not that the bytes did not arrive; the wrapper's death leaves an authorized
child running to its deadline; the binding comes before the parts recorded as shown, so
neither a missing binding nor a missing entry gives a false acknowledgment or blocks a
later showing. The spec's lines on delayed finalizers, on a fresh identical operation in
a later turn and on cancellation were brought in line with these rules the same day.

## What the second review found

The second review (October 1, 2026) closed five of the seven and accepted the
direction, and found two gaps:

1. **A recorded turn start was taken as written before the next turn's calls.** It is
   written in the background, so a `retry` in the next turn after an unheard Esc could
   mark an old step and answer "marked". Now only an attempt in the operation's own turn
   marks; a recorded start counts only as proof the turn ended, and its absence as
   nothing.
2. **One budget was promised as both the lock wait and the whole acknowledgment.** A
   write already begun cannot be bounded, and letting the stream go while it ran let it
   commit above the boundary. Now the budget bounds only the waits before the check,
   and the end is noted before its boundary is taken, which cuts every acknowledgment
   not yet past its check. The Codex stream is no longer held for an acknowledgment's
   waits. How the one already writing is taken in is the third review's first finding.

## What the third review found

The third review (October 1, 2026) accepted the unbounded wait for a write already
begun, and found two transitions not yet finished:

1. **A boundary as the later of a sample and the writing acknowledgment's last
   position** could take in the next holder's read, once that acknowledgment released
   the mailbox before the sample. Now the writing acknowledgment closes with its own
   snapshot under the mailbox lock and the capture takes exactly that; with none
   writing, the clock is sampled inside the mutex no acknowledgment can register past.
2. **A pending mark could not read a note held in the wrapper's memory.** It no longer
   needs one: it takes no read position, and its cut is the end's journal, under the
   mailbox lock the end's choice of mark shares. A mark after the capture and before
   the journal belongs to that end, which takes it.

It also asked that two stage 1 limits be written where the promise is made, not only in
a report: they are in
[mail-bridge-turns.md](mail-bridge-turns.md#a-turns-end-meets-its-calls), and the
spec's promise is narrowed to the capture's moment.
