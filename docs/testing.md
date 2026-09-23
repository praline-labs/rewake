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
| Workflow suite (**F**) | a built rewake end to end against a fixture of each harness, in two columns, with negative controls: some mutate the product, the others change the fixture's world | that the shared service code delivers, groups, steers and reports as each scenario claims, and that each claim can fail | that the real harness parses, renders or behaves as its fixture does | about two minutes; no network; under `REWAKE_WORKFLOW=1` the schema case also runs a real Codex (next row) |
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

**`REWAKE_CODEX_VERSION`** takes an exact version, `latest` or `installed`, and makes
the schema case use that Codex, fetched once into the harness cache before any case
starts and run in a container. The fetch gets half of `-timeout`, at most ten minutes,
which is why the documented command raises `-timeout` to thirty; below about five
minutes that half is not enough for a slow first download and the suite together.
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
toolchain's overlay, inside a started case, refusing an edit that does not match
exactly once; or a switch that changes the fixture's world. A mutant is preferred
wherever one can be built, because it shows the scenario catching a broken rewake
rather than a misbehaving peer. Of today's nineteen controls, ten are mutants —
batch-arrival's four; task-report's no-stop-hook, turn-ended-ignores-stop and
settles-nothing; mid-turn's wait-for-idle; claude-telemetry's tap-without-owner and
uncounted-compaction — and nine are fixture switches: task-report's
wrong-report, read-fails, failure-before-report and early-exit; the three readiness
controls; mid-turn's late and failed-operation. A control names the observation it must
break; the crosswise check then runs every control's observations in every other
control's world and requires them to stand, so a control that breaks on somebody else's
change is caught. Each control answers in three values — broken, not broken, cannot
judge — and a record that is unreadable or empty is "cannot judge", never "not
broken".

**A fixture** is stricter than the harness it plays, never looser: a fixture that
accepts what the harness refuses lets a scenario pass on a product the harness would
reject. The Codex fixture builds every message through the constructors the schema case
reads (`turnReply`, `turnStartedEvent`, `threadStatusChangedEvent` and their
neighbours), so a message the scenarios send cannot escape the shape check.

**A harness column** is a `column` value with its capabilities and a fixture; the
scenarios do not change. What building the second one taught is in
[2026-09-22-fixture-claude-code.md](roadmap/2026-09-22-fixture-claude-code.md).

### Claude Code telemetry budgets

`claude-telemetry` runs on the Claude Code column only: the fixture plays a session's
hooks and status line through `/bin/sh -c` with the payloads seen live, including their
conversation fields, and the case reads the result back through the session's own
`rewake list`. It also times the commands, cold each time, because a telemetry hook
sits in front of every prompt. Its two controls are mutants; they have no crosswise run,
since both break observations of one session read from one listing.

Measured on the development machine, September 23, 2026, 40 runs each: a telemetry hook
(shell plus a cold `rewake observe` sending one datagram) median 5.4 ms, p95 6.7 ms; the
tap in front of a one-line `sed` status line added a median of 4.8 ms, p95 6.8 ms, to
that line's own 2.6 ms. The hooks run in the background (`"async": true`), so the agent
does not wait even for that. The case's budgets are a median of 20 ms and a p95 of 50 ms
for a hook, and a median of 20 ms added by the tap: several times the measurement, so
they catch a wait, a lock or a heavy start rather than a busy machine.

## Traps this suite has already paid for

- **Inherited session variables.** A test run from inside a session read the live state
  as its own — hence `env -u` everywhere
  ([traps.md](traps.md#a-test-that-inherited-the-sessions-variables-declared-its-own-session-foreign)).
- **A fixture softer than the harness.** One review round found two high findings on a
  product with every test green, because the fixture accepted what the real server
  refuses ([research-launch.md](research-launch.md)).
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
