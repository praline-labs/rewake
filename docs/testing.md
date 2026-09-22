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
| Workflow suite (**F**) | a built rewake end to end against a fixture of each harness, in two columns, with controls that mutate the product | that the shared service code delivers, groups, steers and reports as each scenario claims, and that each claim can fail | that the real harness parses, renders or behaves as its fixture does | about two minutes; no network; under `REWAKE_WORKFLOW=1` the schema case also runs a real Codex (next row) |
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

The five checks, the condition for every commit:

```bash
gofumpt -l $(go list -f '{{.Dir}}' ./...)
go vet ./...
staticcheck ./...
golangci-lint run ./...
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on ./...
```

The `REWAKE_*` variables are cleared because a test that inherits them reads the live
session's state as its own. These checks start no harness, no container and no network
request, whatever else the environment says: every real-harness step is behind the
suite switch.

The workflow suite, through the summarizer, which prints a few lines and writes the
whole result to a file:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 go run ./tools/checksummary \
  -- go test -count=1 -json ./test/workflow/...
```

To watch it instead, the same run with `-v` in place of the summarizer:
`REWAKE_WORKFLOW=1 go test -count=1 -v ./test/workflow/...` (with the same `env -u`).
`-v` matters: `go test` prints nothing a passing package printed, so without it the line
naming how many scenarios ran is invisible.

The schema from a named Codex version — an exact version, `latest` or `installed` —
with room for a first download:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 REWAKE_CODEX_VERSION=0.156.0 go run ./tools/checksummary \
  -- go test -count=1 -timeout 30m -json ./test/workflow/...
```

The version is fetched once, before any case starts, within half of `-timeout` and at
most ten minutes; below a `-timeout` of about five minutes that half is not enough for a
slow first download and the suite together. Docker is needed only when a version is
named; without it the run is red with the reason.

The crosswise check runs every control against every other control's mutant and
requires each observation to stand; it multiplies the run, about three and a half
minutes, so it has its own switch:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 REWAKE_WORKFLOW_CROSS=1 go run ./tools/checksummary \
  -- go test -count=1 -json -run Crosswise ./test/workflow/...
```

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
go run ./tools/harnesscache remove codex 0.155.1
```

`run` starts the version in a fresh container: read-only root and harness, a private
HOME on tmpfs, no capabilities, no network unless `--network` names one, and at most one
writable directory, given with `--write` and mounted at `/out`. Codex and Claude Code
both work; `--help` lists flags and exit codes.

## Checking a new harness version before updating

1. Run the suite with `REWAKE_CODEX_VERSION=<version>` as above. The first run downloads
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
FAIL  batch-arrival/codex   fail
      fail        "the recipient can receive mail before the letters leave"
        worker-codex never started listening
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

**The outcomes.** `pass`: every declared observation was made and held. `fail`: one was
made and contradicted the claim. `incomplete`: the case ran and a declared observation
was never made — never green, because "we never looked" is not "we looked and it was
right". `not-run`: nothing was attempted. `skip`: a deliberate selection with a reason.
`unsupported`: the column lacks a capability an observation needs; acceptable on the
search column, red on the gate, where it would mean the regression check had quietly
stopped checking.

## Extending the suite

**A scenario** declares its observations before it starts (`Start` with a `Spec`), so
an observation the code forgot to make is `incomplete`, not silently absent. It runs
through `runInColumns`, and anything a column cannot show goes through
`col.unsupported` with a capability registered by `capability(...)` — per observation,
not per case. The gate column must declare every registered capability; a test fails
otherwise. Waits are anchored on a condition the case can observe (`Await`, `waitFor`),
never on a sleep: a sleep makes a slow machine look like a defect and a lost wakeup look
like slowness.

**A control** proves the scenario can fail. It mutates the product, not the fixture:
`buildMutant` in `mutant_test.go` builds rewake with one edit through the toolchain's
overlay, inside a started case, and refuses an edit that does not match exactly once. A
control names the observation it must break; the crosswise check then runs every
control against every other mutant and requires the named observations to stand, so a
control that breaks on somebody else's mutation is caught. Each control answers in three
values — broken, not broken, cannot judge — and a record that is unreadable or empty is
"cannot judge", never "not broken".

**A fixture** is stricter than the harness it plays, never looser: a fixture that
accepts what the harness refuses lets a scenario pass on a product the harness would
reject. The Codex fixture builds every message through the constructors the schema case
reads (`turnReply`, `turnStartedEvent`, `threadStatusChangedEvent` and their
neighbours), so a message the scenarios send cannot escape the shape check.

**A harness column** is a `column` value with its capabilities and a fixture; the
scenarios do not change. What building the second one taught is in
[2026-09-22-fixture-claude-code.md](roadmap/2026-09-22-fixture-claude-code.md).

## Traps this suite has already paid for

- **Inherited session variables.** A test run from inside a session read the live state
  as its own — hence `env -u` everywhere
  ([traps.md](traps.md#a-test-that-inherited-the-sessions-variables-declared-its-own-session-foreign)).
- **A fixture softer than the harness.** One review round found two high findings on a
  product with every test green, because the fixture accepted what the real server
  refuses ([research-launch.md](research-launch.md)).
- **A control that passed for the wrong reason.** The replay control stayed green on the
  socket column until replays were found by arithmetic rather than by id; an early-exit
  control passed on timing alone until it waited for the departure itself
  ([2026-09-22-fixture-claude-code.md](roadmap/2026-09-22-fixture-claude-code.md)).
- **A mutant built before its case.** A failed build published no record, and the only
  evidence was deleted on the way out; mutants now build inside a started case.
- **The gate losing an observation.** One line removed from the gate's table turned an
  observation `unsupported` and the run stayed green; unsupported is now red there.
- **A shared deadline.** Twelve crosswise pairs in one case exhausted its clock and the
  last pair was blamed; each pair is its own subtest.
- **The five checks reaching outside.** A child of the suite inherited a named version
  and contacted the registry during an ordinary `go test ./...`
  ([2026-09-23-harness-versions.md](roadmap/2026-09-23-harness-versions.md)).

How each scenario was built, and what it does not prove, is in its own record:
[task-report](roadmap/2026-09-21-scenario-task-report.md),
[batch-arrival](roadmap/2026-09-22-scenario-batch-arrival.md),
[mid-turn](roadmap/2026-09-22-scenario-mid-turn.md), and the summarizer in
[2026-09-22-suite-summarizer.md](roadmap/2026-09-22-suite-summarizer.md).
