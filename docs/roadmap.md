# Roadmap

Work for `docs/design.md` closes only after its acceptance criterion passes. This file
keeps the current state and what is still open; the order of what comes next is in
[work-queue.md](work-queue.md). Everything closed before September 21, 2026 — the
first ten milestones and the acceptances of the gateway, session state, grouped inbox
and native mailbox work — is in [roadmap-2026-09-16.md](roadmap-2026-09-16.md), with
the transport milestones of September 17 in
[transport-milestones-2026-09-17.md](transport-milestones-2026-09-17.md) and the review
rounds in [reviews.md](reviews.md) and [reviews-later.md](reviews-later.md).

Open [intermittent bugs](intermittent-bugs.md): the unexplained two-root-thread
refusal on September 17, 2026 cleared after recipient restart.

## How the work is run

- One step, one commit; commit message short, subject line only, in English.
- Before every commit, the five checks listed in `AGENTS.md` are green: the
  formatter over the module's package list, `go vet`, `staticcheck`,
  `golangci-lint` and the race-detected, shuffled test run with the session's
  `REWAKE_*` variables cleared. `AGENTS.md` says why two of them take the form
  they do; this file does not repeat the commands, so they cannot drift here.
  No failing check is waived.
- Live runs happen in a separate `/tmp` directory, with their own `REWAKE_DIR`.
  The user's working harness sessions are left untouched.
- Live Codex runs spend subscription quota: cheap model, short messages, no
  more than one turn per check. Warn the owner before a run that spends quota.
- After milestones 3 and 5, a separate review agent passes over the diff:
  looking for defects and for checks that stay green on broken code. Findings get
  fixed before the next milestone.
- A milestone marked **live criterion** is accepted by running the real
  harnesses, not by tests.

## Milestone 6. Ready for daily use — open

- Cleaning up `done/` by age; clear failures at the edges (a session dying
  while a wait is in progress, a taken name, an unreachable directory).
- `README.md`: what it is, installation, the three commands, the trust
  boundary.
- Switching the inbox from polling to inotify, with polling as a fallback.
- Building for linux-amd64 and linux-arm64, publishing `@iiiokojiadbi/rewake`
  with platform packages; publish only on the owner's explicit word.

