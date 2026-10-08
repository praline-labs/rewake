# rewake

rewake lets coding-agent sessions on one machine pass messages to each other. Before
acting on any task in this repository, read `docs/project.md` in full — every section, not
skimmed. It is the project's goal: the rules, the code, the reviews and the briefs you
receive follow from it and assume you know it; a decision made without it is likely to
contradict the goal and be refused or redone. It is about two hundred lines, one read.

## Where to start a session

0. `docs/project.md` — first: the essence, the principles and what the tool does,
   each linked to its specification, and what to read next by kind of work.
1. `docs/flow.md` — the whole path in one read: a session starts, a message is
   sent, announced, read, and the report comes back.
2. `docs/roadmap/README.md` — what is done and what comes next. A milestone is closed by
   its acceptance criterion, not by code existing. Findings and fixes of the
   earlier review rounds are in `docs/reviews.md` and `docs/reviews-later.md`.
3. `docs/design.md` — how it works: processes, state directory, interface. Two
   parts live next to it: `docs/launch.md` (launching a harness, signals) and
   `docs/delivery.md` (sending, reading, reports, the answer to a question).
   The rules 2.0 keeps, each with the tests that hold it, are in `docs/rules/`: open the
   group a change touches before changing how mail, tools, channels or launches behave.
4. `docs/traps.md` — what behaves other than expected, in rewake and in the
   harnesses it lives with, by symptom.
5. `docs/research.md` — facts about each harness, marked with where they were
   verified. They age with harness versions: re-check before touching an adapter.
   Split by how a fact is obtained, because that is how it ages:
   `docs/research-launch.md` for what a binary answers when you run it — models,
   efforts, argument forms; `docs/research-protocol.md` for what the generated
   schema and the reference tree state; `docs/research.md` for what only a running
   session shows, with Codex's share of that in `docs/research-codex.md`.
6. `archive/` — not live code: the 1.x Codex adapter and MCP injection kept as a
   reference for stage 5, a nested module no check builds (`archive/1.x/codex/README.md`).

The full map of the documentation is `docs/README.md`: every document, grouped by
purpose, with what it contains and when to open it. A change that adds, removes or
renames a document, or changes what one is for, updates the map in the same commit.
`docs/map_test.go` runs with the five checks and fails when a document, in `docs/` or
any directory below it, is on no map, or when a link in `docs/` names a missing file or
heading. A document moved to `docs/archive-1.x/` keeps its bytes, checked against the
hash table in that directory's index; the links of an archived document and of a record
resolve through that table, every other document's links are rewritten.

## Keeping the documentation true

The documents above are the memory of the project; the code is not. Whatever
is not written there has to be recovered from the code next time, and that is
slower and less reliable than writing it down while it is fresh. So the
documentation is part of every change, not a task after it:

- A change of behaviour or contract lands in the same commit as the code:
  `docs/flow.md` when the path of a message changes, `docs/design.md`,
  `docs/launch.md` or `docs/delivery.md` when the mechanism does,
  `docs/research.md` — or its launch, protocol or Codex companion, whichever matches
  how the fact was obtained and which harness it is about — when a fact about a
  harness is learned or found wrong.
- Every review round and every milestone is recorded in `docs/roadmap/` as soon
  as it closes — one file per entry, listed in its `README.md`: what was found,
  what was done, what stays open. Older review rounds move to
  `docs/reviews-later.md`; a file that passes 400 lines is split by date.
- An owner decision is written down where it applies, dated, as a plain
  statement of what was decided; the wording is not quoted.
- The substance of `docs/project.md` — the essence, the principles and what the
  tool does — changes only by the owner's decision. Its links are kept current like
  any other: a document that moves or is renamed updates them in the same commit.
- Before a commit, ask what the change taught that the documents do not yet
  say — and what they say that is no longer true. Rewriting what has gone
  stale is part of the same change: outdated text is corrected or removed,
  never left beside the new and never postponed to a clean-up later.
- A fact about a harness carries where it was verified (which version, live or
  read in the source), because these facts age.
