---
description: The roles a session runs under, what each is told at launch and owes, and how a session's name is chosen.
---

# Roles and names

Which role a session has and what follows from it, and how its name is chosen. The catalogue is
`internal/core/role`; the choice under the room lock is `internal/host`.

## Roles

A session has a role, from the catalogue in `internal/core/role`: one value per role
and a line in its list, the way a harness is added. The help line and whether its turns are
reported come from that value; the launch flag `--<id>` exists only in the `rewaketest` build.
System text lives in `internal/core/brief`, with a reviewed snapshot
for each role; harness adapters only pass the rendered strings. The record keeps the role's id.

| role | started by | turns reported | its playbook says |
|---|---|---|---|
| `general` | main, `space open --alias` of an alias with `role = "general"` or none | yes | take work, end your turn with the result; no Git metadata |
| `main` | the person, `rewake <harness>` | no | hand out work, read reports; raise workers with `space open`, never by a launch |
| `write` | main, `space open --alias` of an alias with `role = "write"` | yes | take work, end your turn with the result; you can commit when authorized |

The briefing carries the role's whole playbook, the text of
[what a session is told](#what-a-session-is-told); the last column only names its
point. A write session commits within its own permissions.

**Owner decision, October 19, 2026:** a person's launch, `rewake <harness>`, is always main, in the
room of its directory, and has no role or name flag; workers are raised by main alone, with
`rewake space open`, and a worker's launch takes its name and role from the space's record
([space.md](space.md)). A second main in a directory whose main runs is
refused, exit 1, naming it: one main per directory, and another directory is another room. The
same pid/start-time and namespace checks as list determine liveness. No session is promoted
when a main leaves. The `rewaketest` build keeps `--main`, `--general`, `--write` and `--name`
for the workflow suite's direct worker launches; there, too, no role flag means main.

The room's `.launch.lock` covers inspection of live sessions, role choice and
name publication. A starting wrapper is already a live claimant before its
harness starts. Concurrent explicit launches cannot claim two mains. The lock is released
before preparing or running the harness. The record, launch note and intro say
which role was chosen and why: "rewake <harness> always starts main, one per directory", or
"raised by main with rewake space open as <role>".

The main session exists to stop a loop: it reads the reports of its workers,
and if its own turns were reported to them, each report would wake the other
side for good. A silent role records no waits and emits no successful turn reports. A failed turn
is still observed: an error from main stays in its own unread mailbox
without waking the same failing conversation. Old records with role `worker` are read as `general`. The zero role value remains a
reporting fallback inside the catalogue.
The write role reports like general.

Main's briefing holds when a project hides every `write` template (`[aliases] off`): it delegates to a template that fits, and where none writes code it writes the code itself and gives the reviewer the review. `rewake workers` reads the catalogue each time and says so, so a mid-session change of the file needs no restart. rewake grants main no permissions beyond its harness's.

## What a session is told

A role's `Play` carries its heading, its steps and its sections — titled groups of
one-line rules; `internal/core/brief` renders them into the launch briefing and `rewake guide`
prints them again under YOUR ROLE, wrapped, with `--json` carrying them as `sections` of
`title` and `lines`, so the two cannot differ. The briefing is the heading, the steps as
`<command> — <what for>`, each section as its title and `- ` lines, and the line pointing
to `rewake guide`. The last section, MAIL (`role.Mail`), is the same for every role: the
one rule every role shares — each message and final reply starts with a line stating its
point — and what a session sees happen to its mail. Main's sections are HANDING OUT WORK,
READING WHAT COMES BACK, LIMITS and OWNER DECISIONS
(`internal/core/role/playbook_main.go`); write and general share their steps and the sections
WORKING, REPORTING and MAIN AND PERMISSIONS, all but the line about Git
(`executorSections` in `internal/core/role/playbook.go`). A test checks that every command a
step names is in the command table, with each verb it spells — `worktree ls`, `land`,
`finish` — in that command's arguments and each flag, in the command or in what the step
says it is for, one that command takes: a session copies them. The one exception is the
launch flag `--worktree` in worktree's explanation, which a harness declares.

A worker raised for a type ([worker-types.md](worker-types.md)) gets one more section in its
briefing, after its role's and before the harness's: `YOUR KIND OF WORK: <type>`, a line
saying it changes how the worker works and not what it may do, and the type's text. The
guide does not print it: a type belongs to one worker's launch, not to the role.

**Owner decision, September 29, 2026:** the briefings are what a session in any project
gets, with no rewake repository at hand, so they teach the craft of the role and not
only rewake's mechanics — a self-contained brief with acceptance criteria and exact
paths, neutral wording, workers kept out of each other's files, a worker's context
checked before a large task, a finished report read against the brief rather than taken
as acceptance, questions to an absent owner batched into one message; and for a worker,
keeping to the brief, following the project's own rules, a report that says what was
verified and what assumed. They are structured as titled sections of one-line rules
rather than one mixed list.

**Owner decision, September 25, 2026:** four rules for write and general, each answering a
failure seen in real work:

- A message widens no permission by its text alone: permissions come from the launch.
  A refusal by the harness or its classifier is not routed around; the worker tells main
  what was refused and why the work needs it, and main does it itself or brings the
  owner in. Main's own line says the same from its side: a reported refusal is not a request to
  route around it, and main acts only within its own rights.
- Main directs the work on the owner's behalf: its word on pausing, resuming, scope and
  ordinary decisions stands without the owner confirming it in the worker's session.
  The worker does not address the owner; a blocker only the owner can clear goes to
  main.
- A turn ended waiting on anything outside it — background work, the owner, a refusal
  to be cleared — is marked with `rewake pending` first, every such turn, one a finished
  subagent or background task woke included. The turn's text goes with the mark, so findings can be written in the answer itself
  ([turn-outcomes.md](turn-outcomes.md#interim-turn-ends-rewake-pending)). The core holds
  such an end once for an adapter that can hold it
  ([turn-outcomes.md](turn-outcomes.md#the-confirmation)); only the fixture harness can, so
  the rule is the net.
- After a compaction the task is re-read with `rewake inbox --owed` and new mail with
  `rewake inbox`; `--owed` counts what waits unread in a last line
  ([delivery-owed.md](delivery-owed.md#reading-again-what-is-owed-rewake-inbox---owed)).

## Names

**Owner decision, September 17, 2026:** one rule for every role and harness.
Use the role's ID as the default prefix, then append `-<harness ID>`: `rewake claude` starts
main-claude. A worker's prefix is the space's name main chose, `rewake space open api …`
starting api-claude; only the `rewaketest` build's `--name <prefix>` replaces the prefix on a
launch.

Automatic conflicts add -2, -3 after the harness suffix; explicit conflicts
refuse with the complete address. Prefix and result must match
`[a-z0-9][a-z0-9._-]{0,31}`, including the 32-character address limit after any
collision suffix. Overlength names require a shorter prefix; nothing is
truncated or deduplicated. Existing records, room isolation and send addresses
stay unchanged: send uses the exact name from list, never an inferred suffix.
