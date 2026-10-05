# Design: the adapter API

What an adapter offers the core, as named capabilities, and what the core does when an
adapter — or one run of it — lacks one. The overview is in [design.md](design.md).

## Why capabilities

1.x grew twenty-two optional interfaces beside `harness.Harness`, each found by a type
assertion where it is used: `backend.go` (fifteen, from `GitGrantHarness` to
`ToolWithdrawer`), `mailtool.go` (two), `plan.go` (`Steerable`, `Accepting`),
`worktree.go` and `thread.go` (three) **[code]**. Which harness can do what is spread
over the call sites, and the core learns of a harness by asserting its types. 2.0 groups
them into the eight capabilities of the plan, each optional, each a small interface in
`internal/adapter` carrying narrow values — never `registry.Session` whole, which
`Deliver` hands an adapter today (revision, layering faults).

Rests on: the plan's architecture direction and decision 7 (the third harness is added
without touching the core).

## Two levels: offered and live

A capability is **offered** by the adapter — static, known from the catalogue before a
launch. It is **live** for one run once the run proves it: the Claude Code adapter
offers ToolTransport and TurnBoundary, but a run whose mod did not load has neither
([design-claude.md](design-claude.md#when-the-mod-does-not-load)). The host records the
live set in the session record, next to the channel record it extends, and every
decision another session makes about this one — may a task be sent to it, may it be
compacted — reads the live set, not the offered one.

Rests on: the channel record's rules (a tool can fail to load, `mail-bridge-channel.md`
rules 1–4), now applied to every capability.

## The capabilities

| Capability | What the adapter does | Without it, the core |
|---|---|---|
| Launch | required: describes and starts one run | — an adapter without Launch is not an adapter |
| Wake | brings a notice to the session: wakes it idle, steers it busy where it can | writes the letter; `send` exits 3, "accepted, no notice can reach this session"; the session reads when it runs `rewake inbox` |
| ToolTransport | offers the mail tools to the model and carries their calls under the tool rules | briefs the session on the shell commands; the guide is its interface |
| TurnBoundary | reports a turn's start and end, with the end's answer, and confirms an end before it closes | publishes no report on its own; see [below](#turnboundary) |
| Telemetry | reports context, limits and cost | `list` and main see the state as unknown |
| Control | compacts, interrupts, clears | `rewake compact`, `interrupt`, `clear` to that session exit 1 naming the missing verb |
| Permissions | applies a grant to the run | `send --grant-*` to that session exits 1, as `--grant-git` to Claude Code does in 1.x |
| PersonUI | shows the person the session's mail state | nothing beyond the notices; stage 6 |

### Launch

Everything the catalogue says about a harness today, less the registry: `ID`, `Title`,
`Summary`, `Examples`, `Notes`, `SingleUseFlags` (non-empty, a catalogue test holds it),
`ProtectedDirs` widened to every directory that holds the harness's metadata, so that
`grant/rules.go:317` no longer lists `.claude`, `.codex`, `.agents` in the core. Plus:

- **The minimum version** and how to read the installed one ([below](#minimum-versions)).
- **The plan of one run**: the argv and environment for the harness, given a neutral
  description — name, role, room, the briefing text, the working directory or
  worktree, a conversation to resume, the endpoint's address — and the run directory
  the host owns, where the adapter may write what lives exactly as long as the run.
- **Briefing words**: the sentences the role texts need about this harness (how it
  commits, how a grant behaves on it). The core's role texts take them from here; they
  name no harness ([design.md](design.md#the-test-that-holds-it)).
- **Resume and worktree**, as `Resumer` and `WorktreeHarness` do today.

### Wake

Given a notice — its kind line, preview and members — and the run, it delivers and
answers delivered, held (with the harness's reason) or failed, and later, for a held
one, the outcome. The host's delivery loop (inbox `Server`, batches, window, watch, now
in `host`) decides what to send and when; the adapter only carries it. Whether a busy
session is steered or queued is part of the answer, because the notice texts promise
"active work is steered, idle work is woken" (the guide, MAIL).

### ToolTransport

The tools are the CLI's commands, offered to a model, and **one table** describes
both, so a tool cannot drift from its command, as the examples cannot today
(`AGENTS.md`, "How the CLI is organised"). Who owns what, without `tool` importing
`cli` (which imports `tool`):

- **`cli` owns the table** — `internal/cli/registry*.go`, as now — and the allowlist of
  commands a tool may run (`cli/bridge_surface.go:12-27` today). From those it builds a
  list of **descriptors**: per allowed command, its tool name, its summary, and each
  flag or positional with its kind and plain description.
- **`tool` owns the descriptor type** and what is derived from a descriptor alone: the
  JSON Schema of a tool's input, the normalization of its arguments into the command's
  words, the digest the binding names (T2). It imports neither `cli` nor an adapter.
- **The descriptors travel down by composition.** `cli` builds them once and hands them
  to the host with the launch; the host gives them to the adapter's ToolTransport,
  which renders them — on Claude Code into the generated mod's registrations. The
  call comes back as words for a CLI child of the wrapper's own image, which parses
  them with the same table and checks them against the same allowlist again, as 1.x
  does (`cli/bridge_surface.go:32-36`). A test holds that every adapter receives the
  same descriptors and that each descriptor parses back to its command.

The surface is an allowlist: a command added later stays off until reviewed. The
starting set is the owner's (answer 2 of [design.md](design.md#the-owners-answers)):
`inbox`, `send`, `pending`, `whoami`, `retry`, `list`.

Each tool's schema declares only the command's flags and positionals, in plain words;
none declares a property a harness reserves for itself (on Claude Code `tool`,
`tool_use_id`, `agentId`, `consent` — types `ToolCallReserved`, `ToolCallInput`
**[types]**). The model sees `mcp__rewake__<name>` (decision 9).

The adapter carries a call to the host's endpoint with its **binding** — the
conversation, turn and call ids the harness itself supplied — and returns the answer.
What makes that binding trusted, and when a read counts, is the tool rules
([design-rules.md](design-rules.md#tools-t1t11)); how Claude Code meets them is in
[design-claude.md](design-claude.md#how-a-tool-call-runs).

### TurnBoundary

Reports, for the session's own conversation (never a subagent's): a turn started (its
id); a turn ended (its id, the reason — answered, aborted, refused, failed — and the
final answer); and asks, before an end closes, whether it may: the core answers "go on"
when the end would close a task early ([design-rules.md](design-rules.md#turn-outcomes)).
An adapter that cannot hold an end open says so in what it offers; the core then
cannot keep a task open across an unmarked interim end, and that limit is written in
the adapter's document rather than discovered.

Without a live TurnBoundary a session's turn ends report nothing, and a worker's
contract — "end your turn; your final reply goes to the sender" — does not hold. The
owner decided (answer 3 of [design.md](design.md#the-owners-answers)): a launch of a
role that must report refuses when the boundary cannot be live — a known cause, named;
once running, the session record says the boundary is not live, so a task or question
sent to it exits 1, naming why; no new word `report` is added. A boundary that was live
and is lost — the mod unloaded, its stream gone — is withdrawn from the live set at
once, so a stale record never promises a report. Notifies and reads still work through
the shell; the shell never stands in for the boundary. On Claude Code the boundary
reaches the host by the same checked requests as the tools, so it depends on P4 as they
do ([design-claude.md](design-claude.md#how-a-tool-call-runs)).

### Telemetry

Context tokens, window and percent; rate limits; cost; folded into the session-state
snapshot (`core/session`) by event time, a late event never overwriting a newer one
(`telemetry/state_test`'s property, carried). Reading a transcript is not a source.

### Control

Three verbs, each answered on its own: compact (with optional instructions), interrupt
the running turn, clear the conversation. Requests travel as files in the run's control
directory (`core/control`, carried); the adapter decides how it hears them. A verb the
adapter does not offer is refused by the CLI before a file is written.

### Permissions

Applies a grant — a directory, the Git metadata — to the run, and withdraws it when the
grant ends. The policy and the authority stay in `core/grant`; stage 6 designs this
capability on both adapters.

### PersonUI

The session's mail state shown to the person (a pane, a status). Stage 6 (decision 8).

## The catalogue

`adapter/catalog` exposes one constructor that returns the catalogue value — the list
of adapters in the order the guide shows them. `cmd/rewake` calls it and hands the
value to `cli`; `cli` hands each launch its adapter and the host the interfaces. The
guide, each harness's help page, the step in FLOW and the `harnesses` field of the
machine form all derive from that value, as today. Adding a harness stays one package
plus one line, now in the constructor instead of an `init`
(`harness/harness.go:13-23` today).

## Minimum versions

Each adapter declares its **minimum version** and how to read the installed one. A
launch reads it before the claim (as 1.x does since `aee71ea`):

- **below the minimum**: exit 1, "Claude Code 2.1.280 is installed; rewake needs
  2.1.287 or later" — the installed and the minimum named;
- **unreadable**: exit 1, saying the version could not be read and how it was asked
  (the owner's answer 6);
- **at or above it**: the launch goes on. A version newer than the last one a live
  check ran on is not refused; the launch notes it once, as the pin in
  `codex/server_version.go:31` notes today.

Exact versions in gates go (answer 8): a gate is a minimum or nothing. Claude Code's
minimum is 2.1.287 (decision 1); Codex's is set in stage 5.

**The legacy mark stays as a mechanism with an empty table** (answer 11): the rules of
`docs/legacy.md` and `docs/legacy_test.go` move to 2.0 unchanged, with no marks and the
table of oldest supported versions holding the minimums above. The table and the
adapters' declared minimums are one fact: the test reads both and fails when they
differ. The first raise of a minimum in 2.x turns the marks below it red, as designed.

Still unknown: the mod API's own compatibility policy across Claude Code versions; the
recon found the GitHub copy of the types two weeks stale (`claude-mods.md:14`). Each
stage 4 probe result is written with the version it ran on.
