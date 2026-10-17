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
   The rules 2.0 keeps are in `docs/rules/`, a reference of behaviour: open the group a
   change touches before changing how mail, tools, channels or launches behave.
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
renames a document, or changes what one is for, updates the map in the same commit; no
test holds the map, so keeping it is part of the change. A document in `docs/archive-1.x/`
keeps its bytes.

## Keeping the documentation true

The documents above are the memory of the project; the code is not. Whatever
is not written there has to be recovered from the code next time, and that is
slower and less reliable than writing it down while it is fresh. The depth matches
the change: behaviour an agent or the owner sees — the commands, `docs/flow.md`,
`docs/design.md` — is updated with it; internals as far as the next reader needs.
Existing documents are not rewritten for tidiness; stale text is fixed when met.

- A change of behaviour or contract lands in the same commit as the code:
  `docs/flow.md` when the path of a message changes, `docs/design.md`,
  `docs/launch.md` or `docs/delivery.md` when the mechanism does,
  `docs/research.md` — or its launch, protocol or Codex companion, whichever matches
  how the fact was obtained and which harness it is about — when a fact about a
  harness is learned or found wrong.
- Every step is recorded in `docs/roadmap/` as it lands — one file per entry, listed
  in its `README.md`, at most ten lines: what was done and what stays open, without a
  list of review rounds.
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

While a step goes on, a commit needs to build and its touched packages to pass:

```bash
go build ./... && go vet ./...
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test ./<the packages touched>/...
```

The last commit of a step, before it lands on `v2`:

```bash
gofumpt -l $(go list -tags rewakefixture -f '{{.Dir}}' ./...)   # empty; the module, not the tree
golangci-lint run ./...         # config in .golangci.yml; golangci-lint fmt formats
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on -timeout 20m ./...
```

and the workflow scenarios the change touches (below). A step is done when it works live
and breaks no existing test. A red test is never somebody else's: everything in the working
tree belongs to the current work. `-p 4` when the process list shows no other heavy run,
`-p 2` when one is going.

`gofumpt` walks the filesystem rather than the module: a plain `.` formats whatever Go file
happens to lie under the working directory. The package list asks the module what belongs
to it, and that is the answer this check wants.

Every `go test` here, the workflow suite's included, runs with the `REWAKE_*` variables
cleared. That is not process but protection: `go test` inherits this session's variables,
and the risk is a test writing into the owner's live state directory, or reading the
running session as its own.

**Before a publication to npm** the full set runs — every build variant, since production
code also builds under two tags: `rewakefault` adds the fault seam of the state directory,
`rewakefixture` the fixture harness the workflow suite runs (`docs/v2/stage3-fixture.md`):

```bash
go vet ./... && go vet -tags rewakefault ./... && go vet -tags rewakefixture ./... &&
  go vet -tags rewakefault,rewakefixture ./...
staticcheck ./... && staticcheck -tags rewakefault ./... && staticcheck -tags rewakefixture ./... &&
  staticcheck -tags rewakefault,rewakefixture ./...
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on -timeout 20m -tags rewakefixture ./...
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on -timeout 20m -tags rewakefault,rewakefixture ./...
```

with the whole workflow suite and the crosswise check below.

`internal/layout_test.go` holds the import rule of 2.0 — the core knows no harness, each
layer imports only what it may — and forbids a registry of adapters
([docs/rules/layout.md](docs/rules/layout.md)).

How the project is tested — tiers, what each proves, reading a result, checking a new
harness version — is in `docs/testing.md`, which explains these commands without
repeating them; this section is the one place the commands live.

### Which tests to write

A test is written when at least one of these holds: it already broke — a regression for a
concrete bug; the breakage would not be seen live, or too late — a lost or duplicated
letter, a report to the wrong recipient, a corrupted room state; it is a contract between
processes — the mod↔host protocol and its drift test, the format of records on disk; it is
pure logic cheaper to test than to try by hand.

