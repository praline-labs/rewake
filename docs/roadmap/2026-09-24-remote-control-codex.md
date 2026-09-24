# A main that compacts and interrupts a Codex session — September 24, 2026

Stage 2 of rewake's function-hooks plugin, part B: `rewake compact` and
`rewake interrupt` for a Codex worker, with the same observable behaviour as part A on
Claude Code ([2026-09-24-remote-control.md](2026-09-24-remote-control.md)). Codex has no
plugin, so the worker's wrapper serves the control directory itself. The design as
built is in [remote-control-codex.md](../remote-control-codex.md).

**The owner's decisions**, recorded where they apply:

- A focus for a Codex compaction is refused with exit 2 before anything is sent; a
  proper route and an emulation are to be researched later, not built (September 24;
  [work-queue.md](../work-queue.md)).
- On Codex no line about an interrupt is added to the worker's next notice: Codex puts
  `<turn_aborted>` into the model's history itself, and the line on Claude Code exists
  to match it (September 24).
- A conversation that has run no turn is refused as `nothing to compact` before
  anything is sent, as Claude Code answers (September 24, main's decision after the
  live run).
- The terminal's `/compact` while main's compaction runs is refused by the gateway with
  an error the terminal shows (September 24, main's decision after the review).

**What was built:**

- `internal/control` gained the served side, `Serve`: the same steps as the module —
  mark taken, check the request is still in place, act, write the answer once, by
  rename.
- The Codex wrapper runs it on the run's control directory once the app-server is up,
  and the Codex harness implements `harness.Steerable` with `CompactFocus` false.
- The gateway compacts on request: under its admission gate, refused as `in a turn`
  while a turn, a request of the terminal's that starts one, a delivery being admitted
  or another compaction runs; otherwise marked manual as the terminal's `/compact` is,
  carrying the request and the asker, and sent as `thread/compact/start`. The answer
  comes when the compaction's turn completes, with the tokens before and after when
  usage was seen. The telemetry puts the request and the asker on the compaction it
  counts, so main's wrapper sends no notice of it and the command finds its count.
- The gateway interrupts on request with `turn/interrupt` and the running turn's id,
  refuses an idle one or a compaction's turn as `no turn running`, and maps the
  server's own refusals to the same. The stopped outcome of that turn says
  "`<main>` interrupted this turn with rewake interrupt".
- The command's answer to an interrupt says what the harness shows the model
  (`InterruptTrace`): the next notice on Claude Code, the history on Codex. The help no
  longer says Codex does not take the commands.
- Tests: the served side in every order, the gateway's compaction and interrupt with
  their refusals, the wrapper serving its directory; the workflow case `codex-steered`
  with five product mutants, the shim answering both requests as the server does and
  aborting a held turn for a compaction sent mid-turn; the shape case checking those
  answers against the schema ([testing-plugin.md](../testing-plugin.md#on-codex)). The
  helpers both steered cases share moved to `test/workflow/steered_helpers_test.go`.

**The live run**, on Codex CLI 0.155.1 in a private state directory, the cheapest model
at effort `low`, one short turn per check, recorded in
[research-protocol.md](../research-protocol.md#compaction-and-interrupt-on-request):

- A compaction on request runs as a turn: its reply first, then the status, the turn
  and the `contextCompaction` item, a token usage, the turn completed. No
  `thread/compacted` appeared. "compacted w1-codex: 17263 tokens before, 4939 after
  (compaction 2)"; main received no compaction notice.
- Codex compacts a conversation that has run no turn, where Claude Code refuses it as
  too short; the answer then carries no token counts. rewake now refuses it, see
  below.
- A message typed in the terminal during the compaction went out as `turn/steer`, was
  refused by the server, held by the terminal and sent as `turn/start` after the
  compaction ended. Nothing was lost.
- The terminal shows "• Context compacted · 4s" for a compaction it did not ask for,
  and for an interrupt on request the same "Conversation interrupted" line as for its
  own Esc.
- An interrupt: its reply 9–11 ms after the request, before the turn's end;
  main read `stopped` naming itself, in its awaited list as well. An idle interrupt was
  refused as no turn running, a focus with exit 2.

**Found in review** (review-claude, with probes in the gateway's tests):

- **A turn between its `turn/start` reply and its `turn/started`** was not seen as
  running: the reply clears the request, and the server sends the turn's start from a
  different code path, in either order. A compaction in that window was sent, and the
  server aborted the just-started turn for it — a delivery's task came back stopped.
  Fixed: a turn the server acknowledged counts as busy until its end is read, for the
  terminal's turns and for deliveries.
- **The terminal's `/compact` behind the gate replaced main's mark.** The server then
  aborted main's compaction for the terminal's; main's answer failed after 80 s, the
  compaction was not counted as main's, and its turn was published as finished work
  with empty text, which could settle a task. Fixed: the gateway answers that request
  with an error while main's compaction runs.
- **Minor**: a request that failed after it was written — no reply in time — laid the
  mark aside though the server might still start the compaction. Fixed: the mark stays
  unless the request was never written or the server refused it. Found while fixing: a
  mark kept for a compaction that never starts held off deliveries and every compaction
  for the rest of the connection; it is laid aside after 30 seconds without its turn.

**Done with the fixes**: the refusal of an empty conversation above, and on the Claude
Code side the module's table of the host's refusals completed — "an external turn is
driving the conversation" is `in a turn`, the thin client's refusal is the new reason
`remote conversation` — so that a refusal the table did not know is no longer counted
as a started compaction ([remote-control.md](../remote-control.md#on-claude-code)).
Codex 0.156.1 was checked against the fixture before the owner updates
([research-protocol.md](../research-protocol.md#codex-01561-against-the-fixture)).

**Found in the re-review** (review-claude; both holes of the first round closed, the
full suite green):

- **The busy window of the first fix had no end once the terminal left the thread.**
  The terminal may `/resume` another conversation mid-turn; the server then stops
  sending it the first one's events, so a turn acknowledged there was never read to
  its end, and after coming back every compaction was refused as "a turn the server
  started has not ended". Fixed with main's decision: only a turn acknowledged in the
  current selection counts.
- **A delivery during main's compaction failed** ("will not be delivered") instead of
  waiting, as the documents said it would: compacting a worker and then sending it its
  task, the owner's ordinary order, lost the task. Fixed with main's decision: the
  reservation waits for the compaction's end, and one that outlasts its three seconds
  leaves the message `pending`, tried again until the compaction has ended
  ([delivery.md](../delivery.md#servicing-process-the-wrapper)). The same holds for
  the terminal's `/compact`.
- The known limit of a kept mark said a turn of the terminal's is not reported anyway;
  it is, to those it owes a report. Rewritten with what the window really costs.
- Two refusal tests called the compaction unbounded; they are bounded at two seconds.

**Not accepted on the Codex side** (review-codex, on the 0.155.1 schema and the Codex
source), and one more from review-claude's check — four defects of one class, each a
window where one reader of the gateway's state was blind to what another knew:

- **P1: a compaction aborted an accepted review.** After the reply to an inline
  `review/start` the review runs, but only `turn/start` and `turn/steer` replies counted
  as work; before `turn/started` a compaction went through, and the core's `compact()`
  aborts every running task.
- **P2: an interrupt refused while a turn ran** — between a `turn/start` reply naming
  the turn and its `turn/started`, and for the rest of a turn after a resume of the
  running conversation, whose reply shows the turn in its snapshot and no
  `turn/started` follows.
- **P2: a delivery queued on the gate behind main's compaction failed** instead of
  staying pending: its wait ran out before it could see the mark.
- **A compaction the terminal left by `/resume` held deliveries pending without
  bound**, and refused every compaction, since its `turn/completed` never reaches the
  connection.
- nit: `rewake inbox --awaited` showed a pending letter as undelivered without its
  reason.

Main decided to stop closing these one window at a time: the gateway keeps one record
of what the selected conversation is doing — which turn runs or is acknowledged — set
by the replies and events that name a turn and by a resume's snapshot, cleared by the
turn's `turn/completed`, by a snapshot showing it idle and by a change of selection. The
compaction's refusal, the interrupt's turn id and the compaction mark read it. The
frames and the Codex source for each order it relies on are in
[remote-control-codex.md](../remote-control-codex.md#what-the-gateway-knows-a-conversation-is-doing).
Found while building it: the server may send a turn's `turn/completed` before its reply
to `turn/start`, so a reply naming a turn already ended starts nothing. Each defect has a
test that failed before the change, review-codex's and review-claude's repros included;
the awaited view prints a pending letter's reason.

**Round 2 not accepted** (review-codex, and review-claude's check): the record was the
one source by then, but five defects remained, all of one root — the server answers
`turn/start`, `turn/steer`, `review/start` and `thread/compact/start` when it has queued
the work, answers a resume before it takes the next queued submission, and sends replies
and events on independent paths. So no snapshot proves a conversation free, and no reply
is ordered against its turn's start.

- **An idle resume after a compaction's `{}` and before its `turn/started` dropped the
  mark**: the compaction's turn was then published as finished work with empty text,
  which can settle a task, and main's answer failed.
- **An idle resume right after an inline review's reply erased the review**, and a
  compaction then went through and aborted it.
- **After a resume without turns — the terminal's ordinary one with paginated
  history — an interrupt failed until the turn ended**: item events carrying the turn's
  id were ignored.
- **A mark answered but not started, then a resume away and back finding the
  conversation running without turns, was never tied to its turn and had no bound**:
  deliveries pending for good, compactions refused, main's wait to its deadline.
- **A late `review/start` reply from an earlier selection replaced the running turn**,
  and an interrupt named the wrong one.
- The document said the compaction's reply comes before its turn starts; the server
  gives no such order.

Main set the rule: for a compaction, when in doubt refuse — accepted work holds the
conversation until its own end, a resume ends nothing, only an idle status after the
turn's first event or a 30-second bound when none came; for an interrupt, only a turn
seen in the current selection, its id learned from any event. Done that way: accepted
turns are kept per conversation across selections, the mark survives an idle snapshot
until its bound, an unbound mark found running is tied to the first new turn any event
names, the running turn's id is learned from any event, and a reply is taken for the
running turn only in the selection that sent its request
([remote-control-codex.md](../remote-control-codex.md#what-the-gateway-knows-a-conversation-is-doing)).
Two tests of round 1 changed with the rule: a turn acknowledged before a `/resume` and
never seen to start now holds the conversation until the bound, and so does a compaction
only answered.

**Round 3 not accepted** (review-codex, review-claude's probes, and review-codex's answer
to main's question from the Codex source): the rule of round 2 still ended accepted work
by things that are not its end.

- **P1: a `systemError` status or snapshot ended an accepted turn.** It comes, among
  others, from a failed `memoryMode/set` while the turn goes on, and a resume keeps it; a
  compaction then aborted the turn.
- **P1: the 30-second bound ended accepted work** — a review queued, a compaction
  answered and starting late — though nothing had ended it; a compaction then aborted
  the review, and a late compaction's turn was published as work.
- **An unseen mark took another turn**: after a resume without turns, the first new turn
  any event named was taken for the compaction — the person's or a goal's — and main
  was told `done` for a compaction that did not happen.
- **A mark whose turn was seen, with another turn running later, held deliveries for
  good.**
- **A test had been weakened**: the left-behind compaction's case shortened the bound
  for every variant and tolerated a held delivery, so a mark wrongly held still passed.
- The document's proof that any first event of a turn means the conversation is noted
  running is false for an inline review, which has no `turn/started`.
- The workflow suite's mutant builds failed with "error obtaining VCS status" in a copy
  of the tree without `.git`.

Main set the rule anew
([remote-control-codex.md](../remote-control-codex.md#what-the-gateway-knows-a-conversation-is-doing)):
an accepted operation whose end was not read makes the conversation uncertain for main's
compaction, which is refused with an explanation and never sent; only that operation's
`turn/completed` with a final status, or the answer to a `turn/start` or `turn/steer`
sent after it, clears it — no status, no snapshot, no time. Deliveries are not held by
uncertainty, only by a compaction mark, and only up to its bound; past the bound the
wait ends and the mark does not. The mark is tied to its turn only by that turn's
`contextCompaction` item. Done that way: the gateway numbers every request that can start
work in the order it is written and keeps the open operations of each conversation by
that number; the bound stays 80 seconds, main's own wait, with the reasons in the
document. The weakened case shortens the bound only for the compaction only answered,
and asserts the held delivery and the refusal's text in the others. Mutant and suite
builds pass `-buildvcs=false`. Each defect has a test that failed before the change,
review-codex's repros and review-claude's probes included.

**Round 4: the rule held, three defects remained** (review-codex from the source,
review-claude with probes and a full suite, green). review-codex confirmed why a later
answer proves an operation's handler has run: input into a running compaction or review
is refused, so only a successful answer counts.

- **A lingering mark took a work turn's own compaction** (both reviewers): an ordinary
  turn compacts inside itself with the same `contextCompaction` item, and the mark —
  unbound, or even tied and re-tied — took it; the turn's report was lost and main was
  told `done`.
- **An operation past the record's 64 was dropped silently**, and the conversation lost
  its uncertainty while it was still open.
- **Main's answer named the wrong thing**: its wait and the hold were both 80 seconds,
  counted from different moments, so the wait always ended first and said the
  compaction had started even when its turn never appeared.
- The document called any later answer proof, and left out the server's refusal of a
  compaction as a way an operation closes.

Main decided: tie only an untied mark, never to a turn a reply named nor to one with
another item first, and never move a tied mark; an overflow is itself uncertainty until
the connection ends. Done that way, with one addition said in the document: either proof
may come after the compaction item — a reply travels apart from the events, and an
auto-compaction at a turn's start precedes its input — so a mark tied to a turn later
shown to be work is untied, which the source says never happens to a manual compaction's
turn. Main's wait now ends the hold, and its answer says whether the turn was seen. A
test the round made impossible — a reply naming the compaction's turn — now plays the
server's refusal instead. Each defect has a test that failed before the change,
review-codex's repros and review-claude's probe included.

**Round 5: safety held, attribution did not** (review-codex with repros, review-claude
with probes; both confirmed round 4's fixes, their overlays green and the full suite
92 of 95 with 3 unsupported). What remained was which compaction is main's:

- **A goal's turn tied after a gap** (review-codex): after a `/resume` away and back, a
  goal's turn that compacted first and failed with no other item closed main's
  compaction and gave main its failure.
- **The telemetry's author written too early** (both reviewers): the asker went on a
  compaction when its item completed, and a mark untied afterwards left it there.
- **Main's answer without a reply kept the hold**, and **an ended compaction racing
  main's wait** could be answered as not ended (review-claude).
- **A delivery's turn that ended before its reply was not published** (review-claude's
  probe): the mark had ended with it, and the reply made it work too late.

Main set three safety properties as what acceptance is judged by — no compaction sent
into a running or accepted turn, no work turn's report lost or withheld, no task settled
by a compaction's turn — with everything about which compaction is main's as best-effort
attribution. And decided: the mark ties only while the stream has been continuous since
the request; a change of selection before its turn was seen loses sight of it, and main
is told so. Done that way. Checking the third property turned up a hole older than the
round: a compaction whose turn came back after a `/resume` with no item seen — a
snapshot naming it running, or a run of unknown id — was published as a finished turn
and could settle a task. So a lost mark keeps any turn not shown to be work from being
published for the rest of the connection, at the cost of a goal's turn that shows no
item; said in the document's limits, with the goal's race in a continuous stream and a
turn whose reply comes after its end. The author is written at the end of the turn the
mark is still tied to; a reply naming a turn always makes it report. Each has a test
that failed before the change, review-codex's repros and review-claude's probes
included.

**Round 6: the third property rested on marks** (review-codex with repros, review-claude
with probes; the overlays of rounds 1 to 5 green, the full suite 92 of 95 with 3
unsupported, and the first two properties held in every late-reply case). A task could
still be settled by a compaction's turn in three ways, each a turn no mark recognized:

- **A compaction stopped before its item** — by Esc, or by its PreCompact hook, which
  the server runs after the turn started — was published as stopped.
- **After a goal's turn took the mark**, the race the document had named as touching no
  safety property, main's compaction then ran with no mark and was published as a
  finished turn with no text.
- **A compaction that outlived the connection** was published on the next one, where
  no mark had ever been set.

Main decided that the third property must not depend on attribution: a turn is
published only with proof of work — a successful reply to `turn/start`, `turn/steer` or
`review/start` naming it, or an item other than a compaction's — whoever started it,
whatever the marks say, on any connection, and when the proof comes after the end, then.
Done that way (`proof.go`): the proofs are kept for all the gateway's connections, the
outcomes waiting for one per connection. The mark-based withholding went with it, and so
did everything else that kept a mark's turn from publication. Checking the second
property against the source: the reply always names a turn of the terminal's or a
delivery's; its `userMessage` item does not always come, since a hook can stop the input.
A run seen only as statuses, its turn never named, might be either, so main decided it
is always reported, but as an advisory `stopped` that settles nothing and keeps the
waits; the next turn that finishes settles the task. Each of the three has a test that failed before the
change, and a turn of the terminal's and a delivery's reports in every order of its reply
and events, also across a reconnect.

**Round 7: open operations and unproven turns across a reconnect** (review-codex with
repros, review-claude with probes; the third property held everywhere, the overlays of
rounds 1 to 6 green, the full suite 92 of 95 with 3 unsupported). Two remained:

- **The first property after a reconnect.** The open operations lived in the
  connection's state, so an inline review accepted on one connection and still queued
  was forgotten; after a reconnect and an idle resume main's compaction was sent and
  would abort the review when it ran. The server keeps queued work when the terminal
  disconnects.
- **A turn with a known id ending with no proof** was dropped: a `turn/start` whose reply
  was lost with the connection and whose input a hook blocked, or a goal's turn failing
  at its first model call before any item. Its error never reached the waiter, and the
  task hung.

Main decided both. The open operations, and the record of what could not be held, now
belong to the gateway (`operations.go`), numbered across connections and cleared only
the same three ways on whichever connection; a request that may start a turn and whose
answer is lost with its connection is recorded as open then. The second property now
reads that no outcome is ever silently dropped: a turn without proof is reported as an
advisory `stopped`, with its error text when it has one, after half a second waiting for
its proof, and its own outcome still follows a later proof; a turn its
`contextCompaction` item shows to be a compaction reports nothing, which was chosen over
reporting compactions too, since a compaction is no outcome of anyone's task. A
compaction stopped before its item shows nothing of what it was and is reported as
advisory. Each finding has a test that failed before the change.

**Round 8: polish** (review-codex with repros, review-claude with probes; properties 1
to 3 held, the regressions of round 7 closed, the overlays of rounds 1 to 7 green, the
full suite 92 of 95 with 3 unsupported). Four were fixed:

- An advisory and a later proven stop of the same turn shared their identity, so the
  gateway dropped the stop as a duplicate and the turn receipts would have merged them,
  keeping its start and end from the pending mark. The advisory now has its own, the
  turn's id with `/advisory` after it.
- The uncertainty refusal said "after the next turn", which a goal's turn does not
  give; it now names a message delivered or a turn typed at the terminal. Operations
  open in 64 conversations, which nothing clears before the wrapper ends, have a
  refusal of their own saying so.
- A compaction whose item and end come on the next connection keeps its operation
  open, since the mark that names it ends with the first: a false refusal, now in Known
  limits, and the claim that the record clears the same three ways on any connection
  corrected. Tying an unnamed operation to the first compaction item was decided
  against, as it would bring back the goal race.
- The summary of the `stopped` kind names a turn that ended with no proof of work.

**Round 9: accepted** (September 25, 2026). review-claude and review-codex closed the
re-review of round 8's fixes with no findings, and review-codex accepted part B on the
Codex side after a live run.

**The live run of the acceptance** [live; Codex CLI 0.155.1; 2026-09-25], observer
review-codex, on the rewake built from the working tree of round 9, in a private state
directory: the subject a general session on `gpt-6-luna` at effort `low`, the main a
passive session. A first attempt with the subject started inside the observer's own
sandbox did not count, as its shell tools did not work; the run counted was the
subject started outside it, its own sandbox left on.

1. Compaction of an idle session: exit 0 in 5.9 s, "16505 tokens before, 4779 after
   (compaction 1)"; `rewake list` showed 1, and main received no compaction notice.
2. Compaction during a turn: exit 1, `in a turn (a turn is running)`. The turn, a
   `sleep 15`, ended normally, main received `finished — BUSY_OK`, and the count stayed 1.
3. Interrupt: exit 0; main received `stopped — lead-claude interrupted this turn with
   rewake interrupt`, and `rewake inbox --awaited` showed the same text. The terminal
   showed "Conversation interrupted - tell the model what to do differently".
4. Delivery around a compaction: sent right after `compact` returned, exit 0, delivered.
   Sent while a compaction ran, exit 3, pending, "a compaction of the conversation is
   running", and delivered after it ended; that compaction exit 0 in 8.7 s, "16902
   tokens before, 4992 after (compaction 2)". The transport held, but the `finished` of
   the new task was not seen: after each compaction the subject read only
   `rewake inbox --owed`, which leaves new mail unread. After the first it ended its turn
   with "nothing owed"; after the second it resumed the old interrupted task and reported
   `INTERRUPT_COMPLETED`. A defect of the role briefing, queued
   ([work-queue.md](../work-queue.md)).
5. A focus: exit 2, `focus not supported by Codex`; the count stayed 2.
6. An ordinary task, read with `rewake inbox`: main received `finished — Point:
   NORMAL_NEW_OK.`

**Still open:** nothing of part B. The focus for a Codex compaction stays queued as
research by the owner's decision.
