# The design of rewake 2.0

Stage 2 of the 2.0 rebuild, written October 5, 2026 on the `v2` branch, for review
before any code. It rests on the owner's decisions of October 5, 2026 — the nine of the
plan and the twelve answers to the revision's proposals — and on the accepted revision
([revision.md](revision.md), commit `8a20bdd`). Every decision names what it rests on
and what is still unknown. The owner answered the questions of the first draft on
October 5, 2026 ([below](#the-owners-answers)); one is still open.

| Document | What it decides |
|---|---|
| this file | the layout of 2.0, the import rule and the test that holds it, the stage plan, the owner's answers |
| [design-api.md](design-api.md) | the adapter API: the capabilities, what the core does without each, the catalogue, minimum versions |
| [design-rules.md](design-rules.md) | the core rules, restated from 1.x without its migration, and the guarantees bound to every tool transport |
| [design-claude.md](design-claude.md) | the Claude Code adapter as one mod: tools, turns, telemetry, control, wake, loading, and the stage 4 probe |
| [design-codex.md](design-codex.md) | Codex: the reserved place, the capabilities it is expected to fill, the pair it must serve |
| [design-state.md](design-state.md) | the state directory of 2.0, clearing the state of 1.x, one build per room |
| [design-docs-tests.md](design-docs-tests.md) | how the documents and the tests of 2.0 are laid out |

Where a fact is marked, the marks are those of the recon: **[doc]** official
documentation, **[types]** the declarations Claude Code 2.1.289 writes for a mod
(`.scratch/v2-recon/cc-2.1.289.d.ts`), **[live]** seen on 2.1.289
(`.scratch/v2-recon/claude-mods.md`), **[code]** read in the tree at `8a20bdd`.

## What does not change

- **The CLI vocabulary** of 1.x stays (decision 5), less what the owner dropped on the
  revision's proposals: the hidden `observe`, `status-tap` and `bridge-hook`, `settle`,
  `--no-mail-tool` on Claude Code, and the Codex notify branch of `turn-ended`.
  `rewake guide` and `rewake edit` stay (answer 12).
- **The CLI is a first-class interface**, not a fallback (answer 12). Every mail
  operation is a command; a harness with no mod, or a session whose mod did not load,
  works through the shell and the guide. The tools a model sees are a projection of the
  same command table ([design-api.md](design-api.md#tooltransport)).
- **The boundaries of `AGENTS.md`**: no pseudo-terminal and no typing into a screen; the
  person's harness configuration is never edited; transcripts are not read; no daemon —
  a wrapper lives as long as its session.
- **What the CLI promises a caller**: one command table, no arguments prints the guide,
  `--json` on every command, an unknown flag is a refusal, exit codes 0/1/2/3.

## The layout

```
cmd/rewake               main: builds the catalogue value and hands it to cli
internal/
  infra/                 state root and paths, locks, the fault seam, proc, boottime, buildtime
  core/                  knows no harness
    mail/                letters, tasks, reports, owed, awaited, questions, notes, withdraw, edit;
                         turn-end journals, marks, read clock, stop, the reconcile barrier, the seam
    receipt/             call operations and their records
    registry/            sessions, names, liveness, epochs, the live capabilities of a run
    session/             the session-state snapshot
    role/  brief/        roles, playbooks, briefings — harness words come from the adapter
    grant/               grant policy and the unforgeable authority
    control/             compact, interrupt and clear as request files
    channel/             the channel record of a run: tool, shell or none
  adapter/               the capability interfaces and the values they exchange; no implementation
    catalog/             the one place that names every adapter; a value, not a registry
    claude/              the Claude Code adapter; mod/ holds the module and its manifest
    codex/               reserved: designed afresh in stage 5 (design-codex.md)
  host/                  the wrapper: launch lifetime, claim, signals, availability,
                         departure, notices, the delivery loop, worktree, launch aliases,
                         the endpoint the adapters reach
  tool/                  the mail tools: descriptor types and their schema encoding, binding,
                         parts, end gate, result bound; the descriptors come from cli
  cli/                   words, help, exit codes — one command table; imports no adapter
tools/  test/workflow/  scripts/
```

What each owns, and where 1.x code lands, is in the revision's one-line table
([revision.md](revision.md#every-part-in-one-line)); the layout above refines it in two
places. The endpoint the mod reaches is the host's (it lives as long as the wrapper),
while what a call means — the descriptor types, binding, parts, the end gate — is
`tool`'s; the descriptors themselves are built by `cli` from its command table and
passed down ([design-api.md](design-api.md#tooltransport)). And
`adapter/codex/gateway` and `tool/mcp` of the revision are not laid out now: nothing of
Codex is ported before stage 5 (answer 7), and how the Codex adapter is built is
decided then.

### The import rule

| Package | May import |
|---|---|
| `infra/*` | the standard library only |
| `core/*` | `infra`, other `core` packages |
| `tool` | `core`, `infra` |
| `adapter` | `core`, `infra` — the values the interfaces carry |
| `adapter/<name>` | `adapter`, `tool`, `core`, `infra`; never another adapter |
| `adapter/catalog` | `adapter` and every `adapter/<name>` — the only package that does |
| `host` | `adapter` (the interfaces), `tool`, `core`, `infra`; never an adapter |
| `cli` | `host`, `adapter`, `tool`, `core`, `infra`; never `adapter/catalog` or an adapter |
| `cmd/rewake` | `cli`, `adapter/catalog` |

Rests on: the layering faults the revision found ([revision.md](revision.md#layering-faults-to-fix)) —
`cli` importing the Claude adapter in five files, the core naming a harness in three
places, the global registry filled from `init`.

### The test that holds it

An import check alone misses what the revision found most: a harness named in a string
(`channel/selection.go:57` compares the harness id with `Codex`; the role texts say "on
Codex" and "on Claude Code" in `role/playbook_main.go:16,41,45,46,49`,
`role/playbook.go:101`, `role/role.go:51`). Comparing a string needs no import. So one
test, `internal/layout_test.go`, runs with the five checks and holds three things:

1. **Imports.** It loads every package of the module with its test imports
   (`go list -deps -test -json`, as `docs/layout_test.go` already asks the module) and
   checks each edge against the table above. A forbidden edge fails with both packages
   and the file that imports.
2. **Harness names outside the adapters.** It parses every Go file under `core`,
   `tool`, `host`, `cli` and `infra` — tests included — and fails on an identifier,
   a string literal or a comment word that names an adapter: the ids and titles the
   catalogue declares, compared without case, plus a short list of words that belong to
   one harness (`mcp__`, `CLAUDE_`, `app-server`, `.claude`, `.codex`,
   `.agents`). The list is the test's own and grows when a review finds a word; an
   exception is a line in the test with its reason, and the test fails on an exception
   nothing matches any more. Comments are included on purpose: a comment that explains
   core code by one harness's behaviour is the first step toward a branch on it.
3. **No registry.** No package under `adapter` declares `func init`, and no
   package-level variable of an adapter type or of a slice or map of them exists outside
   `adapter/catalog`'s constructor. `cmd/rewake` builds the catalogue value and passes
   it down (decision 7 needs the third adapter to be one more argument there).

Still unknown: how noisy rule 2 is on the carried code. The role texts and the channel
fold are known to fail it and are moved in stage 3; the rest is found by running it.

## The stage plan from here

Each stage is closed by its acceptance, not by code existing (`docs/roadmap/README.md`).
Every stage goes through the chain of `AGENTS.md`: rules, a review of the rules, build,
acceptance, live probe where the stage has one, commit.

| Stage | What it builds | Acceptance requires |
|---|---|---|
| 3 core | the layout and its test; `core/*`, `tool`, `host`, `cli` with the full vocabulary on the shell; the carried tests with their rules; the state root of 2.0, clearing 1.x and one build per room ([design-state.md](design-state.md)); a fixture adapter for tests | the five checks; a concurrent scenario where a writer of the old build is still running while a new build launches into the room, and two builds of one revision with different trees refused as different; `internal/layout_test.go` green with no stale exception; every carried rule names its tests and they exist ([design-docs-tests.md](design-docs-tests.md)); the workflow suite's neutral scenarios green in a column on the fixture adapter; Codex-side acceptance, since this is transport and process behaviour |
| 4 Claude Code | the probe first ([design-claude.md](design-claude.md#the-stage-4-probe)), its results written into research with the version; then the adapter and its mod | the probe answered point by point; the suite's Claude column on a mod host; live: a Claude Code main and a Claude Code worker exchange a task, a question, a notify and a report, idle and busy; a read through the tool counted only on proof; the mod not loading is named under `--bare`, `--safe-mode` and `disableAllHooks`; a version below 2.1.287 refused |
| 5 Codex | a design of the adapter first, reviewed; then the adapter | live: a Claude Code main and Codex workers exchange tasks and reports both ways, beside Claude Code workers in the same room; the tool set with the same schemas, one suite scenario per tool in both columns; G2 by reading manifests (decision 4); a version below the minimum refused |
| 6 permissions and the person's UI | grants applied by each adapter's Permissions (on Claude Code in the mod, replacing `grant-hook`); PersonUI | live grants on both harnesses; the person's own status line untouched (answer 4) |
| 7 release 2.0.0 | full live checks, briefings, upgrade notes, the npm package | the full workflow suite and crosswise check; the upgrade from a machine running 1.x walked live ([design-state.md](design-state.md#clearing-the-state-of-1x)) |

Until stage 7, the owner works in a separate 1.x environment and the `v2` branch
releases nothing; 2.0 experiments run with their own `REWAKE_DIR`, and nothing passes
between the generations ([design-codex.md](design-codex.md#meanwhile)). The mixed pair —
a Claude Code main with Claude Code and Codex workers in one room — is required before
the release.

## The owner's answers

The owner answered the design's questions on October 5, 2026; question 4 is still open.

1. **1.x state is deleted on upgrade**, not set aside, after the check that no 1.x
   wrapper is alive, with one line saying what was dropped. That check reads two fields
   of a 1.x record, so 2.0 starts with one legacy mark (`rewake`, dated) beside the
   otherwise empty table of answer 11 ([design-state.md](design-state.md#clearing-the-state-of-1x)).
2. **The first tool set is `inbox`, `send`, `pending`, `whoami`, `retry`, `list`**
   (`cli/bridge_surface.go:21-27` plus the read-only `list`). `withdraw`, `edit`,
   `compact`, `interrupt`, `accept` and `worktree` stay shell-only until a review puts
   each on the surface ([design-api.md](design-api.md#tooltransport)).
3. **A session whose mod did not load has no live TurnBoundary**: a role that must
   report refuses to launch; a task or question to such a session exits 1, naming why;
   no new word `report` is added ([design-api.md](design-api.md#turnboundary)).
4. **Open: a stop whose evidence cannot return** — a record that stays unreadable. The
   accepted rule does not wait for the answer: the stop and its records stay until
   evidence returns, and nothing removes a record to lift it (E1, E8). Main's
   recommendation, pending the owner, is a separate option: an operator decision that
   names the delivery fact as unproven, keeps the operation's identity and the original
   evidence, and marks every later effect with itself, never a claim of delivery
   ([design-rules.md](design-rules.md#not-accepted-an-operator-decision-on-an-unknown-outcome)).
5. **No Codex code on `v2` between stages 3 and 5**: the Codex packages leave the `v2`
   tree at stage 3 (they stay on `main` and in history as reference), so the builds of
   stages 3 and 4 launch Claude Code only ([design-codex.md](design-codex.md#meanwhile)).
6. **A harness version that cannot be read refuses the launch**, saying how it was
   asked; a mod against an unknown API fails silently, which is worse
   ([design-api.md](design-api.md#minimum-versions)).