Not written: a test that a rule has a test or that a document is on a map; a mutant control
per observation; a generated matrix of crashes and races, or a fault scenario, for a new
feature; a test repeating what a workflow scenario already holds. A new feature gets unit
tests by this criterion and one workflow scenario on its main path when the path crosses
processes. The existing mutants and the fault seam's tests stay while they do not get in the
way; no new ones are added. An existing test that is clearly redundant, slow or flaky may be
removed when it gets in the way, one at a time — never a clean-up for its own sake.

A brief to a writer or a reviewer states this criterion.

### Guarantees

The mechanics of `decide`, stops, the turn-end journals, the build lease and epochs are
frozen: not extended, not redone. A new feature need not fit the model of proven, disproven
or unknown; a report lost in a rare case is acceptable, main asks again. The mod gets no new
reliability mechanism without a live failure that needs it.

### The workflow suite

The workflow suite in `test/workflow` runs a built rewake end to end. It is off
by default — its scenarios skip themselves, so the checks stay cheap while
still compiling the code. To run it:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 go test -count=1 -timeout 10m -v ./test/workflow/...
```

It builds its own binary and runs in a private HOME, state directory and PATH;
it never touches an installed rewake or a live session. With the switch set and
no scenario selected, the run fails rather than reporting green on nothing.
`-run` selects the scenarios a change touches.

`-v` is in that command on purpose: `go test` shows nothing a passing package
printed, so without it the line naming how many scenarios ran — and which — is
invisible.

The suite runs its cases in parallel, six at a time by default (`-parallel 8` on an
idle machine, `4` on a busy one; `docs/testing.md`), and takes about three minutes.
The `-timeout 10m` is about three times that; a run past it dies with
`panic: test timed out` and no case to say why.

A change to the fixture harness — its adapter in `internal/adapter/fixture` or its
program in `test/workflow/fixtureshim*` — also runs the workflow suite before the
commit: a program that breaks there turns every case red while the unit tests stay green.

For reading rather than watching, the same run goes through the summarizer, which
prints a handful of lines and writes the whole result to a file:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 go run ./tools/checksummary \
  -- go test -count=1 -timeout 10m -json ./test/workflow/...
```

It exits 0 only when every case passed or was unsupported for a named capability,
and names the failing observation and its evidence directory when one did not.
`go run ./tools/checksummary --help` prints the flags and what each exit code means.
`go run` reports every non-zero exit as 1 and prints the real one as `exit status N`,
so a script that needs the codes apart builds the binary first.

The crosswise check, part of the set before a publication, runs each control's
observations in every other control's world:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 REWAKE_WORKFLOW_CROSS=1 go run ./tools/checksummary \
  -- go test -count=1 -json -run Crosswise ./test/workflow/...