- A verbatim record of an observation is never edited afterwards, so it may
  contain names the rest of the documentation avoids — a model's, for one. The
  rule that keeps such names out protects the project from depending on any one
  of them: they have no business in code, in configuration, or in text saying
  how the system works. A record of what was seen on a given day is none of
  those. Striking a name from it would not make the project more
  provider-agnostic; it would make the evidence untrue, and an acceptance
  document without evidence is empty. The dividing line: a name in text that
  explains how something works or should work is incidental and goes; a name in
  a record of what was observed is part of the observation and stays.
- An artifact address is not an explanation. The registry name of a harness
  package, the path of the binary inside it and the command that starts it are
  what the code fetches and runs, so they stay in code and in documentation
  exactly as they are. Without them nothing can be fetched or run; replaced by a
  variable, the same string only moves to wherever the variable is set, and the
  dependency is as real as before. The rule against names governs text that
  explains how something works or should work — not an address the system has to
  use, just as it does not govern a verbatim record. A model or provider name passed
  as a value inside such a command or example is not an address, and the ordinary
  rule applies to it.

## Checks

```bash
gofumpt -l $(go list -tags rewakefixture -f '{{.Dir}}' ./...)   # empty; the module, not the tree
go vet ./... && go vet -tags rewakefault ./... && go vet -tags rewakefixture ./... &&
  go vet -tags rewakefault,rewakefixture ./...
staticcheck ./... && staticcheck -tags rewakefault ./... && staticcheck -tags rewakefixture ./... &&
  staticcheck -tags rewakefault,rewakefixture ./...
golangci-lint run ./...         # config in .golangci.yml; golangci-lint fmt formats
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on ./...
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on -tags rewakefixture ./...
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on -tags rewakefault,rewakefixture ./...
```

All five green is the condition for the commit that lands a step on `v2` — the step's
last commit. The step's intermediate commits need only build: `go build ./...` and
`go vet ./...`. Fixes after a review run the tests of the packages they touch while the
work goes on, and the five checks once, on the last commit, before acceptance. A red
check is never somebody else's: everything in the working tree belongs to the current
work.

The three test runs take most of the time, and `-p` sets how many packages build and
test at once. Pass `-p 4` when the process list shows no other heavy run (below), and
`-p 2` when one is going.

Two forms differ from the obvious one. `gofumpt` walks the filesystem rather
than the module: a plain `.` formats whatever Go file happens to lie under the
working directory, which is not the same set as the project. The package list
asks the module what belongs to it, and that is the answer this check wants.

Production code also builds under two tags: `rewakefault` adds the fault seam of the state
directory, and `rewakefixture` the fixture harness the workflow suite runs as its third
column (`docs/v2/stage3-fixture.md`). Vet and staticcheck run without them, with each and
with both, so tagged code is compiled and analysed by the checks rather than only by the
rig that builds it; the package list `gofumpt` reads and golangci-lint's run name the
fixture's tag for the same reason. The whole tree's tests run again with the fixture in
the catalog, alone and with the fault seam, so every package meets the tagged catalog and
the layout test holds its rules over that variant too. There the fixture is a harness
like any other, and the layout test looks for it as for a harness built only for tests:
by its title in any text and by its id only as a whole string literal, since "fixture" is
also the plain noun the core's tests use for their fakes (`nameWords` in
`internal/layout_rules_test.go`).

The `go test` line carries two tests of the shape of the code and the documents.
`internal/layout_test.go` holds the import rule of 2.0 over every build variant, with
and without `-race`, keeps harness names inside the adapters and forbids a registry of
adapters; its exception tables shrink as stage 3 moves packages. An entry records how many findings it excuses
and which, so one added beside them fails, and an entry nothing matches fails.
`docs/rules_test.go` requires every rule in `docs/rules/` to name tests that exist, that
`go test` would take for tests, and that a `go test` of the five checks reaches and builds.

And `go test` inherits this session's `REWAKE_*` variables unless they are
cleared. Clear them: the risk is not a red run but a test writing into the
owner's live state directory, or reading the running session as its own.

How the project is tested — tiers, what each proves, reading a result, checking a new
harness version, extending the suite — is in `docs/testing.md`, which explains these
commands without repeating them; this section is the one place the commands live.

