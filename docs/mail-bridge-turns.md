# The mail tool across a turn

What happens to a call of the mail tool after its answer, by the rules of
[mail-bridge-server.md](mail-bridge-server.md): which turn a call belongs to, how a read
is acknowledged, how a turn's end meets the calls of its turn, what becomes of a pending
mark that comes late, and the bound on every wait. **Built and accepted October 1, 2026**
([where the rules live](mail-bridge-server.md#where-the-rules-live)). The changes this makes to stage 1, the tests, what stage 3 must show live, and
what the first review found are in [mail-bridge-checks.md](mail-bridge-checks.md).

Everything here leans on what stage 1 already proves, and adds no parallel mechanism.
The read clock and the boundary an end captures from it, the journal every end with a
known time writes under the mailbox lock and keeps while its run lives, the window that
picks an end's pending marks, and the receipt of each operation are stage 1's
([turn-end-recovery.md](turn-end-recovery.md)). The one new record is the call's
binding.

## The turn a call belongs to

A turn's identity scopes two things: which observation a call matches, and which
earlier operation the same words join (stage 1's key is the run, the conversation, the
turn and the words).

- **Codex** names each turn once: a `turnId` is never reused after its `turn/completed`,
  and the gateway knows which turns are open. The ticket's turn is that id.
- **Claude Code** names a turn by the `prompt_id` every hook input carries, and the
  PreToolUse hook that observes the call carries its own, so the correlation needs no
  other event: the UserPromptSubmit telemetry stays what stage 1 uses it for, the start
  of an end's window. A `prompt_id` can outlive an end, though. A Stop that stage 1
  holds (`holdTurn`) publishes nothing and writes no journal: the turn goes on, and so
  does its identity. A Stop that another hook blocks after ours published lets the same
  `prompt_id` go on past an end that is on record. So the ticket's turn is the
  `prompt_id` together with the `Ended` of the latest end on record that ended after the
  wrapper first saw that `prompt_id`; journals it cannot read refuse the ticket. An end
  of an earlier prompt — an Esc the plugin reports, whose journal may be written late —
  ended before that first sight, so it never changes the identity of a later prompt's
  calls. The Stop hook is synchronous and writes its journal before the harness goes on,
  so a call after a published end always finds it.
- **The same words in a later turn** are a new operation only once no operation of
  theirs is unfinished or unknown: a call whose key finds no record first runs the
  shell's check (`unresolvedBefore`), and stops where the shell would, naming the
  receipt. A pending mark from an earlier turn is never replayed as this turn's.
- **A nested agent.** Codex runs a sub-agent on another thread, which the match refuses.
  Claude Code runs one in the same conversation, and whether its hook input always names
  the agent is unverified: on 2.1.284 a general-purpose subagent's call carried
  `agent_id` and `agent_type` (live, October 4, 2026), one kind in one run. A Codex
  sub-agent's call is not seen by the gateway, which forwards the primary thread's items
  only; before the revision it was refused as unreported after 2 s, which the channel
  took for a fault. Revised and built on October 4, 2026: the request's own `_meta.threadId` names the calling
  thread, so a request naming another thread, or none, is refused at once as an agent's
  call and never waits ([the channel](mail-bridge-channel-codex.md#conversation-connections)). A
  hook input that names one is refused. Until stage 3 shows
  which field a nested agent's call always carries, the Claude Code adapter refuses tool
  reads — `inbox` and `inbox --message` answer "read in the shell" — since only a read
  claims that a model saw something. A heads-up, a pending mark, `whoami` and `--peek`
  run, as the same words from a nested agent's shell would.

## Acknowledging a read

**What the child records.** Before a read's response is printed, stage 1 composes its
whole text, checks it against the bound, and records each part it carries as shown by
this call (`receipt.Shown`: the call id, transport and `CalledBoot`). Stage 2 adds the
hex SHA-256 of that whole text to each of those entries, as `Answer`, and makes the
read's stdout exactly that text, nothing before or after it. So the digest names this
call's answer — the receipt, every letter id, part, count and range it printed — and
not a letter's body alone.

**The evidence.** The observer gets the result the harness recorded for the call: on
Codex `item/completed` (`status`, `result.content`), on Claude Code the PostToolUse
input (`tool_response`, a list of content items) or PostToolUseFailure. It acknowledges
only when all of these hold:

1. the call's binding names a read's record;
2. the result is a success: `status` completed, or PostToolUse rather than its failure,
   and not `isError`;
3. the call is direct: on Codex outside code mode, which stage 3 sets with
   `omit_tools_from=["code_mode"]` and checks in the effective configuration; on Claude
   Code the main agent's;
4. the content, encoded as `EncodedSize` counts it, is within `ResultCap`;
5. the first content item is text, and its bytes — the JSON string decoded, nothing
   re-encoded or trimmed — hash to the `Answer` this call's entries record.

Only the parts this call's entries name are acknowledged, through `cli.AcknowledgeRead`
with the result in the evidence (`bridge.Exposure` gains it). A short body equal to a
line the server writes, two letters with the same body, a replacement that kept the
body, or a `--json` answer seen in another escaping: none hashes to the recorded answer.
Equality alone does not prove the model saw it: probe 2 found the full output in both
harnesses' completion events even where the model was shown a shortened one. That is
why the calibrated bound and the direct call stay conditions of their own.

## A turn's end meets its calls

**Two cuts, by what a commit carries.** An acknowledgment commits reads, which take
positions on the read clock, so it must land on one side of its end's boundary. A pending
mark takes no position: its `At` is fixed before it marks, and an end picks its mark by
that time, under the mailbox lock, when it writes its journal. So the two meet their end
in different places.

**A pending mark meets the journal.** Under the mailbox lock and before it marks, it
looks for an end of the run on record whose `Ended` is at or after its `At`; found, it
marks nothing, and a journal that cannot be read stops it as unknown (stage 1's rule 6).
The mark and the end's choice of its mark share that lock, so a mark written before the
journal is the end's to take, and one after it is never written. A mark may land after
the end's boundary was captured and before its journal: it is that end's mark all the
same, and the end, still unpublished, takes it. Nothing a report published is changed.
A child or the shell needs nothing from the wrapper for this.

**An acknowledgment meets the capture.** The wrapper hosts the endpoint, so every
acknowledgment runs in the process that captures the ends it can race: the Codex
gateway reading `turn/completed`, and the collector hearing the plugin's stop after an
Esc. The Stop hook needs nothing more: stage 1 scopes its end under the mailbox lock, in
the same hold that writes its journal, so an acknowledgment either committed before that
hold or meets the journal. For the other two, one mutex orders an acknowledgment and a
capture:

1. **The acknowledgment** takes its locks within its budget. Holding the mailbox lock,
   it looks for an end on record at or after its `CalledBoot`; then, under the mutex, for
   a noted one. Found either, it writes nothing — not the receipt's acknowledgment
   either. Otherwise, still under the mutex, it registers as writing, with an empty
   result slot of its own.
2. **It writes** to its end: no budget is checked between its writes, and none abandons
   a write it began. Every read, from the tool or the shell, commits under the mailbox
   lock it holds, so nothing else takes a position meanwhile.
3. **It closes** before it lets the mailbox lock go, whether its writes succeeded, failed
   or failed midway: it takes a snapshot of the read clock (stage 1's shared word, a
   memory read that cannot fail), then, under the mutex, puts that snapshot in its slot,
   once, and unregisters. Only then does it release the mailbox lock.
4. **The capture** takes the mutex and notes the end: the boot-clock moment of the
   capture, which is the gateway's `Ended`. If an acknowledgment is registered, the
   capture keeps a reference to that one's slot, lets the mutex go, and waits on the
   slot; the end's boundary is the snapshot found there. If none is, it samples the read
   clock as stage 1 does, before it lets the mutex go.

So the boundary is either the clock at the moment of the note, when no acknowledgment
could be between its check and its close, or the clock at the close of the one that was,
taken while it still held the mailbox lock. A read that commits after that lock is
released — the next holder's, from the tool or the shell — takes a position above it.
An acknowledgment that reaches its check after the note writes nothing. A slot is filled
once and only by its own acknowledgment, so a later one cannot stand in for it.

**What waits, and for what.** The capture's wait has no bound of its own: it is the
writes of one acknowledgment already past its check, which stage 1 does not time under
a held lock either, and while it lasts no read of the mailbox commits. That is the one
exception to rule 9, accepted for a stalled filesystem as everywhere in rewake. The
Codex gateway hands a read's completion to the observer and reads on; its reader stops
only in such a capture. On Claude Code, `rewake bridge-hook` forwards PostToolUse and
returns on the wrapper's reply or its timeout; the acknowledgment goes on either way. An
end does not wait for tickets, processes or acknowledgments that have not begun to
write, so the Stop hook keeps stage 1's bounds (a five-second mailbox wait, a hook
timeout of ten).

An acknowledgment that ran out of its budget or failed is dropped, never retried, and
the letter shows again through `inbox --next` or `retry`. The endpoint spends a call's
completion when it first hears it, before the acknowledgment reads its binding or waits
for a lock: a second record of the same call's result — repeated, or different — starts
nothing, whatever the first one's acknowledgment did.

**A ticket meets the capture.** The acknowledgment's check compares a noted end with
the ticket's `CalledBoot`, so a ticket issued after the note would let a call of the
ended turn write above its boundary. The wrapper therefore issues a ticket under the
same mutex, and only while no end was noted since the call was heard, or on Codex since
its turn started ([the ticket](mail-bridge-server.md#the-ticket)): a call heard before
the end gets no ticket after it, even while `turn/completed` has not reached the table.

**What this proves, and what it leaves to stage 1.** After the end is noted, no
acknowledgment that has not passed its check may begin writing. The acknowledgment
already registered is included through its closing snapshot; no read by a subsequent
mailbox holder can extend that boundary. A pending mark is never written after its
end's journal. Both are measured from the wrapper's capture. Two properties of
stage 1 are kept as they are, and stated here because they bound what a report means:

- **A turn the server starts on its own before the gateway reads the last one's
  `turn/completed`** — a queued review, a continuation — can commit a read before the
  capture, and the boundary covers it. Stage 2 does not widen that window: the reader is
  never held for an acknowledgment's waits. Closing it is a stage 1 task; until then no
  rule here claims more than the capture's moment.
- **A turn start that failed to be recorded** lets the next end's window reach back to
  the mark of a first attempt an Esc cut off, so that end may report interim when it
  finished. The obligation stays open; no wrong success follows from it.

## A pending mark at its turn's end

A pending operation records its text, its time `At` (the ticket's `CalledBoot`, or the
process start in a shell) and its mark's file name before it marks (stage 1).

**Who may mark.** Only an attempt that runs in the operation's own turn writes its mark:

- the first attempt, the call that created the record. It is the call itself: its turn
  can end under it only by an end, which a heard end puts on record under the lock, or
  by an Esc no one heard, which cut the call off, so no model reads its answer;
- a later tool call whose ticket names the same turn as the operation's key (stage 1's
  scope carries the turn): the same words joining in the same turn, or `retry` and
  `--next` from it. On Claude Code that also needs a `prompt_id` that is never reused
  for a later prompt, Esc included; until stage 3 shows that live, only the first
  attempt marks there.

Any other attempt does not mark: a `retry` from the shell, which names no turn; a tool
call from another turn; a call on an operation the shell began, whose key has no turn.
None of them can show that the operation's turn is still open. The record of a later
turn's start does not show it either: UserPromptSubmit writes it in the background
(stage 1, `Async`), so a call of that turn may run before it is written, or after its
write failed. The rule uses a recorded start only one way: one after `At` proves the
operation's turn ended. Its absence proves nothing.

Under the mailbox lock, just before the mark, the attempt decides:

| Found under the lock | Proven | The operation |
|---|---|---|
| its mark file | marked, by an earlier attempt, before any end on record | done: "marked" (its replay below) |
| no mark file; an end on record at or after `At`, or a turn start recorded after it | not marked; no later end's window can hold `At` | done: "not marked: the turn ended first", exit 1 |
| no mark file, none of those, and the attempt is in the operation's own turn | not marked yet, and the turn open as far as this call is part of it | marks, then done |
| no mark file, none of those, and the attempt is not shown to be in that turn | not marked; whether its turn is open is unknown | done: "not marked: this call cannot show it runs in the turn the mark was for", exit 1 |
| the mark or a journal unreadable | — | stays open (stage 1's rule 6); `retry` decides later |

So a child that saved its step and died before the mark is finished by the first that
comes: its turn's end, after which a `retry` from anywhere finds the end and answers not
marked; or a `retry` from that same turn, which marks; or a `retry` from the shell or a
later turn, which answers not marked and says why. None of them waits for the start of
the next turn to be written. The proof is stage 1's: every write of a mark and every
journal share the mailbox lock, and a journal stays while its run lives.

**A replay** of a marked operation gives its recorded answer only to a call in the
operation's own turn. Any other call is told "this mark was for an earlier turn, or one
this call cannot name; this turn is not marked by it", with exit 1. The same holds in
the shell. Erring this way costs a second mark at most, which the end takes as the
latest in its window; the other way would tell a later turn it waits when it does not.
So a finished report, published when its window held no mark, is final: no mark can
arrive for that window later.

## Waits and their bounds

| Wait | Bound | Running out means |
|---|---|---|
| PreToolUse hook for the wrapper's reply (an in-memory record) | 2 s | exit 0, nothing printed: no observation, the call runs nothing |
| a ticket request for its observation | 2 s, on the call's own channel, holding no lock the observation needs | refused, nothing ran |
| a ticket's confirmation (in memory) | 1 s | refused, nothing ran |
| an acknowledgment, for the receipt lock and the mailbox lock together | 2 s (stage 1 waits 10 s for each), checked before its first write and never after | nothing written; the letter shows again |
| an acknowledgment past its check | none of its own: writes under a held lock, as everywhere in rewake | it finishes; while it writes, no other read commits |
| an end's capture, for an acknowledgment already writing | that acknowledgment's writes, the one exception to rule 9 | — ; the boundary is the snapshot it closes with |
| PostToolUse hook for the wrapper's reply | 2 s plus 1 s | exit 0; the acknowledgment goes on and meets its end by rules 7 and 8 |
| the Stop hook | stage 1's: a 5 s mailbox wait, a 10 s timeout | stage 1's: what it could not do stays owed |
| endpoint connections besides the server's | 16 at once | refused at once, as the matching wait running out |
| a child | its deadline plus 5 s | SIGKILL to its group; one that does not die keeps its slot |
| the server at EOF | its children's hard bounds, and 1 s past the kill or the EOF for a child not reaped | it kills what is left and exits; a call whose child outlived the kill answers unknown |
| the server's stdout, once stdin ended or while input backs up behind it | 2 s with nothing written | it exits as one killed outright; an answer not written reads nothing |

The endpoint serves each request on its own; the observation table is held only to
record or take an entry, never across a wait, and an acknowledgment runs outside it.
Writes under a held lock are not timed, here as nowhere else in rewake: a filesystem
that stalls stalls them, and an end captured meanwhile with them — the one place a
capture waits, and while it waits no read of the mailbox can commit.

## Failure points after the answer

| Point | Proven | Unknown | Then |
|---|---|---|---|
| no completion observed for a read call | the CLI recorded its parts as about to be shown | whether they were printed, and whether the result reached the model | not acknowledged; the claim holds; the letter shows again |
| a result other than the recorded answer, `isError`, not direct, or over the bound | the exposure is not proven | whether the bytes reached the model | not acknowledged |
| the binding missing or unreadable at completion | — | which read the call showed | not acknowledged; a later call shows the letter under its own binding |
| an acknowledgment out of budget before its locks | nothing written | — | dropped; the letter shows again |
| an acknowledgment's write fails | stage 1's: what `settleLetters` wrote | stage 1's | the letters stay unread and show again |
| an acknowledgment checks after its turn's end is on record or noted | nothing written | — | dropped; the end stands as captured |
| an end captured while an acknowledgment writes | that acknowledgment's commits | — | the boundary is its closing snapshot; the end counts them, and no read after its release |
| an acknowledgment's write fails midway | stage 1's: what it wrote | — | it closes with a snapshot all the same; a capture waiting on it gets that |
| a repeated completion | — | — | the endpoint spent the call at its first completion; nothing runs |
| a pending mark checks after its turn's end is on record | not marked | — | done as not marked; a replay says so |
| a pending mark lands after its end's capture, before its journal | marked in that end's window | — | the end takes it; nothing published changes |
| a pending step saved, the child dead before its mark | not marked while no end is on record | whether the turn is still open | the end, or a `retry` from that turn, decides; a `retry` from elsewhere answers not marked |
| a later turn's start not yet written, or its write failed | — | whether the operation's turn ended | proves nothing; only a call in the operation's own turn marks |
| Stop held by stage 1 | no end recorded | — | the turn goes on, with the same identity |
| Stop blocked by another hook after ours published | an end on record | — | the turn goes on under a new identity |
| the plugin's end and the hook's for one turn | stage 1's | stage 1's | each is an end of stage 1; nothing here depends on which came first |
| a call from a nested agent on Claude Code | — | whether every such call names its agent | a named one refused; reads refused until stage 3 |
