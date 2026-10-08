# The mail channel of a run

Part of stage 3 of [mail-bridge.md](mail-bridge.md), beside the launch injection of
[mail-bridge-launch.md](mail-bridge-launch.md): what the wrapper records about how this
run's mail travels — through the tool, through the shell, or neither — what it tells
the worker and main, what `whoami`, `rewake list` and main's header show, and what the
briefing says. **Design, third pass, October 4, 2026; built the same day**, with the
readings in [as built](#as-built); corrected in three acceptance rounds and its code
accepted in the fourth, the same day; the live checks ran the same day, and the rules
they changed are marked *revised* and were built after them, the same day. It replaces the single state table
stage 2 left for it, which let a hello stand for a usable tool and could keep a shell
failure forever.

**Cut to the neutral alphabet in S8, October 8, 2026.** The record no longer knows a
harness: its events are a hello, a close, a refused hello, a validated ticket, the timer
passing, a denial, a shell observation and the exit. What only one harness told it — a
Claude Code session start, a call no hook observed, a Codex conversation selected or
bound, a server reporting its command cannot start — left with the 1.x adapter and the
MCP injection ([archive/1.x/codex](../archive/1.x/codex/README.md)). No launch starts
the keeper until a harness offers a tool transport again (stage 3, S10 or S11): until
then every run shows `mail: unknown`, and the text below is the record that keeper will
keep.

## The rules

1. **Two observations and a block, one derived display.** The wrapper keeps a tool
   observation, a shell observation and a policy block, each with its own evidence and
   time; the state shown is derived from them and never stored apart.
2. **Connected is not working.** A hello proves a server reached the endpoint; only a
   child that validated its ticket proves the tool carried a call. A denial is neither.
3. **Silence proves nothing.** No hello, no call and no shell command are not failures;
   a wait for a hello names what was not observed, never what did not happen.
4. **Each channel has one kind of evidence.** The tool's is a child validating its
   ticket, whatever its words; the shell's is a mail operation that wrote under the
   mailbox's or the receipts' lock, or failed reaching them. Nothing else — a refusal
   before a ticket, a guide in the shell, a hold — moves either observation.
5. **Events are ordered by when they happened, not by when they were folded.** Every
   event carries the boot-clock time of the event itself — a ticket issued, an EOF read,
   a timer passing, a shell write done — and the record is folded by those times, never
   by arrival: the tool observation is derived again from the transport's history on
   every event, so one that arrives late lands where it happened and what followed it is
   judged again with it. Events at the same time are ordered by a fixed rank of their
   kinds — the starts a timer waits for, hellos, closes, then failures — and by
   generation. A failure opens an interval that later evidence only closes; the
   interval starts at the earliest failure of the spell and shows the latest one's
   class, by those times, whichever is folded first.
6. **A notice states an action first and fits the preview.** Its first line is fixed by
   the derived state and class, never by a raw error, and reaches the reader whole
   without a read.
7. **A policy refusal is not routed around** (the owner's decision of October 4, 2026).
   A call the harness denied for permission is a defined outcome: main is told once, the
   worker is given no shell advice, and nothing beyond this table is built to detect or
   probe it — the owner does not expect to deny the tool.
8. **Nothing is relaunched, granted or approved** by the channel, and nothing it says
   continues past the harness's exit.

## The tool observation

**Connections.** Each hello the endpoint accepts opens a connection with a generation
number counted up per run, the server's pid and its start time. The transport is the
set of live connections, counted per connection by event time: a connection is live
from its hello to its own close, a close removes only its own connection, and the
server is gone only when no connection is live at the close's time — whichever
generation is newer, and whatever order the endpoint's goroutines told them in. The
endpoint already admits up to four, enough for a restarted server overlapping its
predecessor and for nested threads.

| Event | Source | The tool observation becomes |
|---|---|---|
| launch, tool offered | the plan | **starting**; the hello timer runs from the launch (below) |
| launch, no tool | rule 4 of the launch | **no tool** with its reason, for the whole run; its interval opens at the launch |
| a hello accepted | the endpoint | **connected**, unless working; the timer cancelled; an open interval stays open |
| a child validated its ticket | the endpoint | **working**; the interval closed |
| the last live connection closes (above), harness alive and not ending | the endpoint | **failing**, class "server gone": no harness is taken to start a dead server again on its own |
| one connection closes, others live at its time | the endpoint | unchanged, whichever generation is older |
| a hello refused (build mismatch, capability) from a descendant of the harness, no connection live | the endpoint, by pid ancestry | **failing**, class "server refused" |
| a hello refused from a process outside the harness's tree, or while a connection lives | the endpoint | unchanged, logged |
| a call refused before its ticket: busy, a timeout too short, conflicting ids, a completed turn, an agent's call | the endpoint | unchanged: that call's outcome, told to the model in its answer |
| a call timed out, was cancelled or stopped by Esc, or failed while its connection lives | the harness, as the call's own outcome | unchanged: the call's outcome is its receipt's, and its child may still be running |
| the hello timer passes with no connection | the wrapper | **failing**, class "no hello observed" |
| the run ends: the harness's process exits, or the wrapper accepts SIGTERM or SIGHUP, or loses the backend it would end the harness for | the wrapper | frozen: no further event, no notice, nothing fixed is written; open waits cancelled |

So the transport failure classes are a closed list — server gone, server refused, no
hello observed — and each comes from the
endpoint's own evidence or its timer, never from a harness's error text. A timeout or a
cancellation is not among them: stage 2 leaves that child to its deadline, and a notice
sending the worker to the shell then would invite a second operation beside a live one.

**The end of a run** is what the wrapper itself knows of it (main's decision of October
4, 2026): the harness's process exit as the wrapper observes it, and the wrapper's own
teardown — a SIGTERM or SIGHUP it accepted, or the backend gone, for which it ends the
harness. A harness that ends on its own is seen at its process exit only. Nothing
earlier is taken for an end: not an EOF, which is what the end would explain, and not a
harness's own end-of-session signal, until a live check shows it tells an exit from a
cleared conversation and comes in time. So a close is folded only once a full heartbeat has
passed since it happened (below): a server that closed less than that before the exit
was the end's, one that closed earlier was a failure while the harness ran.

**The failure interval** opens at the first failure event after the tool last worked,
or at the launch for "no tool", and records that event's time. Later failures while it
is open change the class shown to the latest by event time, and move the start only
when one happened before it — a failure folded after a later one. A hello does not
close it — a server that reconnected has carried no call yet — and only a validated
ticket does. A ticket ends every failure up to its own time, whenever it is folded: when
failures after it were folded first, the interval goes on from the first of them, and
when none, the tool is as the ticket and what came after it left it — a later close
opens a new interval as any server gone, and a later hello leaves it
connected, not working.

Every failure holds only while no connection lives — a server gone, a refused hello, a
timer passing. A hello folded late, at a time before such a failure, shows a server was
live then, and the failure never happened: the interval it opened goes, or starts at the
next failure. A close folded late does the reverse.

**The hello timer** waits for the start the run expects, never for an idle server. It
runs 15 s from the run's start, the time the keeper opens the record at launch: a bound
chosen with room over the cold starts of probe 2, not a measured norm. It stops at a
hello, at a validated ticket — which proves a server is connected as a hello does — or
at exit. It passes at its end, which is the failure's time, once the wrapper's heartbeat
or a later transport event shows the end has gone by; a hello at the end itself is in
time.

Until S8 the timer waited for a harness's own start — Claude Code's first SessionStart,
since its servers start only after every startup dialog is answered, and Codex's first
admitted thread — and a second one ran from the first call of the tool seen with no
connection. Both signals left with the 1.x adapter; an adapter that offers a tool
transport again and starts its servers later than its launch tells the record its start
through that capability, which S10 defines.

A hello after the timer passed is a late start: the observation becomes connected, and
the interval closes with the next validated ticket as after any failure.

## The policy block

The harness's denial of our tool for permission sets the block, at the boot-clock time
the wrapper records the native signal (**gate L4** names the signal; until it closes,
no block is ever set, and a denied call is only the harness's own answer to the model).
A later denial moves that time forward, never back, so the block's boundary is the
latest denial by event time (main's decision of October 4, 2026); a denial older than
the newest validated ticket's issue was lifted by that ticket already.
The block is kept apart from the transport: a hello, a disconnect, a failed or timed-out
hello, a timer, a recovered transport — none of them clears it or brings back any
fallback advice — and nothing of the transport delays it: a denial takes effect when the
wrapper is told, whatever transport event waits to be folded. It is cleared by one event only: a child validating a ticket the
wrapper issued after the latest denial. A ticket issued earlier proves nothing about the
denial, however late its call answers. While the block is set, transport events still
update the tool observation silently; when it clears, the derived state is computed
afresh and told as any change.

A denial of one call by the person's own PreToolUse hook is not this: that call's answer
says so, and the channel is unchanged.

## The shell observation

The CLI of a run writes one small file per observation under
`receipts/<epoch>/channel/`, named by the boot-clock time in nanoseconds of the event —
when the write under the lock completed, or when reaching the state failed, read as it
happens rather than when the command returns, since a `send` may wait for delivery long
after its write — and its pid, so concurrent commands never overwrite one another. It holds the outcome, success
or failure with its class, and that time. The wrapper reads the directory at its
heartbeat, folds it to the newest observation by the recorded time, and removes what it
folded.

| Outcome | Counts as | Examples |
|---|---|---|
| a write under the mailbox's or receipts' lock completed | **success** | `inbox` that moved a letter it showed, `pending` that marked, `send` that wrote a message, `retry` that wrote |
| the run's mailbox or receipts could not be reached: `EROFS`, `EACCES`, `EPERM`, an I/O error on the lock or the file | **failure**, its class | a sandbox that turned the state directory read-only |
| anything else | nothing | `inbox --peek`, which writes nothing; `guide`, `--help`, exit 2, a refusal by the target, a delivery hold, a lock wait that timed out |

The CLI writes an observation only while the channel is not working — the tool
starting, failing, blocked, or no tool — which it learns from the run's
channel file the wrapper keeps; when the file cannot be read it writes. A call made by
the tool's own child writes nothing: it is the tool's evidence, through its ticket.

An observation whose event time is earlier than the start of the open interval confirms
nothing about it, so a late success of an earlier spell, however late it is folded, is
not this spell's shell: the display compares the two times.

## What is shown

The display is derived from a closed list of categories; the block comes first, then the
tool, then the shell observed since the interval opened.

| Category | When | `whoami`, `rewake list` and main's header |
|---|---|---|
| denied | the block is set | `mail: tool denied by policy` |
| tool | working, no interval open | `mail: tool` |
| tool pending | starting or connected, no interval open | `mail: tool starting` / `tool connected, unused` |
| tool failing | an interval open on the tool, shell none | `mail: tool failing (<class>) since <T>; shell unconfirmed; tool last worked <T'>` |
| shell | an interval open, tool or no tool, newest shell success | `mail: through the shell since <T>; tool failing (<class>)` or `no tool (<reason>)` |
| no channel | an interval open, tool or no tool, newest shell failure | `mail: no working channel: tool <class>` or `no tool (<reason>)`, `shell <class>` |
| no tool | no tool, shell none | `mail: no tool (<reason>); shell unconfirmed` |

A connection that came back during an open interval adds `; reconnected <T>, unused` to
the line and changes neither the category nor the class, so it sends no notice; a
ticket closes the interval. `whoami` prints the line
in every run, with the reason in words the diagnostics allow
([mail-bridge-launch.md](mail-bridge-launch.md#the-refusal-and-what-may-be-shown)).
`rewake list` shows the category's word (`denied`, `tool`, `starting`, `failing`,
`shell`, `none`, `no tool`); the `--json` forms carry the observations and the block.
The record lives in session state beside `DeliveryHold`, written by the wrapper only.

## Notices

The wrapper sends them itself as notes from the run's name, through
`inbox.PublishOnce`, as it sends hold notices.

**The first line is fixed per category and class** and carries no name: the notice
already shows the sender, the run. It is at most 55 cells, because a grouped notice
puts `<sender> notify: ` before it — 41 cells with a name of the longest allowed, 32
characters — inside the 96-cell preview (`internal/harness/notice.go`, `preview.go`). So
it reaches the reader whole, alone or grouped, and a reader need not open the mail —
which may be the operation that fails. The action comes first; the second line carries
the class, the times and the reason in the allowed words. A test renders every first
line through `harness.Notice` with a 32-character sender, alone and grouped, and
requires it uncut.

| Category entered | To | First line |
|---|---|---|
| tool failing | worker | `rewake tool failed; new calls in the shell, no repeats.` |
| tool failing | main | `rewake tool failed; shell not yet confirmed.` |
| shell | main | `mail goes through the shell.` |
| no channel | main | `no mail channel works; see whoami there.` |
| denied | main | `rewake tool denied by the person's policy.` |
| tool, after a failure or a denial was told | main, only one told of it | `rewake tool works again.` |
| no tool, at launch | main | `started without the rewake tool; shell only.` |

The worker's second line: `a call whose outcome is unknown: rewake retry <token>; never
its words in the shell.` A denial sends the worker nothing, and no notice outranks the
briefing: a call the harness refused for permission is not made in the shell whatever a
notice said before. No tool with a failing shell is the category "no channel", and main
is told it like any other. Until S8 a server gone on Claude Code gave main's notice a
second line naming the person's way back through `/mcp`; it left with the record's
harness, and an adapter that has such a way back will say it through its own text. Worker
and main are each told once under the windows below, as for any failure.

**Suppression and publication are apart.** Suppression is keyed by the run, the
category and its class from the closed lists above — never a call ID, a path or an
error text: one notice per key per ten minutes and no more than six per hour from one
run to one recipient, both bounds chosen, not measured, and both counting published
notices only. One notice to a recipient is in flight at a time: while one is fixed and
not settled, no other is fixed for that recipient, so notices fixed while a mailbox
cannot be written never land together past both bounds once it can. When a window ends, the current category is sent if it differs from the
last one told, so a failure that stayed suppressed is told once its window passes.

Publication has its own identity. A notice the windows allow takes the next number of
the run's notice sequence; its recipient's name and epoch, the number and the whole
body are written into the record before the first attempt, and the message ID is a
hash of the run, the recipient, the recipient's epoch and the number. A failed write is
retried from the record with that same ID, so it lands once; the windows count it when
it lands. A notice fixed this way is published as fixed even if the state has moved on —
its second line carries its time — and the current state follows under the window rule,
with two exceptions. A notice to the worker advising the shell that has not landed when
the block is set is dropped, not published, and so is any notice not written when the
record froze: `PublishOnce` runs the wrapper's check under the recipient's mailbox lock
only when the letter is not already in the mailbox, and the check refuses while the
block is set or once the record froze, so nothing is written, and the record marks the
notice dropped so no retry takes it up. Its ID and body are never reused or rewritten.
A letter that landed before the block stays as it is; the briefing governs it.
A later notice of the same category, an hour or a minute later, has a new number and so
a new letter. The main's run is read again under its mailbox lock, before the letter is looked for:
a notice whose main ended while the keeper waited for that lock is dropped, as one for a
main that left before it. A main of a new epoch gets the current category once under its own number,
unless it is "tool" and nothing was told before; with no live main the notice waits in
the record for the next.

## The briefing

The transport sentence of the specification, with the lines the rules above require:

> Run `rewake <words>` through the `rewake` tool when you have it, otherwise in the
> shell. A call the tool answered "nothing ran" may be made again in the shell. A call
> the harness refused for permission is not made another way; main is told. A call
> whose outcome is unknown is never repeated in the shell: continue it with `rewake
> retry <token>`, or with the same words through the tool in the same turn; with
> neither, leave it and say in your report which call it was.

The shell joins no operation by its words (the CLI's rule,
[mail-bridge-cli.md](mail-bridge-cli.md)), and the tool joins one only in the same
verified turn; so after a commit whose answer was lost, the same words anywhere else
would be a second message. The sentence is in every role's briefing of a run that has
the tool, and the first clause alone in one that has not.

## Failure points of the channel

Every point the rules above meet — what is proven, what is unknown and what rewake does —
is a table of its own, with the sequences the generated tests assert:
[mail-bridge-channel-failures.md](mail-bridge-channel-failures.md).

## As built

The record, its events and notices are `internal/channel` (`record.go`, `events.go`, `history.go`,
`display.go`, `notices.go`); the wrapper's keeper is `internal/wrap/channel.go`, the
shell's side `internal/cli/shell_channel.go`, `internal/receipt/shell.go` and
`internal/state/reach.go`. Where the text above left a choice, the code reads it so:

- **A close waits a full heartbeat** (1 s) after it happened before it is folded, with
  whether the harness still lives then; at the end the held closes fold as ones the
  harness did not outlive, which is no failure. Nothing else waits: every other event,
  a denial among them, folds when told, so the check under the mailbox lock sees a
  block at once, and the close, folded late, lands where it happened by event time.
- **The record keeps the transport's history** in the wrapper's memory, never in the
  session state (`internal/channel/history.go`): every connection's hello and close by
  generation, for the run, and the refused hellos since the last ticket, which ends
  everything before it. It grows by one entry per server connection and one per refused
  hello since the last ticket. The policy block, the shell observation and the ticket's
  own times are kept as the latest by event time and need no history. No order of
  arrival changes the record: `internal/channel/order_test.go` folds every four events
  of the alphabet, two connections' hellos and closes among them, in all their orders;
  the generated space (`space_test.go`) walks every sequence of its letters to depth 4
  from both starts, with the tool and without, and folds each path again by event time
  and in reverse; `silence_test.go` holds that a run with nothing observed shows no
  failure; `connections_test.go` folds two connections across a ticket, the timer's end
  and a late hello in every order against states written out by hand; and
  `internal/bridge/endpoint/channel_order_test.go` holds one connection's hello, or a
  refused hello, behind another connection's events on a real endpoint.
- **Only the exit is taken in arrival order**: an event told after the record froze is
  not of the run (rule 8), and the keeper folds its held closes before it freezes.
- **The channel writes no grant and decides no permission**:
  `internal/wrap/channel_effects_test.go` tells the keeper every kind of the alphabet
  and finds only the notices' letters written under the state directory.
- **Main is told only of another run's channel.** A main's own notices go to it as the
  worker of its run; the main copy would be the same news twice.
- **A notice fixed for a main that has left is dropped**, not delivered to its
  successor: the new epoch gets the current category under its own number, as above.
- **A shell success is any write the command completed in the state directory**, which
  for the commands counted is the write under the mailbox's or receipts' lock. A `send`
  accepted but not delivered yet (exit 3) wrote its message and counts; "a delivery
  hold" in the table means the hold itself, which adds nothing.
- **A state the CLI cannot write loses its own failure observation**: the observation
  lives in that state. The display then says the shell is unconfirmed, which is true,
  and the tool's evidence is unaffected.
- **The list's Mail column**, like its other state columns, is shown to main only;
  `whoami` shows the line in every run, `mail: unknown` where no record was kept.
- **The observations are their own record kind** (`receipts/<epoch>/channel/<boot>.<pid>`),
  which the turn-end barrier does not open as receipts.
