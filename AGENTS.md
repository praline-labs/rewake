# rewake

rewake lets coding-agent sessions on one machine pass messages to each other. Before
acting on any task in this repository, read `docs/project.md` in full — every section, not
skimmed. It is the project's goal: the rules, the code, the reviews and the briefs you
receive follow from it and assume you know it; a decision made without it is likely to
contradict the goal and be refused or redone.

## Where to start a session

1. `docs/project.md`, then `GLOSSARY.md`, the project's own words.
2. `docs/flow.md` — the whole path of a message in one read — then `docs/flow-endings.md`.
3. `docs/design.md`, the system as built, then `docs/code.md`.
4. `docs/roadmap/README.md`: what is open and the order of work, a step closed by its
   acceptance, not by code existing.
5. Then by the work, the reading order of `docs/README.md`: `docs/rules/` before changing how
   mail or launches behave, `docs/traps.md` when something misbehaves, research for a
   harness's facts, re-checked before touching an adapter.

Where each kind of knowledge goes is in `SHIFT.md`.

`archive/` holds the 1.x code, Codex's among it; it stays as it is. Do not read it unless a
task needs a fact only it holds, and leave it out of every count, search, audit and clean-up:
its lines are not rewake's code or tests.

## Keeping the documentation true

The documents are the memory of the project; the code is not. The depth matches the change;
stale text is fixed when met, never rewritten for tidiness.

- A change of behaviour or contract lands with its documents in the same commit — `flow.md`,
  `design.md`, `launch.md`, `delivery.md`, the research document of a harness fact, which
  carries where it was verified — and one that adds, removes or renames a document updates
  the map `docs/README.md`. Before a commit, ask what the change taught that the documents do
  not yet say, and what they say that is no longer true.
- The roadmap, `docs/roadmap/README.md`, holds the open work and its order; a step that
  lands leaves it. A commit's subject is the record of a step, and what the step taught goes
  into the living documents.
- An owner decision is written down where it applies, dated, as a plain statement; the
  substance of `docs/project.md` changes only by the owner's decision.
- No model or provider name in code, configuration or explanatory text; a verbatim record
  and an artifact address keep theirs (`docs/README.md`, "Names in records and addresses").

## Checks