Done so far: the mailbox is watched rather than polled and finished messages are
swept by age, both on September 16, 2026
([progress record](roadmap-2026-09-16.md#milestone-6-progress-september-16-2026));
`README.md` exists; `scripts/pack.sh` builds the platform packages without
publishing ([local installation](install.md)). Not done: the publication itself.

Acceptance: a week of use without manual intervention; not one case of a
message silently getting lost.

## Later, as needed

- pi, opencode, grok — their delivery paths are already covered in
  `docs/research.md`.
- A busy/idle signal for the recipient, and choosing delivery priority from it.
- A status column in `list` sourced from the Claude Code registry.
- macOS: replacements for `/proc` (`lsof`, `ps -o lstart`).
- Read receipts: `rewake inbox` already writes a `read` status; `send` does not
  report it yet.
- Permissions on request (owner idea, September 17, 2026): "grant permissions
  for actions on request — say review cannot reach a folder in /tmp", and
  "restrict the worker and grant it rights dynamically". The mechanism already
  exists for Git metadata: `turn/start.runtimeWorkspaceRoots` travels with a
  delivered task. A general form would be `rewake send <name> --grant <path>`,
  accepted only from the main role, with paths checked (existing, no symlink
  escape) and the thread's roots only ever extended; a general session would
  start with its working directory alone. Limits: Codex through its app-server
  only (Claude Code needs its own research), a turn the person starts in the
  TUI uses the stored roots, and the sandbox has to be workspace-write.

## Risks

| risk | mitigation |
|---|---|
| Claude Code's private socket protocol changes or gets disabled remotely | delivery is isolated in the adapter; on socket failure, a clear `failed`, not silence |
| owned-server thread identity is unavailable or ambiguous | fail explicitly; retain accepted reports when their notification fails; terminal ownership remains open |
| a fresh thread finishes before observer subscription | first input can start immediately; an observation gap reports an error without inventing a result |
| the wrapper died, the harness is alive | the session is treated as dead; delivery fails with a reason instead of going silent |
| identical text in a row gets dropped by the recipient | a short id in every message's header |

## Local installation

See [local installation without publishing](install.md) for source and package checks.

## The Codex transport pin — done, September 21, 2026

The version the app-server transport was last observed working against moved from
0.154.0 to 0.155.1, and the string now lives in one constant,
`lastObservedServerVersion` in `internal/harness/codex/server.go`; the launch note
and the test fixture both read from it, the fixture repeating the literal on purpose
so that a typo fails a test instead of matching itself. The constant was first named
`verifiedServerVersion` and renamed the same day: two probes on a version — the
ordinary path and steer — are an observation, not a verification, and the name said
more than the evidence did. A mismatch produces a note, never a refusal: an
unobserved version is a reason to warn, not to stop a launch the owner asked for.

What was observed on 0.155.1, and what was not, is in [research.md](research.md)
under the tag of that version; the schema facts are in
[research-protocol.md](research-protocol.md). Open: every fact tagged 0.154.0 stays
unrechecked on 0.155.1, and moving the pin again means walking the fixture types
against the new schema by hand. Checking a new version in a disposable environment
before the owner installs it is queued in [work-queue.md](work-queue.md).

## Launch defaults from the environment and from settings files — done, September 21, 2026

A launch may take its model and reasoning effort from `REWAKE_CODEX_MODEL`,
`REWAKE_CODEX_EFFORT`, `REWAKE_CLAUDE_MODEL` and `REWAKE_CLAUDE_EFFORT`, supplied as
flags for that one launch and announced in a launch note. Strongest first: a flag on
the line, a variable already in the environment, `.rewake.env` in the working
directory, then `~/.config/rewake/settings`. An unset value means no default and no
guess; the set of names a file may decide is closed, so a file found in whatever
directory somebody is in cannot steer the state directory or the room. No model name
is in the repository.

Where it lives: `internal/harness/defaults.go` for the substitution and the note,
`internal/harness/settings.go` for the files and their order, and in each adapter
the pair of defaults that harness takes. Documented in [launch.md](launch.md) under "Launch defaults
from the environment"; what each harness accepts for a model and an effort, and in
which spelling, is in [research-launch.md](research-launch.md). The workflow suite's
paid tier relies on it: a case sets two variables and touches no harness
configuration ([check-runner.md](check-runner.md)).

Not to be developed further ([work-queue.md](work-queue.md)): an alias states the
choice explicitly, which is what a default was approximating.

## The workflow suite — in progress since September 21, 2026

`test/workflow` runs a built rewake end to end: its own binary, a private HOME, state
directory and PATH, process groups it owns and kills, and a switch, `REWAKE_WORKFLOW=1`,
without which every scenario skips itself and the five checks stay cheap. With the
switch set and nothing run, the suite fails rather than reporting green on nothing.
The harness in front of it is a shim that plays the Codex app-server, re-executed
from the test binary, whose answers are checked against the saved 0.155.1 schema.

What exists, by scenario name: `stub`; `codex-conversation-accepted`, a Codex session
taken to an accepted conversation without delivery, with readiness controls that must
turn the case red; `task-report`, the first of the three selected scenarios — a task
delivered to an idle session and the report that answers it — with its negative
controls; `second-terminal`, which states that a turn ended twice by a misbehaving
server yields one report, and says in its own text that it has no reachable control;
and two self-checks, `shim-answers-match-schema` and `self-check-incomplete`.
The scenarios are described in [check-runner-scenarios.md](check-runner-scenarios.md),
the shape in [check-runner-proposal.md](check-runner-proposal.md), the evidence
contract in [check-runner.md](check-runner.md).

What does not exist: `batch-arrival` and `mid-turn`, the second and third selected
scenarios, and `ack-recovery` after them; a fixture for the Claude Code column, so the
scenario × harness matrix has one column running; and a runner command — the proposal
chose `go test` with a summarizer over `go test -json` and rejected a separate binary,
and the summarizer is not written either. The paid tier with a real model has not run
under the suite.

## The harness research, split by how a fact is obtained — done, September 21, 2026

`docs/research.md` had grown into one file for facts that age at different speeds. It
is now three: [research-launch.md](research-launch.md) for what a binary answers when
it is run — models, efforts, argument forms; [research-protocol.md](research-protocol.md)
for what the generated schema and the reference tree state; [research.md](research.md)
for what only a running session shows. The reading order in `AGENTS.md` names all three
and says which to consult before touching an adapter. Each fact still carries where and
on which version it was verified.

## Launch aliases, on the project's first dependency — done, September 21, 2026

`rewake <alias>` is a whole launch: `internal/alias` reads `[alias.<name>]` tables from
`~/.config/rewake/aliases.toml` and `.rewake.toml` in the working directory, each with
three fields — the harness to start, a string; the flags rewake reads and the
arguments the harness gets, two lists — and the launch proceeds as if the arguments
had been typed. For a flag the harness takes at most once, the ones it names in
`SingleUseFlags`, a copy typed on the line replaces the alias's rather than merely
outranking it, so such a flag reaches the harness once; everything else, `--add-dir`
and `-c` among them, is appended, because repeating those is how a second value is
added. An unknown name is a refusal listing the names that exist; an alias
that expands into something unusable is a refusal showing the expansion; an alias can
name arguments to rewake and nothing else, for the reason the settings file has no
substitution. Lists, not strings: a string would have to be split into words, and
splitting words means quoting rules. Documented in [launch.md](launch.md) under
"Naming a whole launch" and in the launch help; what was
learned about repeatable flags is in [research-launch.md](research-launch.md).

The file is read by `pelletier/go-toml/v2`, the project's first dependency. The
owner's decision of September 21, 2026 lifted the standard-library-only rule: a
dependency is allowed one deliberate decision at a time, judged on having no
transitive dependencies of its own and on live maintenance, and one library that
covers several places beats three that each cover one. The rule is in `AGENTS.md`
and in [design.md](design.md). This library was chosen on reconnaissance — actively
maintained, rewritten this year, faster, current specification, and, like its main
alternative, with no transitive dependencies of its own, which was the deciding
property. Why the alias came only now: a hand-written TOML parser here broke on valid
TOML through two review rounds and was removed. The format was not the problem;
writing the parser was.

Open: parsing the Codex configuration with the same library instead of looking for a
mention of a key in the file's text, queued in [work-queue.md](work-queue.md).

## A role-shaped first page — done, September 21, 2026

An agent now reads what its own role does, in the briefing it is launched with and again
in `rewake guide`, which recognizes the session calling it and opens with that role's
moves. A caller that is not a session sees the general map, unchanged.

Both come from one place, `internal/role.Playbook`, the `Play` field of a role: the
heading, the ordered steps and the limits are written once and rendered twice. A copy would have drifted the
way copies here have drifted before, and it would have drifted unevenly — the briefing
arrives first and is read once, the guide is consulted later and often, so a
disagreement between them would be settled in favour of whichever the reader saw first.
A test requires the briefing to carry every step and limit of the playbook.

What prompted it: on September 21, 2026 an executor sent every report by hand *and* let
the turn report it, all day. `flow.md` and `delivery.md` both described the mechanism
correctly; no instruction said what to do about it. The playbook says it in one line —
ending the turn is what sends the report — and the guide repeats it to the same session
later.

## The research packages, folded into the documents that outlive them — done, September 22, 2026

The local research packages — raw runs, fixtures, launch records, and the Python
harness that checked the native arrival row — were deleted. What they proved was
written into the documents that stay: the owner's decisions in their own words in
[owner-decisions.md](owner-decisions.md), what behaves other than expected in
[traps.md](traps.md), the evidence and its limits in
[native-mailbox-acceptance.md](native-mailbox-acceptance.md),
[native-mailbox-ui-check.md](native-mailbox-ui-check.md),
[gateway-native-evidence.md](gateway-native-evidence.md) and
[intermittent-bugs.md](intermittent-bugs.md). The reading order in `AGENTS.md` now
names the first two. Every document that cites a deleted run says so and dates the
deletion, so a reader knows the record cannot be re-examined.

Open: the arrival-row check has no automated stand any more; rebuilding it on the
workflow suite, up to the one step a person has to observe, is in
[work-queue.md](work-queue.md).

## Remaining work — owner decisions, September 19–21, 2026

What comes next, in the order the owner set on September 21, 2026, is in
[work-queue.md](work-queue.md): the rest of the workflow suite, then pinning harness
versions in a disposable environment, then a two-way channel for Claude Code, then the
parity queue. Launch aliases, which stood second in that order, closed the same day
(above). This section keeps the decisions behind those items.

The native-notification priority is complete, and so is the Claude Code handoff it
pointed to: orchestration moved to Claude Code on September 21, 2026. The first three
entries of the [parity queue](harness-features.md) closed the same day — HF-12, HF-07
with HF-20, and HF-21 — each on one observed run
([claude-parity-2026-09-21.md](claude-parity-2026-09-21.md)). Queue entries are not
milestones; the remaining ones are listed in the feature map.

Check automation is the [workflow suite](check-runner.md) above: the research answer,
[check-runner-proposal.md](check-runner-proposal.md) with its selected scenarios in
[check-runner-scenarios.md](check-runner-scenarios.md), was accepted on September 21,
2026 and the first scenario is implemented; short console output and a summary file,
and paid and manual tiers as opt-in, are still requirements rather than code.

A command that lists the models and effort levels a harness offers is **deferred**.
Owner decision, September 21, 2026, asked directly: not now. The reconnaissance that
would feed it is done and recorded in [research-launch.md](research-launch.md) — what can be read
from each harness, how, and at what cost — so the work would start from facts rather
than from scratch. Roughly a day.

It would answer, per harness: which models are available to this account, which effort
levels each of them takes, and what the configured context window is. For Codex the
window comes from the catalogue and a configuration key, with arithmetic. For Claude
Code neither the subcommands that were read nor the catalogue cache carries one — but
the Agent SDK and the data handed to the status line do, and the reconnaissance did not
go there. So the open question is not
"is there a source" but "which of them is worth depending on", and nobody has yet found
a catalogue of windows for every model that does not require starting a session.

The sources are uneven: one protocol method, belonging to the Codex protocol, and three
unofficial ones — a debug subcommand group, an undocumented per-login cache, and the
text of a warning message. On the Claude Code side the documented route is the SDK.

The requirement that shapes it: **a source that disappears must turn its cell into
"unknown", never leave a stale truth standing.** Caches expire, debug commands are not
promises, and an account's list changes. A table that keeps showing yesterday's answer
because today's lookup failed is worse than one that admits it does not know — that is
the same mistake as a check reporting success it did not earn, one layer up.

A lead for the missing Claude Code telemetry collector (entry 5 of the
[parity queue](harness-features.md)). Claude Code has a channel where the harness calls
a command **we** name and hands it a structure describing the session, the context
window size among its fields — so the source is the input to a program of ours, not
text meant for a person. rewake already works this way elsewhere: the wrapper sets a
command for the end of a turn, which calls `rewake turn-ended`
(`internal/harness/hooks.go`). A different integration point, the same shape — the
harness calls what we named and hands it data — and that shape looks like a fit for
the collector.

Not proposed, and not to be done: parsing the rendering of the status line. It is a
display for a person, its format is not promised, and a reader of the screen would be
fragile for no reason. How much that structure actually carries — model, effort,
context fill, compactions, activity, the things collected from Codex — has not been
established; that is its own reconnaissance.

Persistent Git permissions remain a separate future item: explicit orchestrator
event, scope to run and repository, preservation across later owner turns, and revocation. A running turn keeps its prior permission context.
Only validated Git metadata roots are in scope, not tools, credentials or arbitrary
folders. Do not implement that persistence in the current batch feature.
The main-readiness/missing-notice incident stays open separately.
