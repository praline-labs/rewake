# rewake

rewake lets coding-agent sessions on one machine pass messages to each other. Before
acting on any task in this repository, read `docs/project.md` in full — every section, not
skimmed. It is the project's goal: the rules, the code, the reviews and the briefs you
receive follow from it and assume you know it; a decision made without it is likely to
contradict the goal and be refused or redone.

## Where to start a session

1. `docs/project.md` — first: the essence, the principles and what the tool does, each
   linked to its specification, and what to read next by kind of work.
2. `docs/flow.md` — the whole path in one read: a session starts, a message is sent,
   announced, read, and the report comes back.
3. `docs/design.md` — how it works: processes, state directory, interface; its parts
   `docs/launch.md` and `docs/delivery.md`.
4. `docs/roadmap/README.md` — what is open and the order of work; a step is closed by its
   acceptance, not by code existing.

Then by the work, through the map `docs/README.md`: `docs/rules/` before changing how mail,
tools, channels or launches behave; `docs/traps.md` when something misbehaves; research for a
harness's facts, re-checked before touching an adapter. A change that adds, removes or renames
a document, or changes what one is for, updates the map in the same commit.

## Keeping the documentation true

The documents are the memory of the project; the code is not. The depth matches the change,
and stale text is fixed when met, never rewritten for tidiness.

- A change of behaviour or contract lands in the same commit as the code: `docs/flow.md`
  when the path of a message changes, `docs/design.md`, `docs/launch.md` or
  `docs/delivery.md` when the mechanism does, the research document that matches how a fact
  about a harness was obtained when one is learned or found wrong.
- Every step is recorded in `docs/roadmap/` as it lands — one file per entry, listed in its
  `README.md`, at most ten lines: what was done and what stays open.
- An owner decision is written down where it applies, dated, as a plain statement of what
  was decided; the wording is not quoted.
- The substance of `docs/project.md` changes only by the owner's decision; its links are kept
  current like any other.
- Before a commit, ask what the change taught that the documents do not yet say, and what
  they say that is no longer true. A fact about a harness carries where it was verified.
- No model or provider name in code, configuration or text that explains how the system
  works. A verbatim record of an observation keeps the names it saw, and an artifact address
  — a package name, a binary path, a command — stays exactly as the system must use it; why,
  in `docs/README.md`, "Names in records and addresses".

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

Every `go test` here, the workflow suite's included, runs with the `REWAKE_*` variables
cleared: `go test` inherits this session's variables, and a test could write into the
owner's live state directory or read the running session as its own.

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

`docs/testing.md` explains these commands — tiers, reading a result, a new harness version —
without repeating them; this section is the one place the commands live.

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

`test/workflow` runs a built rewake end to end in a private HOME, state directory and PATH;
it never touches an installed rewake or a live session. Its scenarios skip themselves unless
switched on, and with the switch set and no scenario selected the run fails:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 go test -count=1 -timeout 10m -v ./test/workflow/...
```

`-run` selects the scenarios a change touches; keep `-v`, or the line naming what ran is
invisible. About three minutes, six cases at a time. A change to `internal/adapter/fixture` or
`test/workflow/fixtureshim*` runs the whole suite. The summarizer prints a few lines and writes
the rest to a file (`go run ./tools/checksummary --help`):

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 go run ./tools/checksummary \
  -- go test -count=1 -timeout 10m -json ./test/workflow/...
```

The crosswise check, part of the set before a publication, runs each control's
observations in every other control's world:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 REWAKE_WORKFLOW_CROSS=1 go run ./tools/checksummary \
  -- go test -count=1 -json -run Crosswise ./test/workflow/...
```

### Tools and the shared machine

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest mvdan.cc/gofumpt@latest \
  github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest   # into ~/go/bin, on PATH
```

Sessions share one machine: before a heavy run (`go test -race` over the tree, the workflow
suite, the crosswise check) check the process list for another and wait with a bounded wait or
run only the narrow part; a timing taken under load is not evidence. Live runs use their own
`REWAKE_DIR` in `/tmp` — a room belongs to one build — and start through
`env -u REWAKE_SESSION -u REWAKE_EPOCH`, since a launch inside a session exits 2. Never touch
the owner's sessions; a run that spends quota uses a cheap model, and the owner is warned.

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

**A role** is a value in `internal/core/role/role.go` plus its line in the list. Its `Play`
field carries what a session of that role is told, and that one text feeds both the briefing
at launch and the guide's section; a copy of another role's fails a test over the catalogue.

**A message kind** is three places: the constant in `internal/core/mail/inbox.go`, a
`messageKind` in `internal/cli/send_<kind>.go`, and its line in `sendKinds`. Beyond those,
**a kind the other places do not know is treated as a task**: `mail.Owed` is true for every
kind it does not list, so a new kind obliges its recipient to report, and a blocking
`--question`, a turn's outcome, the announcement and the rendering each recognize kinds by
name. Before adding one meant to behave differently, grep for `mail.Task`, `mail.Note` and
their neighbours and decide each place.

## How the CLI is organised

- The command table is `internal/cli/registry.go`; the parser, the guide, the help pages and
  a refusal's hint are derived from it, so a command is changed there.
- A command builds a model and prints it through `printValue`, which gives the lines or that
  same model under `--json`.
- Every example in the table is a real invocation; a test parses each of them.
- Every parsed form is a view or a change (`internal/cli/access.go`): a view writes nothing
  and takes no lease; a change holds the room's build lease to its last effect, its answer's
  printing included.

## The state directory and builds

The base is `$REWAKE_DIR` (by default `rewake-<uid>` in the system's temporary directory);
2.0 keeps everything under its root `v2/`, each room at `v2/rooms/<room>`. No resolver creates
a directory: a launch's bootstrap makes the room under its lease. The rules, with their tests,
are [docs/rules/state.md](docs/rules/state.md):

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

An outcome rewake cannot prove stops the mailbox until evidence settles it or a decision is
recorded: `rewake decide <name> <report> --arrived | --not-arrived`, kept as a decision and
never as evidence; "not arrived" sends the report again
([docs/rules/effects.md](docs/rules/effects.md), D1–D8).

## Code

Go. A dependency is its own decision: no transitive dependencies, actively maintained, and
one library that covers several places beats three; prefer the standard library where it
does the job. Comments answer "why" rather than restating the line below. Everything — code,
comments, docs, commit messages — is in English; commits are a single subject line, no body.
Keep a file under 400 lines, split by subject; the generated `protocol.gen.ts` and
`protocol_gen.go` are exempt. Code kept only for an older harness version or for records an
earlier rewake wrote carries the mark of `docs/legacy.md`.

## Delegation and review

Working files of a review or a delegation go to `.scratch/`, ignored by git and the module.
Work is handed out through rewake with a brief that stands on its own and names exact paths;
the orchestrator waits for the final report. A session that dropped off is revived by the
owner, not an agent.

A change goes through one chain: the writer writes; one review; the writer fixes the
blockers; the commit. A blocker is only one of: it does not work; it loses or duplicates a
letter in an ordinary scenario; it corrupts state; it breaks what worked. Everything else is a
note the writer decides on. A second review looks only at the blockers' fixes, at most once.
Acceptance on the Codex side is spent only on a change of the on-disk state format or of the
mod↔host protocol.
A design document is written only for a step that changes the protocol or the state, at most
a page; otherwise the brief is the design. A live probe runs when code cannot be written
without its answer.

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