Every commit runs the pre-commit hook of `lefthook.yml` over what it stages: a staged `.ts`
or `.tsx` file goes through `biome check --write` (`biome.json`), a staged `.go` file through
`golangci-lint fmt`, and then the module through `go build ./... && go vet ./...` and
`golangci-lint run ./...`; what the formatters rewrite is staged again, and a finding stops the
commit. The tools are binaries on `PATH`
([docs/testing.md](docs/testing.md#before-a-publication-to-npm)), and `lefthook install` is run
once per clone, which every linked worktree shares.

While a step goes on, a commit also needs its touched packages to pass:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test ./<the packages touched>/...
```

The last commit of a step, before it lands on `v2`, adds what the hook does not run:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  go test -race -shuffle=on -timeout 20m ./...
(cd shell && bun install --frozen-lockfile && bun run check)   # when shell/ or face/ changed
(cd face && bun install --frozen-lockfile && bun run check)    # when face/ changed
```

and the workflow scenarios the change touches (below). The commit-msg hook refuses a message
without a one-line subject and both trailers (`scripts/commit-msg-check.sh`), and the
post-rewrite hook warns on an amend or a rebase, which are forbidden: add a new commit instead.
The mod's own tests run inside
`go test ./internal/adapter/claude/` (`claude plugin test`). A commit made without the hook
runs its checks by hand: `golangci-lint fmt` over the files it touched, `golangci-lint run
./...` and `biome check .`. A step is done when it works live and breaks no existing test. A red
test is never somebody else's: everything in the working tree belongs to the current work. `-p 4` when the process list shows no other heavy run,
`-p 2` when one is going. Before a publication to npm the full set runs — every build
variant, staticcheck, the tagged `-race` runs and the whole workflow suite
([docs/testing.md](docs/testing.md#before-a-publication-to-npm), with the tools to install).

Every `go test` here, the workflow suite's included, runs with the four variables naming a
session cleared: `go test` inherits this session's variables, and a test could write into the
owner's live state directory or read the running session as its own. The others a session
carries (`REWAKE_ROLE`, `REWAKE_EXECUTABLE`, `REWAKE_WORKTREES`, `REWAKE_MOD_SOCKET`,
`REWAKE_WORKTREE_ENTER`, …) stay, so the tests are hermetic themselves: a test that starts
rewake's own code in a child — `bun test`, `claude plugin` — gives it the environment less every
`REWAKE_*`, and one whose code reads such a variable sets it with `t.Setenv`, or it inherits
`REWAKE_ROLE=main` from main's session. The import rule — the
core knows no harness, each layer imports only what it may — is held by `depguard` in
`.golangci.yml` ([docs/rules/layout.md](docs/rules/layout.md)).

`test/workflow` runs a built rewake end to end in a private HOME, state directory and PATH.
Its scenarios skip unless switched on, and with the switch set and none selected the run
fails; `-run` selects those a change touches, and a change to `internal/adapter/fixture` or
`test/workflow/fixtureshim*` runs the whole suite, about half a minute. Keep `-v`, or the line
naming what ran is invisible; the summarizer prints a few lines and writes the rest to a file:

```bash
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 go test -count=1 -timeout 10m -v ./test/workflow/...
env -u REWAKE_SESSION -u REWAKE_EPOCH -u REWAKE_DIR -u REWAKE_ROOM \
  REWAKE_WORKFLOW=1 go run ./tools/checksummary \
  -- go test -count=1 -timeout 10m -json ./test/workflow/...
```

Sessions share one machine: before a heavy run (`go test -race` over the tree, the workflow
suite) check the process list for another and wait with a bounded wait or run only the narrow
part; a timing taken under load is not evidence. Live runs use their own `REWAKE_DIR` in
`/tmp` and start through `env -u REWAKE_SESSION -u REWAKE_EPOCH`, since a launch inside a
session exits 2. Never touch the owner's sessions; a run that spends quota uses a cheap model,
and the owner is warned.

### Which tests to write, and what to build for

The criterion for a test is [docs/testing.md](docs/testing.md#which-tests-to-write); a brief to
a writer or a reviewer states it. The rule of building a protective mechanism only for a
failure seen in real work is [docs/build-for-failures.md](docs/build-for-failures.md); open it
before adding a guard, a retry or a test of a failure.

## Adding a harness, a role or a message kind

The steps and what each touches are in [docs/adding.md](docs/adding.md); open it before adding
one.

## Working on the Claude Code mod

Claude Code's adapter in 2.0 is one mod ([docs/claude.md](docs/claude.md)).
The mod API — its events, `$`, rendering, the plugin layout, how a mod is loaded, tested and
debugged, and what changed between versions — is not written down in this repository, and it
changes with every Claude Code release. Load the `mods` skill before planning, writing or
reviewing anything the mod does, and compare the version the skill was taken from with the
installed `claude --version`; where they differ, the skill says what to check. A brief for mod
work names the skill; a mod change made without it is refused in review.

The project's documents do not copy the mod API; they cite the skill. What stays in them is
rewake's own decisions and invariants about the mod, and dated records of what its probes
observed, each with the version it was made on.

## How the CLI is organised

The command table is `internal/cli/registry.go`: the parser, the guide, the help pages and a
refusal's hint derive from it, and a test parses every example in it. A command prints its
model through `printValue`, as lines or under `--json`. Every parsed form is a view or a
change (`internal/cli/access.go`): a view writes nothing and takes no lease; a change holds
the room's build lease to its last effect, its answer's printing included.

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
- **The fault seam**, the `rewaketest` build of `internal/infra/state`, takes faults from
  `REWAKE_FAULT` (`internal/infra/state/fault_build.go`); a release is never built with it.

A record that cannot be read fails the operation that met it, which is retried; it stops
nothing else, and no command settles a mailbox by hand
([docs/rules/effects.md](docs/rules/effects.md)).

## Code

Go. A dependency is its own decision: no transitive dependencies, actively maintained, the
standard library where it does the job. Comments answer "why". Everything — code, comments,
docs, commit messages — is in English; commits are a single subject line, no body. A file
stays under 400 lines, split by subject (the generated `protocol.gen.ts` and `protocol_gen.go`
are exempt); code kept for an older harness or older records carries the mark of
`docs/legacy.md`.

## Delegation and review

Working files of a review or a delegation go to `.scratch/`, ignored by git and the module.
Work is handed out through rewake with a brief that stands on its own and names exact paths;
the orchestrator waits for the final report; a session that dropped off is revived by the
owner. A change goes through one chain: the writer writes; one review; the writer fixes the
blockers; the commit. A blocker is only one of: it does not work; it loses or duplicates a
letter in an ordinary scenario; it corrupts state; it breaks what worked. Everything else is a
note the writer decides on; a second review looks only at the blockers' fixes, at most once.
A change of the on-disk state format or of the mod↔host protocol gets the reviewer's
acceptance. A design document, at most a page, is written only for a step that changes
the protocol or the state, otherwise the brief is the design; a live probe runs when code
cannot be written without its answer.

## Commits

Author the commit as the owner and keep both agents as co-authors: every commit here ends with
two `Co-Authored-By` trailers, one for Codex and one for Claude, whichever of them wrote the
change — the orchestrator sets the work and checks it, the executor writes it, so the record
names both. Never one without the other. The write session may commit and push when the work is
accepted; never a force push, a migration of state, or an automatic restart of anyone's
windows. The current lines:

```
Co-Authored-By: Codex (gpt-6-astra) <noreply@openai.com>
Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>
```
