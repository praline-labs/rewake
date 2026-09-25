# Testing rewake

The one place that says how this project is tested: what each tier runs, what it proves
and what it does not, how to run it, how to read what comes out, how to check a new
harness version before updating, and how to extend the suite. It describes the code as
it is. For depth it links to the documents where each decision was argued:
[check-runner.md](check-runner.md) for the evidence contract,
[check-runner-proposal.md](check-runner-proposal.md) for why the suite has its shape,
[check-runner-scenarios.md](check-runner-scenarios.md) for each scenario's invariant
and controls, and the roadmap records named below for how each piece was built.

## The tiers

| Tier | Runs | Proves | Does not prove | Costs |
| --- | --- | --- | --- | --- |
| The five checks | formatting, vet, two linters, `go test -race -shuffle=on ./...` | unit invariants: parsing, publishing races, liveness, inbox order and expiry, the owned server's framing on a fake socket, the map of `docs/`; the suite's own classifier and summarizer | anything end to end: every workflow scenario skips itself | about a minute; no network, no harness, no container |
| Workflow suite (**F**) | a built rewake end to end against a fixture of each harness, in two columns, with negative controls: some mutate the product, the others change the fixture's world | that the shared service code delivers, groups, steers and reports as each scenario claims, and that each claim can fail | that the real harness parses, renders or behaves as its fixture does | about twenty minutes — 19m28s on September 25, 2026, 102 cases one after another, eight of the minutes `claude-steered` and its controls — so a full run needs a `-timeout` past `go test`'s default ten minutes; no network; under `REWAKE_WORKFLOW=1` the schema case also runs a real Codex (next row) |
| Schema of a Codex version | the installed Codex, or a named version fetched into a cache and run in a container, generating its protocol schema; every message the fixture sends is checked against it | that the fixture speaks the shape that version accepts: no missing required field, no field it does not have, no delivery it refuses and the fixture accepts | behaviour: order of events, readiness, reactions to a refusal — a schema has none of that | seconds from the cache; a first download is 150 MB and about half a minute |