The workflow suite in `test/workflow` runs a built rewake end to end. It is off
by default — its scenarios skip themselves, so the five checks stay cheap while
still compiling and analyzing the code. To run it:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 go test -count=1 -timeout 30m -v ./test/workflow/...
```

It builds its own binary and runs in a private HOME, state directory and PATH;
it never touches an installed rewake or a live session. With the switch set and
no scenario selected, the run fails rather than reporting green on nothing.

`-v` is in that command on purpose: `go test` shows nothing a passing package
printed, so without it the line naming how many scenarios ran — and which — is
invisible, and a run that exercised three of four looks the same as one that
exercised all four.

The suite runs its cases in parallel, six at a time by default (`-parallel 8` on an
idle machine, `4` on a busy one; `docs/testing.md`), and takes about three minutes.
The `-timeout 30m` leaves room for the first download of a named Codex version below;
without enough room the run dies with `panic: test timed out` and no case to say why.

A change to the Claude Code plugin module (`internal/harness/claude/plugin.js`)
or to the fixture that hosts it also runs the workflow suite before the commit.
The five checks do not start the module under the harness's host, and a module
that fails there unloads silently: every plugin case goes red while the five
checks stay green.

For reading rather than watching, the same run goes through the summarizer, which
prints a handful of lines and writes the whole result to a file:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 go run ./tools/checksummary \
  -- go test -count=1 -timeout 30m -json ./test/workflow/...
```

It exits 0 only when every case passed or was unsupported for a named capability,
and names the failing observation and its evidence directory when one did not.
`go run ./tools/checksummary --help` prints the flags and what each exit code means.
`go run` reports every non-zero exit as 1 and prints the real one as `exit status N`,
so a script that needs the codes apart builds the binary first.

To check a Codex version before installing it, name it — an exact version, `latest` or
`installed` — and give go test room for a first download:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 REWAKE_CODEX_VERSION=0.156.0 go run ./tools/checksummary \
  -- go test -count=1 -timeout 30m -json ./test/workflow/...
```

The version is fetched once, before any case starts, into
`~/.cache/rewake/harness/codex/<version>/` (`REWAKE_HARNESS_CACHE` moves it), and the
schema is generated in a disposable docker container; only the schema comes from it,
the scenarios run against the fixture. The fetch gets half of `-timeout`, at most ten
minutes, and the suite the rest: under go test's default of ten minutes a slow first
download would be cut at five, and a download that outlived the timeout would kill the
test binary with no case to say why. The suite alone takes about three minutes, so
a slow first download taking its full ten minutes still fits in the thirty. The
variable is read only under `REWAKE_WORKFLOW`.
`go run ./tools/harnesscache --help` lists, fetches and removes cached versions;
nothing is removed automatically.

The crosswise check runs each control's observations in every other control's world;
run it after adding or changing a control:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 REWAKE_WORKFLOW_CROSS=1 go run ./tools/checksummary \
  -- go test -count=1 -json -run Crosswise ./test/workflow/...
```

The tools are installed with `go install` into `~/go/bin`, which has to be on
`PATH`:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest mvdan.cc/gofumpt@latest \
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

Sessions working on this repository share one machine, and heavy runs collide when two
of them go at once: the five checks with `-race`, the workflow suite, the crosswise check,
container probes. Before starting one, check whether another session is running one
(`go test`, `checksummary`, `test/workflow`, `harnesscache` in the process list). If it
is, wait for it with a bounded wait, or run only the narrow part needed — one package,
`-run` on one scenario. A timing measured while another heavy run was going is not
evidence: re-measure it, or say it was taken under load.

Live runs happen in a separate `/tmp` directory with its own `REWAKE_DIR`. From a
session's shell a live launch goes through `env -u REWAKE_SESSION -u REWAKE_EPOCH`,
since rewake refuses to start a harness inside a session with exit 2. Never
touch the harness sessions the owner is working in. Codex runs spend
subscription quota: cheap model, short messages, warn the owner first.

## Adding a harness

Meant to be one move:

1. A new package `internal/harness/<name>` with a type implementing
   `harness.Harness`: `ID`, `Title`, `Summary`, `Examples`, `Notes`,
   `SingleUseFlags`, and from milestone 3 on, launch and delivery.
   `SingleUseFlags` names the flags that harness takes at most once, each with
   all its spellings and whether it carries a value. Returning nothing is not a
   neutral answer — it says every flag may be repeated, and a launch alias plus
   a typed flag then reach a harness that may refuse to parse them. A test over
   the catalogue requires the list to be non-empty.
   `ProtectedDirs` names the harness's own configuration directories, which no
   task may grant a session to write (`docs/grants.md`); a harness that keeps
   none returns nothing, and one that forgets them leaves them grantable.
