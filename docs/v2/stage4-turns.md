# Stage 4: a worker's turns through the mod

The design of step S4-6, written October 16, 2026 on branch `s4-6`, for review before any
code. It makes a Claude Code session of a reporting role report its turns: on 2.0 such a role
is refused on Claude Code, since nothing tells rewake that a turn ended, and 1.x did it with a
Stop hook in the person's settings, which 2.0 never edits. The mod reports the turns now.
It rests on the decisions of the owner and main of October 16, 2026, recorded below, on
[stage4-space.md](stage4-space.md#5-reports) and on the `mods` skill taken from Claude Code
2.1.296, the installed version that day. A call, event or field of the mod API is named only
to say what rewake does with it; its shape and limits are the skill's
([design-claude.md](design-claude.md#where-the-api-facts-live)). Its rules are in
[the rules](#the-rules), its tests and live acceptance in [the build](#the-build).

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
conversation, the turn id, the reason, the answer — writes it to its outbox and sends it once,
awaited in the hook, before the hook returns. The wait is inside `$.http.fetch`, which the
hook's 10 s budget does not count (the skill, gotchas); the host answers within 20 s. Awaiting
the first attempt keeps the end ahead of the next turn wherever the engine runs the hooks of
`turn.complete` before it starts another turn; whether it does is not in the skill and is
checked live in part C. An end not acknowledged by then stays in the outbox, and the outbox is
sent again from a timer.

**The outbox.** The ends not yet acknowledged, in order, one in flight at a time. Kept in two
places: the module's memory, and `$.store` under a key naming the run's mod socket and the end
id, since a reload of the mod resets the module's variables and cancels its timers (the skill,
gotchas, on module state across reloads), and `$.store` is shared by every session on the
machine. The socket's path names one run of one session, so no other run's mod reads
the entry. Each entry carries when it was written; an entry of any run older than 24 hours is
removed at `session.start`, which keeps the 4 MiB store from filling with the ends of runs that
were killed. A store that refuses a write — full, say — leaves the end in memory alone, and the
mod says so once with `$.ui.log`; a reload then loses it
([what not holding costs](#what-not-holding-costs)).

**The retry.** A chain of `$.clock.after` callbacks, the documented way to run work between
events: a timer's callback runs outside any event and keeps running between turns (the skill,
`official/api.md`, "Run work in the background"), and a reload cancels it with the old copy.
Each callback sends the head of the outbox once and schedules the next: at once after a
receipt, after 1 s, 2 s, 4 s up to 10 s after a failure. What counts as a failure is narrow: a
fetch that threw, a status 500 or 503. Any other refusal — 400, 403, 409, 413 — says the end
can never be taken as it is, so it leaves the outbox and the mod logs it once with `$.ui.log`;
a refusal repeated forever is not a retry. A new load sends what the store holds for its run
after its hello is admitted, oldest first, under the end ids they were made with.

**The size.** A request body is at most 64 KiB, and a turn's answer can be longer. `turn/end`
has a bound of its own, `maxEndBodyBytes`, 1 MiB; an answer that would carry the request past
it is cut by the mod at the last whole character that fits a margin under it, and a line is
appended saying how many characters of the answer were left out. The cut is never silent, and
no request is refused for its size.

**The heartbeat.** After its hello is admitted the load starts a second timer chain: each
callback sends `mod/alive` and, when it is answered, schedules the next at once; after a
failure, after 1 s. The host holds each poll up to `aliveHoldSeconds`. A refusal 409 says the
load's heartbeat lapsed: the load says hello again and starts the chain anew. A refusal 403
ends the chain.

**The hello, again.** The hello of S4-5 stays the first act of `session.start`. One change: a
hello whose fetch threw is tried again from a timer of the same kind, after 1 s up to 10 s,
until it is answered; a refusal other than 503 is final for the load. So a hello lost on the way does not
leave a mod that runs and is never admitted.

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
admitted in this run (400); a known `endId` with another digest (409); a server closing (503).
Once the run begins to end, admission among its workers fails and the request is refused; the
harness has exited by then, so no mod is there to send. A handler admitted before the close
that has not captured its end captures nothing (the `closed` check above), so no capture runs
after the read clock closes (`core/mail/read_boundary.go`); one that began its `Publish` holds
the run's lease until it returns.

**A run that no longer owns its name.** `EndTurn` answers success with no effect when the name
has passed to a later run (`core/mail/turn_end.go`). The handler answers that with a receipt:
the end has no mailbox it could ever reach, and a retry would gain nothing. The decision stays
the core's; the adapter adds no check of its own.

## The live set

**What the mod proves.** An admitted hello makes TurnBoundary live, and Spaces on a run that
hosts them. Wake stays the inbox lane's, live from the service's start.

**Generations.** Each admitted hello opens a generation of the run's mod. The heartbeat's polls
belong to the generation of their load; a poll of an older load or of a lapsed generation is
answered 409 at once. The server stamps each poll's arrival. A lapse is no poll arriving within
`aliveLapseSeconds` of the generation's last arrival, the hello counting as the first; the
watch is one timer per generation, and a timer that fires for a generation no longer current
does nothing. On a lapse the generation ends, and the run reports its whole set without what
the mod proved: Wake alone. A later admitted hello opens a new generation and reports the set
with them again. Every report carries its generation, and `modRun` drops a report older than
the last it reported, so a timer or a poll of an earlier generation never changes the set of a
later one. The watch stops with the server's close; a lapse that fires after it reports
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
hello and gone with the load. On the host the arrival stamp and the lapse watch are one piece,
`modhost/alive.go`, which S4-7 feeds from `events` polls as well; nothing of `events` is built
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
  before its receipt; the session's task stays owed and main sees it unanswered.

## The rules

What S4-6 brings into `docs/rules/`, each clause in the group it belongs to; a clause no test
holds yet is stated in its rule's text as a known limit, dated, owned by S4-6's build, since
the rules test refuses a `Gap:` line and a named test that does not exist. The build moves
each into its rule's `Tests:` list with the test that holds it.

- **O1** (`turns.md`): the limit of an unheld interim end on Claude Code.
- **O2** (`turns.md`): the reasons mapped from `turn.complete`, a subagent's turn not sent.
- **E7** (`effects.md`): a retried end reuses its first capture; one with the same id and
  another digest is refused.
- **The live set** (`host.md`): a boundary proven after the start is not awaited before it;
  availability waits for it within its bound; a lapse withdraws what the mod proved, a new hello
  restores it, an older generation changes nothing; an end after a lapse is taken.
- **The capabilities** (`adapter-api.md`): a reporting role launches on Claude Code; a known
  cause refuses it; a launch through `--command` that does not exec is never admitted.
- **SP4** (`space.md`): the peer checked before each effect of a start, an end and a poll.

## The build

Three parts, each closed by its acceptance; the steps' table carries them
([stage4-space-steps.md](stage4-space-steps.md#the-build-steps)).

**Part A — the turns.** The schema's endpoints and bounds; `modhost/turns.go`, the dispatch
table and `modhost/hello.go`; `modRun` as `TurnBoundary`; the offer and `AfterStart`; the known
causes and the launch's refusal; availability waiting for a boundary proven after the start;
the mod's `turn.start`, `turn.complete`, outbox, retry chain and cut. Tests it adds:

- `modhost`: each reason maps to its end reason; an end without an admitted hello is refused;
  a failed `Publish` answers 503 and the retry under the same `endId` hands the core the same
  capture — boundary, `Started`, `Ended`, event; a lost receipt and a retry after later reads
  and marks answer only what the first capture saw; two copies of one end at once capture once;
  a known `endId` with another digest is refused; a request delayed across `Close` captures
  nothing; a begun `Publish` holds the lease; a body past `maxEndBodyBytes` is refused 413; a
  start records no timed start.
- `adapter/claude`: the offer reports turns and does not hold; the known causes refuse a
  reporting role and leave a silent role with a note; the set is Wake until the hello and Wake
  with TurnBoundary after it, and with Spaces on main.
- `host`: a reporting role on a boundary proven after the start starts its harness, and a task
  to it fails until the hello; availability waits for the boundary within its bound; the
  fixture's wait before the start is unchanged, with its negative control.
- `cli`: the refusal of a reporting role still covered by a test harness that reports none, now
  that no catalog harness lacks a boundary.
- The mod's own tests in `claude plugin test`: a subagent's `turn.complete` is not sent; the
  first attempt is awaited; a 503 is retried and a 409 is dropped; a long answer is cut with
  its line; the outbox is written to the store and sent again by a new load.

**Part B — the heartbeat.** `mod/alive`, generations, the lapse watch and the withdrawal. Tests
it adds: a lapse withdraws TurnBoundary, and Spaces on main, leaving Wake; a new hello restores
them; a poll of a lapsed generation or an older load is refused 409 and changes nothing; a lapse
timer of an earlier generation does nothing after a new hello; an end after a lapse is taken
and makes nothing live; a lapse after the close reports nothing; the mod's chain says hello again
on 409.

**Part C — live**, on the installed Claude Code, by main, in a separate `REWAKE_DIR`: main sends
a task to a worker started by hand with `rewake claude --write` and reads its report; the same
through a wrapper script given with `--command` that ends in `exec`, and one that does not,
whose worker refuses the task; a worker whose mod stops polling — its module in the run's directory
replaced by one that registers nothing, which an interactive session's hot reload of a
`--plugin-dir` loads (the skill, `official/troubleshoot.md`) — has its boundary withdrawn within
30 s and a task to it refused, and has it back once the module is restored; an interrupted
turn reported as stopped; a subagent's turn not reported; whether
the engine runs `turn.complete`'s hooks before the next turn starts; that the heartbeat's chain
keeps polling between turns and through a reload.

The workflow suite's columns stay the fixture's alone (`test/workflow/column_test.go`), and the
confirmation scenarios keep the fixture, which holds an end; Claude Code's turns are held by
the tests above and by part C.
