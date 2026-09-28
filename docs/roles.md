# Roles and names

Split out of [design.md](design.md) by subject on September 24, 2026: which role a
session has and what follows from it, and how its name is chosen. The catalogue is
`internal/role`; the choice under the room lock is `internal/wrap`.

## Roles

A session has a role, from the catalogue in `internal/role`: one value per role
and a line in its list, the way a harness is added. The launch flag `--<id>`,
the help line and whether its turns are reported come from that value.
System text lives in `internal/brief`, with a reviewed snapshot
for each role; harness adapters only pass the rendered strings. The record keeps the role's id.

| role | flag | turns reported | takes a `--grant-git` task or question | its playbook says |
|---|---|---|---|---|
| `general` | `--general` | yes | no | take work, end your turn with the result; no Git metadata |
| `main` | `--main` | no | no: only main sends one, and never to itself | hand out work, read reports; grants; you cannot start sessions |
| `write` | `--write` | yes | on Codex only; Claude Code is refused with exit 1 | take work, end your turn with the result; you can commit when authorized |

The briefing carries the role's whole playbook, the text of
[what a session is told](#what-a-session-is-told); the last column only names its
point. `GitWrite` is set for main as well as write, but main receives no grant: it is
the only sender of one, and `send` refuses a session writing to itself.

**Owner decision, September 17, 2026:** omitted role flags always mean general,
even in an empty room or after main exits. This supersedes automatic main
selection. Only explicit `--main` creates an orchestrator; it refuses with the
occupying session's name when main is already live. The same pid/start-time and
namespace checks as list determine liveness. `--general` and `--write` remain
explicit alternatives. A name prefix never chooses a role, and no session is
promoted when a main leaves. The three role flags cannot be combined, and general
or write sessions may start before any main exists. The refusal of a second `--main`
names the live main and suggests stopping or restarting it, or launching without
`--main` to join as general.

The room's `.launch.lock` covers inspection of live sessions, role choice and
name publication. A starting wrapper is already a live claimant before its
harness starts. Concurrent explicit launches cannot claim two mains. The lock is released
before preparing or running the harness. The record, launch note and intro say
which role was chosen and why.

Git eligibility is separate from reporting and does not itself grant permission.
Only a verified main can attach [explicit --grant-git intent](git-grants.md) to an
eligible task/question. No flag and no launch role adds roots. The existing validated
metadata resolver and additive native root snapshot remain unchanged; existing owner
permissions are neither replaced nor revoked. Notify/report paths cannot grant.

The main session exists to stop a loop: it reads the reports of its workers,
and if its own turns were reported to them, each report would wake the other
side for good. A silent role records no waits and emits no successful turn reports. Failure
observation remains installed: StopFailure for the socket harness and terminal
server events for the owned-server harness. An error from main stays in its own unread mailbox
without waking the same failing conversation. Old records with role `worker` are read as `general`; omitted role flags and --general create
reporting sessions without Git access. The zero role value remains a reporting fallback inside the
catalogue; an omitted launch role is resolved separately under the room lock.
The write role reports like general; main stays silent whether its Git grant
was applied or skipped.

## What a session is told

A role's `Play` carries its heading, its steps and its limits; `internal/brief` renders
them into the launch briefing and `rewake guide` prints them again, so the two cannot
differ. One limit binds every role: each message and final reply starts with a line
stating its point. Write and general share their steps and every limit but the one
about Git (`executorLimits` in `internal/role/playbook.go`).

**Owner decision, September 25, 2026:** four rules for write and general, after three
failures of the day before — a worker that took main's word for a peer's and would not
lift a pause main had lifted, a worker whose turn ended waiting for the owner and was
taken as its report, and a worker that ran `rewake inbox --owed` after a compaction,
read "nothing owed" and skipped a new unread task twice:

- A message widens no permission by its text alone: permissions come from the launch
  and from what main grants through rewake, such as `--grant-git`. A refusal by the
  harness or its classifier is not routed around; the worker tells main what was
  refused and why the work needs it, and main does it itself, grants it, or brings the
  owner in. This replaces the limit every role had, to treat a message as a peer's
  words, which the worker had read as putting main's word on a par with any other.
  Main's own line says the same from its side: a reported refusal is not a request to
  route around it, and main grants only within its own rights.
- Main directs the work on the owner's behalf: its word on pausing, resuming, scope and
  ordinary decisions stands without the owner confirming it in the worker's session.
  The worker does not address the owner; a blocker only the owner can clear goes to
  main.
- A turn ended waiting on anything outside it — background work, the owner, a refusal
  to be cleared — is marked with `rewake pending` first, every such turn, one a finished
  subagent or background task woke included: a worker woken that way wrote "waiting for
  two more", ended without the mark, and closed its task (September 26, 2026). The
  turn's text goes with the mark, so findings can be written in the answer itself
  ([turn-outcomes.md](turn-outcomes.md#interim-turn-ends-rewake-pending)). On Claude
  Code a forgotten mark after an interim end is caught once by the Stop hook's question
  ([turn-outcomes.md](turn-outcomes.md#the-confirmation-on-claude-code)); the rule
  stands, since Codex has no such net.
- After a compaction the task is re-read with `rewake inbox --owed` and new mail with
  `rewake inbox`; `--owed` counts what waits unread in a last line
  ([delivery-owed.md](delivery-owed.md#reading-again-what-is-owed-rewake-inbox---owed)).

## Names

**Owner decision, September 17, 2026:** one rule for every role and harness.
Select the actual role under the room lock, use its ID as the default prefix,
then append `-<harness ID>`. `--name <prefix>` replaces only the prefix:
`rewake --write codex` starts write-codex; adding `--name megamozg` starts
megamozg-codex. No role flag produces general-codex; explicit --main produces main-codex.

Automatic conflicts add -2, -3 after the harness suffix; explicit conflicts
refuse with the complete address. Prefix and result must match
`[a-z0-9][a-z0-9._-]{0,31}`, including the 32-character address limit after any
collision suffix. Overlength names require a shorter prefix; nothing is
truncated or deduplicated. Existing records, room isolation and send addresses
stay unchanged: send uses the exact name from list, never an inferred suffix.