2. One line in `internal/harness/catalog/catalog.go`.

Everything else follows: the launch command, the guide entry, its own `--help`
page, the step in FLOW, the `harnesses` field of the machine form. A harness
cannot be half-registered — either it is in the catalogue and fully described, or
it does not exist.

## Adding a role or a message kind

**A role** is a value in `internal/role/role.go` plus its line in the list. Its
`Play` field carries what a session of that role is told — the heading, the
ordered steps, the limits — and that one text feeds both the briefing at launch
and the guide's own section, so there is nowhere else to write it. Leave it out
and the compiler asks; fill it with a copy of another role's and a test over the
catalogue says so, because a session reading instructions about a different role
is the failure nothing else would notice. The launch flag, the help and the
briefing follow from the value.

**A message kind** is three places, not two. The kind itself is a constant in
`internal/inbox/inbox.go`; what `send` does with it is a `messageKind` in
`internal/cli/send_<kind>.go` plus its line in `sendKinds`, from which the flag,
the refusal and the help are derived.

Beyond those, several places ask what kind a message is, and **a kind they do
not know is treated as a task**. That is not a neutral default: `inbox.Owed`
answers false only for the kinds it lists and true for everything else, and an
empty kind reads as a task outright — so a new kind obliges its recipient to
report. A blocking `--question` recognizes the terminal kinds by name and takes
anything else for an ordinary answer; the outcome of a finished turn is derived
from the kind as well; and the announcement and the rendering each have their
own list. Before adding a kind that is meant to behave differently — a report
that owes nothing, say — grep for `inbox.Task`, `inbox.Note` and their
neighbours and decide each of those places, or the kind will arrive owing a
report and going unrecognized by whoever is waiting.

## How the CLI is organised

The principles are in `docs/project.md`; where they live in the code:

- The command table is `internal/cli/registry.go`; the parser, the guide, the help
  pages and a refusal's hint are derived from it, so a command is changed there.
- A command builds a model and prints it through `printValue`, which gives the lines
  or that same model under `--json`.
- Every example in the table is a real invocation; a test parses each of them.

## Code

Go. A dependency is allowed, and each one is its own decision: the library must
carry no transitive dependencies and be actively maintained, and one library that
covers several places beats three that each cover one. Prefer the standard library
where it does the job.

Comments and doc comments answer "why" rather than
restating the line below them. Everything — code, comments, docs, commit
messages — is in English; commits are a single subject line, no body.

Keep a file under 400 lines; split by subject, not by size.

Code kept only for an older harness version or for records an earlier rewake wrote
carries the mark described in `docs/legacy.md`.

## Delegation and review

Working files of a review or a delegation — probes, overlays, reports passed between
sessions, backups of a tree — go to `.scratch/` in the repository: it is ignored by git
and by the Go module, and unlike `/tmp` it survives a restart of the machine.

Work is handed out through rewake, with a brief that stands on its own and names
exact paths: the session that receives it runs on a different prompt and cannot
see what the orchestrator sees. The orchestrator then waits for the final report
rather than watching the writer's intermediate code. A session that has dropped
off is not revived by an agent: the owner comes and helps.

A change goes through one chain before it lands: the writer writes; a session on
the same harness as the orchestrator reviews it independently; the writer fixes;
then, for the heavy cases, a session on the Codex side accepts; only then the
commit. Acceptance on the Codex side is spent on the heavy cases only: the Codex
path, transport, protocol, process behaviour. Documents, scenarios on the Claude
Code side, file splits and ordinary edits land after the review alone.

## Commits

Author the commit as the owner and keep both agents as co-authors: every commit
here ends with two `Co-Authored-By` trailers, one for Codex and one for Claude,
whichever of them wrote the change — the orchestrator sets the work and checks
it, the executor writes it, so the record names both. Never one without the
other. The write session may commit and push when the work is accepted; never a
force push, a migration of state, or an automatic restart of anyone's windows. The
current lines:

```
Co-Authored-By: Codex (gpt-6-astra) <noreply@openai.com>
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
```
