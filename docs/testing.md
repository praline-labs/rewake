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
| The five checks | formatting, vet, two linters, `go test -race -shuffle=on ./...` | unit invariants: parsing, publishing races, liveness, inbox order and expiry, the context endpoint's framing, the map of `docs/`; the suite's own classifier and summarizer | anything end to end: every workflow scenario skips itself | about half an hour at `-p 4` on an idle machine, nearly all of it the three `-race` test runs; no network, no harness, no container |
| Workflow suite (**F**) | a built rewake end to end against a fixture of each harness, in two columns, with negative controls: some mutate the product, the others change the fixture's world | that the shared service code delivers, groups, steers and reports as each scenario claims, and that each claim can fail | that the real harness parses, renders or behaves as its fixture does | about three minutes — 3m06s and 3m10s on September 25, 2026, 102 cases six at a time, against 19m22s one after another the same evening; 2m23s eight at a time. The longest cases are `claude-steered`'s controls, about 30 s each. It fits in `go test`'s default ten minutes, which the documented commands name; no network |

Two tiers are planned and not built. **P**, a real harness of a named version against a
local responder instead of a model provider, would catch a change of behaviour that a
schema cannot see; the fetch and the container it would need left with
`tools/harnesscache` in S8 and are kept in
[archive/1.x/codex](../archive/1.x/codex/README.md). **M**, a real model through a real
harness, paid, would show that a model actually reads its mail and acts on it; it runs on
the cheapest model only, as [check-runner.md](check-runner.md) requires. The full ladder,
including the owner's own eye on a terminal, is in
[check-runner.md](check-runner.md#evidence-tiers-and-matrix).

### The fault build inside the five checks

The neutral rig's tests (`test/toolrig`) build rewake themselves, with the `rewakefault`
tag, while the five checks run. A binary of that build reads `REWAKE_FAULT`
(`internal/state/fault_build.go`) and logs, ends, fails or holds its process at a durable
step, or leaves behind a process that keeps its output open past a kill, so the fault
tests take their cases from the logged steps of clean runs rather than from a list kept
by hand ([mail-bridge-checks.md](mail-bridge-checks.md#how-the-checks-are-built)). A
release build never reads the variable.

### Three columns, three meanings

Since S8, October 8, 2026, there are two: the Codex column left with its adapter. Shared
scenarios run across the columns; adapter-specific scenarios and controls run on the
columns they exercise. The columns: a fixture that plays a Claude Code session, and the
fixture harness — a harness of its own, built
only under the `rewakefixture` tag, which the suite's rewake is built with
([stage3-fixture.md](v2/stage3-fixture.md)). **The fixture column is the regression
gate** since S6 ([stage3-fixture.md](v2/stage3-fixture.md#the-gate-across-the-steps)):
a red there blocks, and an absent capability there is red too, because a gate that
stopped checking must not look green. It proves the core through the adapter contract,
and its program can withhold each capability. **Claude Code is a search
column** until its column goes: a red there is a finding to investigate, and an
absent capability is reported as `unsupported`, by name. The gate is a field of the
column (`gate` in `test/workflow/column_test.go`), set on exactly one; the gate declares
every registered capability but those its exception table names, each with its reason
and the step that retires it — since S8 none: conversation selection, Codex's own, left
with it. The summary
names the column of every result and promotes none; ending the asymmetry is the owner's
decision, recorded in [harness-features.md](harness-features.md).

## Running it

The commands live in one place, the [Checks section of AGENTS.md](../AGENTS.md#checks),
which every agent session loads and whose five checks are the condition for the commit
that lands a step; the commits before it need only build.
What follows is what their variables and flags do, and when each run is the right one.

**The five checks** clear `REWAKE_SESSION`, `REWAKE_EPOCH`, `REWAKE_DIR` and
`REWAKE_ROOM`, because a test that inherits them reads the live session's state as its
own. They start no harness, no container and no network request, whatever else the
environment says: every real-harness step is behind the suite switch. Run them on the
commit that lands a step, and once before a fix after review is accepted.

**The workflow suite** is switched on by `REWAKE_WORKFLOW=1`; without it every scenario
skips itself. Run it through `tools/checksummary` to read a result, which prints a few
lines and writes the rest to a file; run it with `go test -v` to watch it, and keep the
`-v`: `go test` prints nothing a passing package printed, so without it the line naming
how many scenarios ran is invisible. `-count=1` keeps a cached pass from standing in for
a run. Run it after any change to delivery, reading, reporting or a fixture.

**Cases run side by side**, six at a time unless `-parallel` says otherwise, and the
suite's binary runs seven of the product's waits shorter than a release.
[testing-pool.md](testing-pool.md) says how to choose the width, how a case's cleanup
leaves its neighbours alone, and which values the suite does not run at their real
length.

**`REWAKE_WORKFLOW_CROSS=1`** with `-run Crosswise` turns on the crosswise checks: each
control's observations are run again in every other control's world — under the other
product mutants and under the other fixture switches — and each must stand, so a control
that breaks on somebody else's change is caught. Each pair is a case of its own in the
pool, so its 75 cases take about a minute and a quarter — 1m17s on September 25, 2026,
against 7m02s one pair after another. Run it after adding or changing a control.

`REWAKE_WORKFLOW_SELFCHECK` is not for people: the suite sets it on a child of itself
to prove that a scenario missing an observation turns the run red.

### The stand-in API

Until S8, October 8, 2026, `tools/standin` answered for the model API a Claude Code
harness talks to, replying with a call of the 1.x mail tool's MCP name, so a live check of
that tool could run in a scratch configuration with no login. It served the live checks of
stage 3 ([mail-bridge-live.md](mail-bridge-live.md)). The fixes of S8's review moved it to
the 1.x archive with the MCP contract it answered
([archive/1.x/codex](../archive/1.x/codex/README.md)); no product launch offers that tool.
Stage 4 rebuilds a stand-in for the mod's live checks, answering with a call of the tool
contract that stage defines.

## Checking a new harness version before updating

Until S8, October 8, 2026, the suite checked a Codex version before an update: it fetched
the named version into a harness cache, generated its protocol schema in a container and
held the fixture's messages to it. That check, `REWAKE_CODEX_VERSION` and
`tools/harnesscache` left with the Codex adapter and are kept in
[archive/1.x/codex](../archive/1.x/codex/README.md). Claude Code's launch reads the
installed version before the claim and refuses one below its minimum; a new version's
behaviour is checked live, as [research.md](research.md) records. Stage 5 brings a
version check back with the Codex adapter.

## Reading a result

The summarizer prints, and its exit code says, 0 green, 1 red, 2 a wrong call, 3 a
stream it could not read (`go run` reports every non-zero exit as 1 and prints the real
one; build the binary to keep them apart):

```
workflow  24 scenarios, 36 cases: 33 pass, 3 unsupported   2m0.7s
against   scenarios against the suite's own harness programs in every column
          unsupported  mid-turn/claude  observes-mid-turn-arrival
FAIL  batch-arrival/claude   fail
      fail        "the recipient can receive mail before the letters leave"
        worker-claude never started listening
      evidence  /tmp/rewake-case-270015062
      stderr    worker.stderr: claude-shim: a delivery carried more than one line
summary   .rewake-checks/<time>/summary.json
```

The first line counts scenarios and cases by outcome. `against` says what the run was
checked against. An `unsupported` line names the case, the column and the capability it
lacks. A `FAIL` block names the case and column, each observation that did not pass
with its detail, one evidence directory, and a `stderr` line for each session that
printed to its standard error, at most four: the file and the last line in it.
`run FAIL` is a failure of the run itself, such as a binary that could not be built; `engine FAIL` means `go test` failed with no case to explain it, and the
failed tests are listed. `summary.json` holds all of it,
every case and every observation, under `.rewake-checks/`, which git ignores.

A red case keeps its directory: `/tmp/rewake-case-*` with the private HOME, state
directory and shim records of that case, or `/tmp/rewake-mutant-*` with `failure.txt`
and `build.log` when a mutant could not be built. A green case removes its own. Read the
evidence before deleting it. Each session the case started wrote its standard error to
`<name>.stderr` in that HOME — the wrapper's refusals, the fixture's complaints — and a
red case keeps those files cut to 64 KiB, their first and last halves, so a session
that looped on an error does not fill the disk. They were added on September 27, 2026,
after a launch that exited 1 said why only in a rerun with the capture added by hand.

**The outcomes**, in the order the classifier ranks a case. `fail`: an observation was
made and contradicted the claim, or cleanup failed, or the deadline expired — it wins
over everything below. `incomplete`: the case ran and a declared observation was never
made, which includes one recorded as `skip` or `not-run`; never green, because "we never
looked" is not "we looked and it was right". `unsupported`: the column lacks a
capability an observation needs; acceptable on a search column, red on the gate,
where it would mean the regression check had quietly stopped checking. `pass`: every
declared observation was made and held. `skip` and `not-run` appear on observations,
never as a case's outcome: a declared observation nobody made is listed as `not-run`,
and a case that has not finished has no verdict yet.

## Extending the suite

**A scenario** declares its observations before it starts (`Start` with a `Spec`), so
an observation the code forgot to make is `incomplete`, not silently absent. It runs
through `runInColumns`, and anything a column cannot show goes through
`col.unsupported` with a capability registered by `capability(...)` — per observation,
not per case. The gate column must declare every registered capability but those in
its exception table (`column_gate_test.go`), and an exception that no longer matches
fails too. A control runs on the gate column (`gateColumn()`) unless it breaks a path
only another column has. Waits are anchored on a condition the case can observe (`Await`, `waitFor`),
never on a sleep: a sleep makes a slow machine look like a defect and a lost wakeup look
like slowness.

**A control** proves the scenario can fail. It breaks one claim, in one of two ways: a
product mutant, built by `buildMutant` in `mutant_test.go` with its edits through the
toolchain's overlay, inside a started case, refusing an edit that does not match exactly
once; or a switch that changes the fixture's world. A mutant is preferred wherever one
can be built, because it shows the scenario catching a broken rewake rather than a
misbehaving peer, and most controls are mutants. Which control belongs to which
scenario is the table in [testing-cases.md](testing-cases.md#every-control), which
`docs/controls_test.go` keeps equal to the suite; a count written here would go stale
with the next mutant, as the list that stood here did. A control names the
observation it must break; the crosswise check then runs every control's observations in
every other control's world and requires them to stand, so a control that breaks on
somebody else's change is caught. Each control answers in three values — broken, not
broken, cannot judge — and a record that is unreadable or empty is "cannot judge", never
"not broken".

**A fixture** is stricter than the harness it plays, never looser: a fixture that
accepts what the harness refuses lets a scenario pass on a product the harness would
reject.

**A harness column** is a `column` value with its capabilities and a fixture; the
scenarios do not change. The fixture harness's program is `test/workflow/fixtureshim_test.go`
and `fixtureshim_turn_test.go`; its adapter, with its own tests of the readiness
exchange, each switch, the versions, L1, L2 and E3, is `internal/harness/fixture`. What building the second one taught is in
[2026-09-22-fixture-claude-code.md](roadmap/2026-09-22-fixture-claude-code.md).

### The cases

What each case claims, with its controls, is in [testing-cases.md](testing-cases.md):
the Claude Code telemetry budgets, launching and reporting, the worktrees, a directory
granted with a task, actions on a sent message, conversations and the inbound gate, and a
pointer to the plugin's cases.

## Traps this suite has already paid for

- **A shim rewritten while it runs.** Every session a case starts installed the shim
  scripts again, writing them in place, while an earlier session of the same case was
  executing them: an exec of a file open for writing fails with ETXTBSY. Seen once while
  eight copies of the suite ran at a time; the scripts are now written beside and renamed over
  (September 25, 2026).
- **Timers set at the edge of an idle machine.** Shortening the widened-window mutant's
  collection interval from ten seconds to five broke its control in both columns: the
  first two sends wait out `send`'s five-second wait for a status before the third letter
  leaves, so the widened window has to outlast that, not only `laterSendDelay`. And with
  six cases at a time, the sends writing batch-arrival's heads-ups started over a second
  late: at 1.5 s of quiet and a 2 s cap, one crosswise run in four had the third heads-up
  announced alone. The suite's window is 2 s and 3 s since, which cost about 25 s of a
  full run. Every shortened wait now carries, beside its value, what it has to outlast
  (September 25, 2026).
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