```

### Tools and the shared machine

The tools are installed with `go install` into `~/go/bin`, which has to be on
`PATH`:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest mvdan.cc/gofumpt@latest \
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

Sessions working on this repository share one machine, and heavy runs collide when two
of them go at once: `go test -race` over the tree, the workflow suite, the crosswise check,
container probes. Before starting one, check whether another session is running one
(`go test`, `checksummary`, `test/workflow` in the process list). If it
is, wait for it with a bounded wait, or run only the narrow part needed — one package,
`-run` on one scenario. A timing measured while another heavy run was going is not
evidence: re-measure it, or say it was taken under load.

Live runs happen in a separate `/tmp` directory with its own `REWAKE_DIR`. From a
session's shell a live launch goes through `env -u REWAKE_SESSION -u REWAKE_EPOCH`,
since rewake refuses to start a harness inside a session with exit 2. A room belongs to
one build of rewake: a build of the working tree refuses to change a room that sessions of
another build hold, so a live run of a new build takes a `REWAKE_DIR` or a room of its own. Never
touch the harness sessions the owner is working in. Codex runs spend
subscription quota: cheap model, short messages, warn the owner first.

## Adding a harness

Meant to be one move:

1. A new package `internal/adapter/<name>` with a type implementing
   `adapter.Harness`: `ID`, `Title`, `Summary`, `Examples`, `Notes`,
   `SingleUseFlags`, `ProtectedDirs`, `Version`, `EnvNotInherited`, `Offers` and
   `Launch`.
   `SingleUseFlags` names the flags that harness takes at most once, each with
   all its spellings and whether it carries a value. Returning nothing is not a
   neutral answer — it says every flag may be repeated, and a launch alias plus
   a typed flag then reach a harness that may refuse to parse them. A test over
   the catalogue requires the list to be non-empty.
   `ProtectedDirs` names the harness's own configuration directories, which no
   task may grant a session to write (`docs/grants.md`) — those outside a project
   and the names it keeps inside one; a harness that keeps none returns nothing,
   and one that forgets them leaves them grantable.
   `Offers` says what the adapter offers beyond Launch, and the plan `Launch`
   returns carries what one run serves — its service, wake, turn boundary,
   telemetry — as the interfaces of `internal/adapter` (`docs/code.md`). An offer
   with nothing in the plan behind it is a capability the CLI admits and the run
   cannot serve.
   The offer answers `Conversation` either way — the flag that names a new
   conversation with rewake's id, or why there is none — and offers `Spaces` only
   where the run proves them through its live set; the catalogue tests hold both
   (`docs/v2/design-api.md`).
2. One entry in `New` of `internal/adapter/catalog/catalog.go`.

Everything else follows: the launch command, the guide entry, its own `--help`
page, the step in FLOW, the `harnesses` field of the machine form. A harness
cannot be half-registered — either it is in the catalogue and fully described, or
it does not exist.

## Working on the Claude Code mod

Claude Code's adapter in 2.0 is one mod ([docs/v2/design-claude.md](docs/v2/design-claude.md)).
The mod API — its events, `$`, rendering, the plugin layout, how a mod is loaded, tested and
debugged, and what changed between versions — is not written down in this repository, and it
changes with every Claude Code release. Load the `mods` skill before planning, writing or
reviewing anything the mod does, and compare the version the skill was taken from with the
installed `claude --version`; where they differ, the skill says what to check. A brief for mod
work names the skill; a mod change made without it is refused in review.

The project's documents do not copy the mod API; they cite the skill, which is kept current
with every Claude Code release. What stays in the documents is rewake's own decisions and
invariants about the mod, and dated records of what its probes observed, each with the
version it was made on.

## Adding a role or a message kind

**A role** is a value in `internal/core/role/role.go` plus its line in the list. Its
`Play` field carries what a session of that role is told — the heading, the
ordered steps, the limits — and that one text feeds both the briefing at launch
and the guide's own section, so there is nowhere else to write it. Leave it out
and the compiler asks; fill it with a copy of another role's and a test over the
catalogue says so, because a session reading instructions about a different role
is the failure nothing else would notice. The launch flag, the help and the
briefing follow from the value.

**A message kind** is three places, not two. The kind itself is a constant in
`internal/core/mail/inbox.go`; what `send` does with it is a `messageKind` in
`internal/cli/send_<kind>.go` plus its line in `sendKinds`, from which the flag,
the refusal and the help are derived.

Beyond those, several places ask what kind a message is, and **a kind they do
not know is treated as a task**. That is not a neutral default: `mail.Owed`
answers false only for the kinds it lists and true for everything else, and an
empty kind reads as a task outright — so a new kind obliges its recipient to
report. A blocking `--question` recognizes the terminal kinds by name and takes
anything else for an ordinary answer; the outcome of a finished turn is derived
from the kind as well; and the announcement and the rendering each have their
own list. Before adding a kind that is meant to behave differently — a report
that owes nothing, say — grep for `mail.Task`, `mail.Note` and their
neighbours and decide each of those places, or the kind will arrive owing a
report and going unrecognized by whoever is waiting.

## How the CLI is organised

The principles are in `docs/project.md`; where they live in the code:

- The command table is `internal/cli/registry.go`; the parser, the guide, the help
  pages and a refusal's hint are derived from it, so a command is changed there.
- A command builds a model and prints it through `printValue`, which gives the lines
  or that same model under `--json`.
- Every example in the table is a real invocation; a test parses each of them.
- Every parsed form carries its class, view or change, in the command table
  (`internal/cli/access.go`); a form is the command with the flags that change what it
  does, so `inbox` and `inbox --owed` are two. A table test fails on a form without one.
  A view writes nothing and takes no lease; a change holds the room's build lease from its
  build check to its last effect, the printing of its answer included.

## The state directory and builds

The base is `$REWAKE_DIR` (by default `rewake-<uid>` in the system's temporary directory);
2.0 keeps everything under its root `v2/`, each room at `v2/rooms/<room>`, and reads the
1.x tree only for the first launch to clear it. No resolver creates a directory: a launch's
bootstrap makes the room under its lease. The rules, with their tests, are
[docs/rules/state.md](docs/rules/state.md):

- **One build per room.** The build id is the SHA-256 of the running executable; every
  writer holds `build.lock` shared and checks `build` before its first effect, and a launch
  of another build takes it exclusive only when nothing holds it (`internal/infra/build`).
- **Every write goes through `internal/infra/state`.** Its primitives refuse with
  `ErrNoLease` when the process holds no lease for the room the path lies in, and with
  `ErrViewWrite` in a view; a static test (`internal/infra/state/bypass_test.go`) fails on
  a write anywhere else under `internal/` unless its table names it with a reason. A test
  that writes a room has it made and leased by `buildtest.Room`
  (`internal/infra/build/buildtest`), as a launch of the test binary would.
- **The fault seam** is the `rewakefault` build of `internal/infra/state`: a binary built
  with the tag takes faults from `REWAKE_FAULT` — a crash, a failure or a hold before the
  n-th durable step, a failed read — so a test can break a step of a process it does not
  run itself (`internal/infra/state/fault_build.go`). A release is never built with it.

An outcome rewake cannot prove — a report that may or may not have arrived — stops the
mailbox until evidence settles it or a decision is recorded:
`rewake decide <name> <report> --arrived | --not-arrived`, by the recipient's run or the
person, kept as a decision and never as evidence; deciding "not arrived" sends the report
again. Its rules are D1–D8 in [docs/rules/effects.md](docs/rules/effects.md), its contract
[docs/v2/stage3-decision.md](docs/v2/stage3-decision.md).

## Code

Go. A dependency is allowed, and each one is its own decision: the library must
carry no transitive dependencies and be actively maintained, and one library that
covers several places beats three that each cover one. Prefer the standard library
where it does the job.

Comments and doc comments answer "why" rather than
restating the line below them. Everything — code, comments, docs, commit
messages — is in English; commits are a single subject line, no body.

Keep a file under 400 lines; split by subject, not by size. Generated files — `protocol.gen.ts`
and `protocol_gen.go`, which `tools/protogen` writes — are exempt.

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

A change goes through one chain before it lands: the writer writes; one review; the
writer fixes the blockers; the commit. A blocker is only one of: it does not work; it
loses or duplicates a letter in an ordinary scenario; it corrupts state; it breaks what
worked. Everything else is a note the writer decides on. A second review looks only at
the blockers' fixes, and happens at most once. Acceptance on the Codex side is spent only
on a change of the on-disk state format or of the mod↔host protocol.

A design document is written only for a step that changes the protocol or the state, and
is at most a page; otherwise the brief is the design. A live probe runs when code cannot
be written without its answer, not ahead of time for a whole list of questions.

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