Two tiers are planned and not built. **P**, a real harness of a named version against a
local responder instead of a model provider, would catch a change of behaviour that a
schema cannot see; the fetch and the container it needs exist already. **M**, a real
model through a real harness, paid, would show that a model actually reads its mail and
acts on it; it runs on the cheapest model only, as [check-runner.md](check-runner.md)
requires. The full ladder, including the owner's own eye on a terminal, is in
[check-runner.md](check-runner.md#evidence-tiers-and-matrix).

### Two columns, two meanings

Every scenario runs twice: against a fixture that plays a Codex app-server, and against
one that plays a Claude Code session. **Codex is the regression gate**: a red there
blocks, and an absent capability there is red too, because a gate that stopped checking
must not look green. **Claude Code is the search column**: a red there is a finding to
investigate, and an absent capability is reported as `unsupported`, by name. The summary
names the column of every result and promotes neither; ending the asymmetry is the
owner's decision, recorded in [harness-features.md](harness-features.md).

## Running it

The commands live in one place, the [Checks section of AGENTS.md](../AGENTS.md#checks),
which every agent session loads and whose five checks are the condition for a commit.
What follows is what their variables and flags do, and when each run is the right one.

**The five checks** clear `REWAKE_SESSION`, `REWAKE_EPOCH`, `REWAKE_DIR` and
`REWAKE_ROOM`, because a test that inherits them reads the live session's state as its
own. They start no harness, no container and no network request, whatever else the
environment says: every real-harness step is behind the suite switch. Run them before
every commit.

**The workflow suite** is switched on by `REWAKE_WORKFLOW=1`; without it every scenario
skips itself. Run it through `tools/checksummary` to read a result, which prints a few
lines and writes the rest to a file; run it with `go test -v` to watch it, and keep the
`-v`: `go test` prints nothing a passing package printed, so without it the line naming
how many scenarios ran is invisible. `-count=1` keeps a cached pass from standing in for
a run. Run it after any change to delivery, reading, reporting or a fixture.

**The suite's binary** serves a shorter coalescing window than a release: 1.5 seconds of
quiet and a 2-second cap instead of three and four, set at build through
`-ldflags -X` on `internal/inbox`'s `builtQuiet` and `builtCap`, for the binary under
test and every mutant alike (`windowFlags` in `test/workflow/suite_test.go`). A heads-up
or a report waits for company in most scenarios, and at the real window that wait added
about nine minutes to a full run — measured while another session's cases ran — while
proving nothing a shorter one does not. The real
values are held by the unit tests, including one that keeps the cap a second inside
`send`'s five-second wait. A scenario that times something against the window takes
`suiteQuiet` or `suiteCap` rather than a number of its own.

**`REWAKE_CODEX_VERSION`** takes an exact version, `latest` or `installed`, and makes
the schema case use that Codex, fetched once into the harness cache before any case
starts and run in a container. The fetch gets half of `-timeout`, at most ten minutes,
which is why a first run of a new version raises `-timeout` to forty: the suite alone
takes about twenty minutes, and what is left after a slow first download has to hold it.
Docker is needed only when a version is named; without it the run is red with the
reason. Run it before updating Codex, as described below.

**`REWAKE_WORKFLOW_CROSS=1`** with `-run Crosswise` turns on the crosswise checks: each
control's observations are run again in every other control's world — under the other
product mutants and under the other fixture switches — and each must stand, so a control
that breaks on somebody else's change is caught. It multiplies the run to about three
and a half minutes. Run it after adding or changing a control.

`REWAKE_WORKFLOW_SELFCHECK` is not for people: the suite sets it on a child of itself
to prove that a scenario missing an observation turns the run red.

### The harness cache

`tools/harnesscache` fetches a harness version from the npm registry once, checks it
against the registry's sha512 before unpacking it, and keeps it in
`~/.cache/rewake/harness/<harness>/<version>/` (`REWAKE_HARNESS_CACHE` moves it). The
suite reads the same cache. Nothing is ever removed automatically.

```bash
go run ./tools/harnesscache fetch codex 0.156.0     # prints the executable's path
go run ./tools/harnesscache run codex 0.156.0 -- --version
go run ./tools/harnesscache list
go run ./tools/harnesscache remove codex <version>
```

`run` starts the version in a fresh container: read-only root and harness, a private
HOME on tmpfs, no capabilities, no network unless `--network` names one, and at most one
writable directory, given with `--write` and mounted at `/out`. Codex and Claude Code
both work; `--help` lists flags and exit codes.

## Checking a new harness version before updating

1. Run the suite with `REWAKE_CODEX_VERSION=<version>`, the command in
   [AGENTS.md](../AGENTS.md#checks). The first run downloads
   it; the `against` line says `downloaded this run`, and every later run says
   `from the cache without a download`.
2. **Green** means every message the fixture sends matches the schema that version
   generates, and the fixture accepts no delivery that schema refuses. It says nothing
   about behaviour — the same messages in a different order, a different moment of
   readiness, a different answer to a refusal would all still be green.
3. **Red on the schema case** names the message and the field: the version requires a
   field the fixture leaves out, dropped one it sends, or changed a type. Look at what
   changed in the schema before touching the adapter; the shim is fixed to match the
   real shape, never loosened to pass.
4. **Red with `run FAIL`** is not a verdict on the version: it could not be resolved,
   fetched or run, and the line says why.
5. To see what changed beyond what the fixture uses, generate both schemas with
   `harnesscache run codex <version> --write <dir> -- app-server generate-json-schema
   --experimental --out /out` and compare them. The record of the first such comparison,
   0.155.1 against 0.156.0, is in
   [2026-09-23-harness-versions.md](roadmap/2026-09-23-harness-versions.md).

## Reading a result

The summarizer prints, and its exit code says, 0 green, 1 red, 2 a wrong call, 3 a
stream it could not read (`go run` reports every non-zero exit as 1 and prints the real
one; build the binary to keep them apart):

```
workflow  24 scenarios, 36 cases: 33 pass, 3 unsupported   2m0.7s
against   schema from codex 0.156.0 (…); scenarios against the fixture in both columns
          unsupported  mid-turn/claude  observes-mid-turn-arrival
FAIL  batch-arrival/claude   fail
      fail        "the recipient can receive mail before the letters leave"
        worker-claude never started listening
      evidence  /tmp/rewake-case-270015062
summary   .rewake-checks/<time>/summary.json
```

The first line counts scenarios and cases by outcome. `against` says what the run was
checked against. An `unsupported` line names the case, the column and the capability it
lacks. A `FAIL` block names the case and column, each observation that did not pass
with its detail, and one evidence directory. `run FAIL` is a failure of the run itself,
such as a version that could not be fetched; `engine FAIL` means `go test` failed with
no case to explain it, and the failed tests are listed. `summary.json` holds all of it,
every case and every observation, under `.rewake-checks/`, which git ignores.

A red case keeps its directory: `/tmp/rewake-case-*` with the private HOME, state
directory and shim records of that case, or `/tmp/rewake-mutant-*` with `failure.txt`
and `build.log` when a mutant could not be built. A green case removes its own. Read the
evidence before deleting it.

**The outcomes**, in the order the classifier ranks a case. `fail`: an observation was
made and contradicted the claim, or cleanup failed, or the deadline expired — it wins
over everything below. `incomplete`: the case ran and a declared observation was never
made, which includes one recorded as `skip` or `not-run`; never green, because "we never
looked" is not "we looked and it was right". `unsupported`: the column lacks a
capability an observation needs; acceptable on the search column, red on the gate,
where it would mean the regression check had quietly stopped checking. `pass`: every
declared observation was made and held. `skip` and `not-run` appear on observations,
never as a case's outcome: a declared observation nobody made is listed as `not-run`,
and a case that has not finished has no verdict yet.

## Extending the suite

**A scenario** declares its observations before it starts (`Start` with a `Spec`), so
an observation the code forgot to make is `incomplete`, not silently absent. It runs
through `runInColumns`, and anything a column cannot show goes through
`col.unsupported` with a capability registered by `capability(...)` — per observation,
not per case. The gate column must declare every registered capability; a test fails
otherwise. Waits are anchored on a condition the case can observe (`Await`, `waitFor`),
never on a sleep: a sleep makes a slow machine look like a defect and a lost wakeup look
like slowness.

**A control** proves the scenario can fail. It breaks one claim, in one of two ways: a
product mutant, built by `buildMutant` in `mutant_test.go` with one edit through the
toolchain's overlay, inside a started case, refusing an edit that does not match exactly
once; or a switch that changes the fixture's world. A mutant is preferred wherever one
can be built, because it shows the scenario catching a broken rewake rather than a
misbehaving peer. Of today's fifty-seven controls, forty-eight are mutants — batch-arrival's
five; task-report's no-stop-hook, turn-ended-ignores-stop and settles-nothing;
mid-turn's wait-for-idle; claude-telemetry's tap-without-owner, uncounted-compaction,
silent-compaction and model-window; pending-report's pending-ignored and
pending-settles; claude-inbound's ungated, gate-on-session-start, held-as-delivered,
expiry-unannounced, refusal-as-delivered and late-word-dropped; owed-reread's
owed-empty; thread-changed's delivery-unpinned, stop-thread-ignored and
thread-always-changed; awaited-view's awaited-interim-ignored and awaited-never-settled;
claude-interrupted's interrupt-unpublished, every-end-stopped, plugin-not-passed and
stop-not-heard; claude-steered's compact-not-run, in-turn-unmapped, interrupter-unnamed,
line-repeated, idle-interrupt-done, silent-not-answering, any-role-steers,
own-compaction-announced, compaction-uncounted, asker-untold and stop-by-a-person;
codex-steered's busy-unchecked, codex-asker-untold, codex-interrupter-unnamed,
codex-idle-interrupt-done and focus-taken; stopped-routing's stopped-to-main — and nine are fixture switches:
task-report's wrong-report, read-fails, failure-before-report and early-exit; the three
readiness controls; mid-turn's late and failed-operation. A control names the
observation it must break; the crosswise check then runs every control's observations in
every other control's world and requires them to stand, so a control that breaks on
somebody else's change is caught. Each control answers in three values — broken, not
broken, cannot judge — and a record that is unreadable or empty is "cannot judge", never
"not broken".

**A fixture** is stricter than the harness it plays, never looser: a fixture that
accepts what the harness refuses lets a scenario pass on a product the harness would
reject. The Codex fixture builds every message through the constructors the schema case
reads (`turnReply`, `turnStartedEvent`, `threadStatusChangedEvent` and their
neighbours), so a message the scenarios send cannot escape the shape check.

**A harness column** is a `column` value with its capabilities and a fixture; the
scenarios do not change. What building the second one taught is in
[2026-09-22-fixture-claude-code.md](roadmap/2026-09-22-fixture-claude-code.md).

### Claude Code telemetry budgets

`claude-telemetry` runs on the Claude Code column only, with a main and a worker: the
fixture plays the worker's hooks and status line through `/bin/sh -c` with the payloads
seen live, including their conversation fields, and the case reads the result from the
main's side — its `rewake list`, its header on a message from the worker, and the
compaction notice its wrapper sends. The worker tells the main only once its wrapper has
published it idle — the header is drawn from that publication, a quarter second apart,
and a main whose first notice waited for its status line once read the worker's message
inside that gap. A listing the main's fixture could not finish in five seconds is
recorded and fails the observations that needed it. The worker runs rewake's plugin
under node with a 150K `CLAUDE_CODE_AUTO_COMPACT_WINDOW` in its settings, under the
status line's 200K, so both must show 33% of 150K; without node it is unjudged. Accepted
September 24, 2026: no case plays a plugin-less session end to end; its model window and
`interruptions unheard` header rest on unit tests and claude-interrupted's `unobserved`.
The case also times the commands, cold each time, because a telemetry hook sits in front
of every prompt. Its four controls are mutants, and each names every observation it must
break and requires all the others to hold, which stands in for a crosswise run: a mutant
that broke everything would not pass as the control of one thing.

Measured on the development machine, September 23, 2026, 40 runs each: a telemetry hook
(shell plus a cold `rewake observe` sending one datagram) median 5.4 ms, p95 6.7 ms, and
on UserPromptSubmit, which also records the turn's start on disk, median 5.1 ms, p95 5.9
ms against the same budget; the
tap in front of a one-line `sed` status line added a median of 4.8 ms, p95 6.8 ms, to
that line's own 2.6 ms. The hooks run in the background (`"async": true`), so the agent
does not wait even for that. The case's budgets are a median of 20 ms and a p95 of 50 ms
for a hook, and a median of 20 ms added by the tap: several times the measurement, so
they catch a wait, a lock or a heavy start rather than a busy machine.

`wrapped-launch` runs in both columns, twice each, in two rooms: a plain launch and one
with `--command ./<harness>-worker`, a stand-in wrapper that exports a marker and execs the
fixture. Every fixture process records its arguments and whether it saw the marker, and
the case requires the wrapped processes — one for Claude Code; for Codex the version
check, the app-server and the terminal — to get argument for argument what the plain
launch gets, all to see the marker, and none of the plain ones to. The Codex launches
carry `-C` into a directory holding another script of the same name, which must never
run. It has no control in the suite; with the relative path left relative, it went red on
three of its four observations (September 23, 2026).

`pending-report` runs in both columns with three sessions. A worker reads a task and,
in that turn, runs `rewake pending` before ending it; the sender must read a `pending`
message about the task first, and the worker's awaiting record must still be there.
A third session then sends the worker a task of its own, and that turn end — with no
mark — must be the `finished` report settling the first task. Its two mutants, each in
both columns, ignore the mark and let the interim turn end settle the task; like the
telemetry controls, each names what it must break and requires the rest to hold.

`owed-reread` runs in both columns with a main and a worker. The worker reads a
multi-line task and, in the same turn, runs `rewake inbox --owed` in both forms, as a
session re-reading its task after a compaction would. Both must print that task in full;
the machine form carries the id the worker read it under, and the text form opens with
`Rewake: owed a report for 1 message:` and the sender's header and leaves the id out.
The turn end must then report it once and
settle it: asking changed nothing. Its mutant, an `--owed` that finds nothing, must break
the first observation and hold the second.

`awaited-view` runs in both columns with a main and two workers. main runs its own
commands when the scenario asks (the fixture's request directory): it sends one task to
each worker. One reports at once; the other runs `rewake pending` in its turn, so its
task stays owed. main's `rewake inbox --awaited`, in both forms, must then list exactly
that second task, by id, as `pending` with the interim's text, under `to <worker>`, and
leave the first out. A note from main wakes the second worker, whose next turn end
reports; the view must then be `Rewake: nobody owes you a report.` Its two mutants run on
the Claude Code column only — the view reads files the same way whichever harness wrote
them: one blind to interim reports breaks the first observation alone, one that never
lets a task go breaks both.

`thread-changed` runs on the Claude Code column only, with two workers and a sender
for each. The fixture names one conversation, the session's own, in every hook and in its
status line, and on a switch plays `/clear` once its first task has arrived: a
SessionStart with source `clear` and a new `session_id`, which every later event then
carries. The report from that worker must carry `threadChanged` and still arrive once
and settle its task; the report from the worker that stayed where its task landed must
carry none. Its three mutants, like the telemetry controls, name what they break and
require the rest to hold: a wrapper that never pins the delivery and a turn end that
ignores the Stop hook's `session_id` must each lose the mark, and a comparison that
takes every known conversation for a change must mark the worker that stayed. Before
this case the fixture's status line named a conversation of its own while its hooks
named the session, which a tracker would have read as a `/clear` in every case.

`claude-inbound` runs on the Claude Code column only: Codex has no inbound gate, and its
column is unchanged. The fixture plays the gate as the binary does — it holds whatever
arrives in the first 600 ms after its socket listens, longer than the real two hundred so
a notice that does not wait is caught every time, and it answers held, released, expired
or refused lines with receipts on the reply socket, after checking that socket is in its
own directory and listened on by the process that wrote the line. It refuses a line that
does not ask for receipts, which the real one would merely not answer, and in one mode
reports a hold 900 ms late, past rewake's 300 ms wait for a first word, since nothing
bounds how soon the real one speaks under load. Five workers each get one task from a
sender of their own: one sent as soon as the socket appears, which must go out once the
session is up and not be held; one held and released, reported delivered after the hold
and then worked; one held until it expires, reported `held` with exit 3, its sender sent a
note that it never arrived, and the task failed, unread and not owed; one refused,
reported failed with exit 1 and never worked; one held late, whose task must end failed
and unworked with its sender told, whatever `send` said first. Its six mutants deliver
without waiting, wait for SessionStart instead of the status line, read a held receipt as
a delivery, settle an expiry without telling the sender, read a refusal as a delivery,
and drop a word that comes after the window; each names what it must break and requires
the rest to hold.

### The plugin's cases

The cases that run rewake's function-hooks plugin under node — `claude-interrupted`,
`stopped-routing` and `claude-steered` — and what the fixture's plugin host plays for
them are in [testing-plugin.md](testing-plugin.md), with `codex-steered`, the same
commands on the Codex column.

## Traps this suite has already paid for

- **A fixture session that outlives nothing.** A fixture session stops itself after 25
  seconds unless it serves a request directory, which keeps it up for 85. A worker the
  scenario asks nothing of, launched late in a long case, was gone when its last step
  came — and a step that expected a refusal took "no such session" for it, since both
  exit 2. When the coalescing window made heads-ups wait, three steered and interrupted
  cases crossed the line. Such a worker is launched with `staysUp`, and a finding that
  expects a refusal names its text, not only its exit code (September 25, 2026).
- **An anchor weaker than its judgement.** The consuming-overview control waited for one
  read attempt while the Codex column's judgement needs two, and the second comes at the
  next delivery: red 5 of 20 alone. It now anchors on both there, and was 0 of 20
  (September 23, 2026).
- **A record read between two of its writes.** The Claude Code fixture records a
  delivery and then the overview it read for it; batch-arrival's wait for a delivery
  once landed in between under a full run and failed the case in half a second. A wait
  now waits on through that gap; a judgement after its anchor still fails on it
  (September 23, 2026).
- **A zombie counted as a live session.** A fixture that exits while a hook it started
  is finishing leaves that hook's shell an orphan, adopted by the suite as subreaper and
  dead but uncollected; the check for a live process group counted it, and the case
  waited out its deadline for a session that had ended — two of eight parallel runs of
  pending-report. A zombie in the group is now collected and not counted (September 23,
  2026).
- **Inherited session variables.** A test run from inside a session read the live state
  as its own — hence `env -u` everywhere
  ([traps.md](traps.md#a-test-that-inherited-the-sessions-variables-declared-its-own-session-foreign)).
- **A fixture softer than the harness.** One review round found two high findings on a
  product with every test green, because the fixture accepted what the real server
  refuses ([research-launch.md](research-launch.md)). It happened again with the
  Claude Code plugin: the fixture's `$.process.run` took an options object the harness
  refuses, and the module that passed every case was silent live; the fixture now
  checks the call as the binary does
  ([2026-09-23-claude-plugin.md](roadmap/2026-09-23-claude-plugin.md)).
- **A control that passed for the wrong reason.** The replay control stayed green on the
  socket column until replays were found by arithmetic rather than by id
  ([2026-09-22-fixture-claude-code.md](roadmap/2026-09-22-fixture-claude-code.md)).
- **A mutant built before its case.** A failed build published no record, and the only
  evidence was deleted on the way out; mutants now build inside a started case.
- **The gate losing an observation.** One line removed from the gate's table turned an
  observation `unsupported` and the run stayed green; unsupported is now red there.
- **A shared deadline.** Twelve crosswise pairs in one case exhausted its clock and the
  last pair was blamed; each pair is its own subtest.
- **The five checks reaching outside.** The direct shape tests were not behind the suite
  switch, so every ordinary `go test ./...` ran the installed Codex with the owner's HOME,
  and with a version named it contacted the registry and started a container; they need
  the switch now, and the suite's self-check child no longer inherits a named version
  ([2026-09-23-harness-versions.md](roadmap/2026-09-23-harness-versions.md)).

How each scenario was built, and what it does not prove, is in its own record:
[task-report](roadmap/2026-09-21-scenario-task-report.md),
[batch-arrival](roadmap/2026-09-22-scenario-batch-arrival.md),
[mid-turn](roadmap/2026-09-22-scenario-mid-turn.md), and the summarizer in
[2026-09-22-suite-summarizer.md](roadmap/2026-09-22-suite-summarizer.md).
