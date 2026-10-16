# Stage 4: a worker's turns through the mod

The design of step S4-6, written October 16, 2026 on branch `s4-6`, for review before any
code. It makes a Claude Code session of a reporting role report its turns: on 2.0 such a role
is refused on Claude Code, since nothing tells rewake that a turn ended, and 1.x did it with a
Stop hook in the person's settings, which 2.0 never edits. The mod reports the turns now.
It rests on the decisions of the owner and main of October 16, 2026, recorded below, on
[stage4-space.md](stage4-space.md#5-reports) and on the `mods` skill taken from Claude Code
2.1.296, the installed version that day. A call, event or field of the mod API is named only
to say what rewake does with it; its shape and limits are the skill's
([design-claude.md](design-claude.md#where-the-api-facts-live)). Its rules, its tests and
its live acceptance are in [stage4-turns-build.md](stage4-turns-build.md).

## Decided

By the owner and main, October 16, 2026:

1. **Reporting without holding.** Claude Code offers `TurnBoundary{Reports: true, HoldsEnd:
   false}`, live once the run's mod says hello. `classic.Stop` with `block`, which would hold an
   end (O1's confirmation, probe P3), is not in this step; what that costs is a known limit
   ([below](#what-not-holding-costs)).
2. **The session's own conversation only**, and the end's answer is the report.
3. **An end is retried with the same end id until its receipt comes**; the core takes it once.
4. **The start time is unknown.** The mod sends none, and the start is recorded with `at = 0`,
   never the arrival time.
5. **A heartbeat**: the mod keeps one held request, and when none arrives within a bound the
   host withdraws everything the mod proved.
6. **Admission**: a reporting role launches on Claude Code; a known cause that the mod will not
   load refuses it, named.
7. **A launch through `--command`** keeps the peer check as it is; what that means for a
   wrapper script is a documented limit, not a new mechanism.
8. **Readiness after the start** (main, the same day, from the Codex side's reading): a
   boundary that only the running harness can prove is designed for, not awaited before the
   harness starts.
9. **After the design's readiness check** (main, the same day): a long answer is cut at 1 MiB of
   the whole JSON body with a line saying so; every role's mod sends its own turns, main's
   included; an end of a run proven gone gets no receipt.
10. **The mod orders its own loads** (main, the same day, after the check of the fixes): each
   load takes the next number of a counter the run keeps in `$.store` and sends it in its hello;
   the host orders hellos by that number, not by when they arrive.
11. **Generated files and the 400-line rule** (main, the same day, during the build): the files
   `tools/protogen` writes, `protocol.gen.ts` and `protocol_gen.go`, are exempt from the rule, and
   `docs/rules/effects.md` is not split for the clauses this step names in it.

## The mod's side

**Which events.** `turn.start` and `turn.complete` of the main loop. A `turn.complete` that
carries `agentId` is a subagent's — main's keeper of S4-7, a workflow's agent, an engine's
fork — and is not sent; a subagent's run raises no `turn.start` (the skill, `TurnStartInput`,
`TurnCompleteFields`). Every role's mod sends them: main's turns report what a silent role
reports, its failures (`core/mail/turn_reports.go`), and the core decides that, not the mod.

**The reasons.** `turn.complete`'s `reason` is one of `answer`, `aborted`, `refusal`, `error`
(the skill, `TurnCompleteReason`), mapped to `answered`, `aborted`, `refused` and `failed`.
`error` is a turn ended by the service — retries exhausted, the context limit — so
`classic.StopFailure` adds nothing: no end of its own is sent, and the mod does not hook it.
The design's earlier table, which had `StopFailure` mark the turn failed for a later
`turn.complete`, predates the skill's union and is corrected in
[design-claude.md](design-claude.md#the-turn-boundary). The answer is `e.answer` as it is; a
refusal with an empty answer carries the refusal's explanation when the service sent one, so
the report says why.

**The start.** At `turn.start` the mod sends `turn/start` with the conversation
(`$.session.id()`) and the turn id, once, awaited in the hook, and does not retry it: with no
start time and no mail tool on Claude Code yet, a lost start costs nothing
([the start time](#the-start-time)).

**The end.** At `turn.complete` the mod makes the end — a fresh random `endId`, the
conversation, the turn id, the reason, the answer cut to its bound ([the size](#the-mods-side))
— and writes it to its outbox before anything is sent; from then on the end is frozen, and
every attempt sends the same end id and content, under the id of the load sending it
([the outbox](#the-mods-side)). The outbox has one sender. When the new end is its head and
nothing is in flight, the hook is that sender for the first attempt: it sends the end once and
awaits it before it returns. The wait is inside `$.http.fetch`, which the hook's 10 s budget
does not count (the skill, gotchas); the host answers within 20 s. When an older end is still
queued or in flight, the hook only appends: a later end never passes an earlier one, and the
hook does not wait on the chain, whose promise would count against its budget. Awaiting the
first attempt keeps the end ahead of the next turn wherever the engine runs the hooks of
`turn.complete` before it starts another turn; whether it does is not in the skill and is
checked live in part C. An end not acknowledged by then stays in the outbox, and the chain sends
it again.

**The outbox.** The ends not yet answered for good, in order, one in flight at a time. Kept in
two places: the module's memory, and `$.store` under one key per run, `rewake/outbox/<epoch>`,
the run's epoch read from its environment as the socket's path is — since a reload of the mod
resets the module's variables and cancels its timers (the skill, gotchas, on module state across
reloads), and `$.store` is shared by every session on the machine. An epoch names one run of one
session, so no other run's mod sends the entry. An end leaves the outbox, in memory and in the
store, only on its receipt or on a terminal refusal ([the retry](#the-mods-side)); its age never
removes it, whoever starts or reloads in the meantime.

A load reads its run's key at `session.start`, before it writes the key and whatever becomes of
its hello, and the ends it holds go ahead of any this load makes. A write replaces the whole key,
so nothing is written under it until that read succeeds: a read that fails leaves the queue
unknown, never empty, and is tried again after 1 s up to 10 s, said once with `$.ui.log`; the
ends this load makes meanwhile wait in memory, and none is sent, since an older one may still be
in the store.

An end is its end id and its content; the load id a `turn/end` carries names the admitted load
sending it. A load sends what the store holds for its run under its own load id, so an end
stored by a load whose hello never arrived, or by one since reloaded, is taken under the load
admitted now; the host checks that load's admission and the harness's peer as for any request,
and leaves the load id out of the capture's digest, so the end under a later load meets its
first capture. Decided while building part A, October 16, 2026, after its review found that
such an end, sent under the id it was stored with, was refused 400 and dropped: an end is
stamped by the load that sends it, not taken under a load the run never admitted.

The ends of a run that was killed go only once that run is proven ended. The store keeps an
index, `rewake/outboxes`, of the epochs that have a key of the run's: an outbox, or the load
counter ([the hello, again](#the-mods-side)). Once its hello is admitted, a
load asks `mod/ended` with up to 32 epochs of the index other than its own, and the host answers
those whose wrapper `registry.ObserveRun` proves ended — of another boot, or its process gone
with its start time; one alive or that cannot be told stays. The question changes nothing on
the host. The load removes both keys of each and
their lines of the index. It is the proof a later launch takes before it removes a killed run's
directory (L3), asked of the host because the mod reads no processes. Two sessions writing the
index at once may lose a line of it; that leaks a key, never an end, since a run's own load reads
its key by its epoch and not through the index. What the store keeps of a killed run outlives
the run until a later run of rewake on Claude Code says hello on the same machine: a known limit
against L3, stated there. A store that refuses a write — full, say — leaves the end in memory
alone, and the mod says so once with `$.ui.log`; a reload then loses it
([what not holding costs](#what-not-holding-costs)).

**The retry.** A chain of `$.clock.after` callbacks, the documented way to run work between
events: a timer's callback runs outside any event and keeps running between turns (the skill,
`official/api.md`, "Run work in the background"), and a reload cancels it with the old copy.
Each callback sends the head of the outbox once and schedules the next: at once after a
receipt, after 1 s, 2 s, 4 s up to 10 s after a failure. A receipt is a 200 whose body decodes
to a `Receipt` naming the end's turn and end id; a 200 that is not is a failure. What counts as
a failure is narrow: a fetch that threw, a 200 that is not the end's receipt, a status 500 or
503. A 410 says the run the end belongs to is proven gone — its name passed to a later run, or
its record removed — so no attempt has a destination: the end leaves the outbox without a
receipt, logged once with `$.ui.log`; it did not arrive, and nothing counts it as arrived. Any
other refusal — 400, 403, 409, 413 — says the end can never be taken as it is, and goes the
same way; a refusal repeated forever is not a retry. A new load sends what the store holds for
its run after its hello is admitted, oldest first, under the end ids they were made with and its
own load id.

**The size.** A request body is at most 64 KiB, and a turn's answer can be longer. `turn/end`
has a bound of its own, `maxEndBodyBytes`, 1 MiB, counted in the UTF-8 bytes of the whole JSON
body as sent: the escaping of quotes, backslashes and control characters, the ids and the
marker included. An answer that would carry the body past it is cut at the last whole character
that keeps the body within the bound with a line appended saying how many characters of the
answer were left out. The cut is made once, before the end is written to the outbox. The tail
of such an answer is lost on purpose: the report is bounded, says so, and is never refused for
its size (main's decision, October 16, 2026). Every id the mod makes is 24 characters, so an end
sent under a later load's id keeps the size its cut measured.

**The heartbeat.** An admitted hello answers the load's heartbeat `generation`, and the load
starts a timer chain for it: each callback sends `mod/alive` with the load id and that
generation and, when it is answered, schedules the next. The host holds each poll up to
`aliveHoldSeconds`. One chain runs per load at a time, and a reload cancels it with the old copy.

| Answer | What the chain does |
|---|---|
| 200 | sends the next poll at once |
| 409: the generation lapsed or is not the load's current one | if it is the newest generation the load was given, says hello again and runs a chain for the generation that hello answers; otherwise ends, since a newer chain of the load runs |
| 410: a later load superseded this one, or the run is gone | ends; the load says no hello again |
| 400, 403 | ends, logged once |
| a throw, 500, 503 | sends again after 1 s, doubling up to 10 s |

**The hello, again.** The hello of S4-5 stays the first act of `session.start`, and its
`loadId`, random per load, names its retries. Before its first hello a load takes its number:
it reads the run's counter, `rewake/load/<epoch>` in `$.store`, adds one, writes it back and
sends it as `loadSeq`. Loads of one run follow one another — a reload ends the old copy before
the new one registers — so the read and the write have no rival. A hello whose fetch threw, or
that was answered 500 or 503, is sent again under the same `loadId` and `loadSeq` from a timer
of the same kind, after 1 s up to 10 s, until it is answered; any refusal but 410 is final for
the load. A 410 says a load with a higher number holds the run. A load that reads no number in
the store above its own — a store cleared under it, say — takes the next number once and says
hello once more, so a live load is not left silent by a counter it cannot see; a second 410
ends it. So a hello lost on the way does not leave a mod that runs and is never admitted, a
hello sent twice is one load's, and a hello of an older load, however late it arrives, cannot
take the run from a newer one ([loads and generations](#the-live-set)).

## The host's side

**Where the code goes.** `modhost/server.go` is at 400 lines. Its dispatch becomes a table of
paths to handlers, the hello's handler and record move to `modhost/hello.go`, the turns' to
`modhost/turns.go`, the heartbeat and its lapse to `modhost/alive.go`. `modhost.Config` takes
the run's `TurnHandler` and a callback for what the mod proves; `modRun` in
`adapter/claude/mod.go` implements `TurnBoundary` and hands the handler to the server, and
reports the live set.

**Every request is checked as the hello is.** Admitted among the run's workers, held until the
host names the harness, the peer checked on arrival, the body decoded within its bound, the
peer checked again and the server's `closed` read under the lock that orders the effects,
before any effect; and the peer checked a third time before the answer (SP4). A request that
fails a check before its effect has none.

**A turn's start.** Taken from any load the run admitted: the handler calls `Started` with the
conversation, the turn and `at = 0`, and answers a receipt with the turn id.

**A turn's end, taken once.** Under one lock, the end's capture is looked up by its `endId`.
On its first arrival the handler captures it while the server is open — the read boundary
through `Capture`, `Ended` on the boot clock at that moment, `Started` zero — and keeps the
capture with a digest of what the request carried: conversation, turn, reason and answer. That
capture is the end from then on: every request of the same `endId` is handed to the core with
the same event, `claude/<endId>`, the same boundary, `Started` and `Ended`, so the core's
journal names one operation (`core/mail/turn_reports.go` digests the event, the outcome and
both times; E7). A request of a known `endId` whose digest differs is refused, 409, and
nothing is handed on. The capture is kept for the run's life, kept when `Publish` fails, and
kept across a newer hello: it is a boundary and two numbers per end, bounded by the run's
turns. `Publish` runs in the handler, counted among the run's workers; its success is answered
with a receipt naming the turn and the end, its failure with 503. A receipt lost on the way
proves nothing either way, and the retry under the same `endId` gets the same answer from the
core's journal.

**An end is taken whatever the live set says.** A heartbeat that lapsed withdraws what the mod
proved, so that no new task is sent to a run that may not report; it does not make an end the
harness process itself sends less true. An end from a load the run admitted is taken after a
lapse, and taking it makes nothing live again. The fixture refuses an end once its connection
lost the boundary (`adapter/fixture/turns.go`); that fits its program, whose connection is the
proof, and is not copied here.

**What refuses an end.** The peer (403); a body that is not the request, or no hello ever
admitted in this run (400); a known `endId` with another digest (409); the run proven gone
(410, below); a server closing (503). Once the run begins to end, admission among its workers
fails and the request is refused; the harness has exited by then, so no mod is there to send.
The `closed` check and the start of `Publish` are one decision under the lock, for a capture
already cached as for a new one: a handler admitted before the close that has not reached it
captures and publishes nothing, so no capture runs after the read clock closes
(`core/mail/read_boundary.go`), and one that began its `Publish` holds the run's lease until it
returns. No lock is held across a poll's wait, so a held poll never delays an end.

**A run that no longer owns its name.** `EndTurn` answers success with no effect when the run's
name has passed to a later run (`core/mail/turn_end.go`), and the same when the read of its
record failed: `registry.OwnsName` is false on either. That nil is never a receipt, since the
core took nothing. The build separates the two. Ownership is read as owned, proven not owned —
the record gone, or another epoch's — or unknown, a read that failed. Unknown returns the read's
error: the handler answers 503, the end is sent again and taken once the read succeeds. Proven
not owned returns `mail.ErrNameLost`, which `cli/completion.go` hands the adapter as
`adapter.ErrRunGone`; the handler answers it 410 with no receipt, and the mod drops the end
([the retry](#the-mods-side)). The adapter adds no check of its own: the core's answer, read
under the core's own order, decides. An end already taken whose receipt was lost is answered
from the journal while the run owns its name, and 410 after. The wrapper's other callers — the
fixture's program — answer `ErrRunGone` as they answered the nil, so nothing changes for them.

## The live set

**What the mod proves.** An admitted hello makes TurnBoundary live, and Spaces on a run that
hosts them. Wake stays the inbox lane's, live from the service's start.

**Loads and generations.** The host keeps, per run, the current load — its `loadId`, its
`loadSeq`, its heartbeat's generation, live or lapsed — and the load ids it has seen superseded.
A hello binds under the server's lock, ordered by its `loadSeq` alone, never by when it was
accepted or read:

- of the current load while its generation is live: a retry of a lost answer, answered with the
  same generation, nothing reported;
- of the current load after its generation lapsed: a new generation of the same load, and the
  set reported with what the mod proves;
- with a `loadSeq` above the current load's: it becomes the current load with a new
  generation, and the previous load is superseded;
- with a `loadSeq` below the current load's, or of a load id superseded: refused 410, with no
  effect.

The order exists before any request is sent: a load has its number before its first hello, and
a later load's is higher. So a hello of load A that arrives after B's — delayed while its body
is read, or a fetch issued before the reload and accepted after — is refused, and B stays
current. A hello of another load id with the current load's number means a counter that did not
advance — a store that refused the write — and is bound as the later load: there, and only
there, the order falls back to arrival, a known limit. A poll of the current load and generation
renews it; one of the current load and another or a lapsed
generation is answered 409; one of a superseded load, 410. An end of any load the run admitted,
superseded or not, is taken ([the host's side](#the-hosts-side)) and renews nothing.

**The lapse and the reports.** The server stamps each poll's arrival. A lapse is no poll of the
current generation arriving within `aliveLapseSeconds` of its last arrival, the hello counting
as the first; the watch is one timer per generation, and a timer that fires for a generation no
longer current does nothing. On a lapse the generation ends, and the run reports its whole set
without what the mod proved: Wake alone. Every transition — a hello bound, a lapse, the close —
takes the next number of one sequence per run under the server's lock, and its report carries
it; `modRun` drops a report whose number is not above the last it reported, so no late timer,
poll or report changes a later set, within one generation or across them. The watch is counted
among the run's workers and stops with the server's close; a lapse after the close reports
nothing.

**The bounds.** `aliveHoldSeconds` is 15: under the protocol's 20 s wait, and well under the
30 s after which `$.http.fetch` gave up in every probe run. `aliveLapseSeconds` is 30: a poll is
answered 15 s after it arrived and the next follows at once, so a lapse leaves 15 s for the
mod's turn — more than the 10 s of its own time a hook of any mod may hold the shared hooks
worker (the skill, gotchas), and room for one failed poll and its retry after 1 s. A lost mod
is noticed within 30 s of its last poll. A worker held longer than that, by another mod's
stall, lapses and comes back with a new hello; a lapse is loss of proof, not of the run.

**Loss the host sees no other way.** Claude Code unloads a mod that crashed the hooks worker,
and every non-built-in mod after three crashes, until `/reload-plugins` or a new session (the
skill, `official/admin.md` and `official/troubleshoot.md`), and tells rewake nothing. The
heartbeat is how that loss is seen. `/reload-plugins` and a worker replaced after a crash load
the module again, its `session.start` runs, and its hello opens a new generation.

**The shared part with S4-7.** Main's `events` loop is the same shape: a chain of
`$.clock.after` callbacks, each one held request, renewed on its answer, started after the
hello and gone with the load. On the host the arrival stamp, the table of loads and generations
and the lapse watch are one piece, `modhost/alive.go`, which S4-7 feeds from `events` polls as
well; nothing of `events` is built
here. The skill answers the first half of Q8 — a timer's callback runs between hooks; the live
check of part C answers the rest for this chain.

## Readiness and admission

**A boundary proven after the harness starts.** Today a launch of a reporting role waits, before
the harness starts, up to the readiness bound for its TurnBoundary (`host/wrap.go`,
`awaitBoundary`), and the wait ends as soon as the run reports anything (`host/live.go`,
`await`). The mod can only say hello once the harness runs, so that wait would refuse every
such launch. `TurnBoundaryOffer` gains `AfterStart`: the run proves the boundary only once its
harness runs. For such a boundary the launch does not wait before the start. Claude Code
offers `TurnBoundary{Reports: true, HoldsEnd: false, AfterStart: true}`; the fixture keeps its
readiness exchange before the start, and its wait stays as it is.

**A known cause refuses the launch** (the owner's answer 3). The run's service says which of
its offered capabilities it will not prove and why, through a new optional interface the host
asks after the service started and before the harness starts. Claude Code's knows, today: the
mod socket's path past what a unix socket allows even in its hashed form, a launch with no run
directory or no inbox socket of rewake's, the mod's files not written, the mod's endpoint not
listening; a run that loads no mod has its lane wrapped so that it can say so. For a role that
owes reports and a boundary named there, the launch is refused before the harness starts, exit
1, naming the cause by where it is (L2): "the rewake mod will not load for this run: its socket
path is longer than a unix socket allows; start it with a shorter session name or
`REWAKE_DIR`". A silent role goes on, with a note, as today. The causes the design-claude list
names that rewake does not read yet — flags and settings keys that disable mods, a person's own
plugin named `rewake` — are not detected in this step: such a run starts, its hello never comes,
and it takes no tasks.

**Until the hello.** The session record's live set has Wake alone, so a task or question to the
run fails, exit 1, through `refuseUnreported` with its words for a run that "has not proven, or
has lost" its boundary. Main learns that the worker is available through the availability
notice; for a reporting role whose boundary is proven after the start, availability waits for
TurnBoundary to be live, up to `adapter.AfterStartBound`, 30 s from the harness's start, so
the first task main sends is not refused for a hello still on its way. The bound is a first
guess, not a measurement: part C records how long the hello takes after the start, and the
bound is set from it. Past the bound the
availability goes ahead and the record says the boundary is not live; the wrapper writes
nothing on the terminal the harness holds. A hello that comes later makes it live, as any
hello does.

**A launch through `--command`.** The peer check is equality with the process the wrapper
started, not ancestry (`modhost/server.go`, `fromHarness`). A wrapper script that ends in `exec
claude …` keeps its pid and its start time across the exec, so the mod's requests come from
the named process and are admitted — read from the check and the kernel's process semantics,
not yet seen live; part C runs it. A script that runs Claude Code as its child is never
admitted: the run starts, its boundary is never live and it takes no tasks. rewake cannot tell
the two apart before the start, so that is a documented limit and not a refusal; a person's
wrapper for a reporting role ends with `exec`. This corrects S4-5's open item that such a run's
mod "is never admitted": it holds for a script that does not exec.

## The start time

The mod sends none, and the host records the start with `at = 0`, which records no timed
start (`host/tools.go`, `handler`). `turn.complete`'s `durationMs` is a length on the engine's
clock, not the boot clock the core's windows use; it is a possible source of a start later,
named and not built on. Two consequences, stated as limits
([what not holding costs](#what-not-holding-costs)):

- **No start proves a turn ended.** A shell `rewake pending` whose turn's end was lost cannot
  learn from a later start that its turn ended; only a later end on record shows it
  ([mail-bridge-turns.md](../mail-bridge-turns.md#a-pending-mark-at-its-turns-end)).
- **A window reaches back to the last end on record.** An end with `Started` zero has the
  window from the previous end of the run's journal to its own `Ended`
  (`core/mail/journal.go`, `TurnWindowStart`). When an earlier turn's end never reached the
  core, a pending mark of that turn falls into the next end's window: a final answer is
  published as pending, and the task stays owed until the end after it.

## What not holding costs

Accepted for this step, each a known limit dated October 16, 2026 and owned by stage 4: the
confirmation with P3 for the first, the start time for the second, the step that measures the
store for the last.

- **An interim end closes a task early.** A turn woken after `rewake pending` that ends without
  a new mark publishes its answer as final; O1's confirmation, which would ask once and hold
  the end, needs `classic.Stop` with `block`.
- **The windows of [the start time](#the-start-time)** above.
- **An end captured late.** An end whose first attempt fails is captured when it first
  arrives: a read or a pending mark the next turn made before that moment falls into its scope
  or its window. The first attempt is awaited in the hook so that this needs a failure of the
  host, not merely a slow one.
- **The window of a withdrawal.** A lost mod is seen within 30 s of its last poll, and a task
  sent in that time is accepted. A withdrawal and a send are not one step: a send that read the
  live set before the withdrawal landed is accepted (`cli/send.go` reads it, then writes), and a
  record whose write keeps failing leaves other sessions reading the set before. An accepted
  task stays owed; a withdrawal retracts nothing.
- **An end lost with its load.** An end the store could not keep is lost if the mod reloads
  before its receipt; the session's task stays owed and main sees it unanswered. So is one made
  while the store could not be read, and a store that is never read again holds every later
  end of the load in memory unsent.
