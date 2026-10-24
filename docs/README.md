# The documentation, mapped

New to the project: read [project.md](project.md) first; the order to start a session in is
in `AGENTS.md`.

One line per living document, grouped by what a reader wants from it. A subdirectory with an
index of its own is linked here rather than repeated: [rules/](rules/README.md),
[roadmap/](roadmap/README.md) and [archive/](archive/README.md). The map
is kept by hand: a change that adds, removes or renames a document, or changes what one is
for, updates this file in the same commit; no test checks it.

## Start here

- [project.md](project.md) — what rewake is, its principles, what the tool does and what to read next; changes only by the owner's decision.
- [flow.md](flow.md) — the whole path of one message, Acts 1–5: start, send, announce, read, report back.
- [flow-endings.md](flow-endings.md) — the rest of the flow: a blocking question, a notify, the session's end, where the flow can stall.
- [design.md](design.md) — the specification: scope, process model, state directory, rooms, environment, CLI contract, dated decisions.
- [roadmap/README.md](roadmap/README.md) — what is done and the order of work from here; the list of roadmap entries lives only there.

## How it works

Sessions and launches:

- [design.md](design.md#session-record) — the session record: every field, the boot, how a record is published, updated and pruned.
- [roles.md](roles.md) — roles and names: the catalogue, what each role is told and reports, how a role and a name are chosen.
- [launch.md](launch.md) — the wrapper's launch sequence, signals, main in the room of its directory, the harness flags, the intro.
- [launch-defaults.md](launch-defaults.md) — model and effort defaults nobody typed; the worker aliases, main's catalogue for `space open --alias`, and which flags a typed one replaces ([the step](roadmap/2026-10-19-worker-catalogue.md)).
- [worker-types.md](worker-types.md) — the kinds of work a worker is raised for: the built-in, user and project types, `[types] off`, `space open --type`, the briefing's section and the memory by type; a project's `.rewake/` ([the step](roadmap/2026-10-21-worker-types.md)).
- [worktree.md](worktree.md) — the checkout `--worktree` gives a launch, `.worktreeinclude`, and `rewake worktree land`, `finish`, `ls`, `rm`.
- [worktree-tools.md](worktree-tools.md) — the worktree tools of both harnesses: a template's `worktree = true`, `rewake worktree status` and its close forecast, the `worktree_status` tool, main's `worktree_list` and `worktree_land`, and a Claude Code worker's entry: `enter_worktree = true`, `rewake worktree enter`, `worktree_enter` and its guard.
- [install.md](install.md) — building and installing locally, the npm dry run, the upgrade from 1.x, the release gate.

Mail and reports:

- [delivery.md](delivery.md) — sending and reading at the level of files: the message record, task, question, notify, the lock, the servicing loop.
- [delivery-turn-end.md](delivery-turn-end.md) — a finished turn becoming a report: the order of its steps, the report's derived id, what a retry finds.
- [delivery-owed.md](delivery-owed.md) — `rewake inbox --owed` and `--awaited`: what a session still owes and what a run still waits on.
- [delivery-sent.md](delivery-sent.md) — `rewake withdraw`, `rewake edit` and `send --to`: what a sender can still do with a sent message.
- [delivery-adapters.md](delivery-adapters.md) — how a notice reaches a harness: the Claude Code socket line and receipts, the pi extension, startup gates.
- [mailbox-records.md](mailbox-records.md) — every kind of file a mailbox holds, a reference kept by hand.
- [turn-outcomes.md](turn-outcomes.md) — turn ends that are not an ordinary report: a failed turn, a keyboard stop, `rewake pending`.

Main and its workers:

- [session-state.md](session-state.md) — the telemetry line main sees for its workers: context, model and effort, compactions, staleness.
- [worker-profile.md](worker-profile.md) — the service folder a Claude Code worker runs in: its login, onboarding, skills, plugins, `settings.json`, the run's `--settings`, the memory by type.
- [shell.md](shell.md) — the shell: a TypeScript application on OpenTUI and Solid in `shell/`, the screen of every main started at a terminal, a pane per program fed the raw stream, the space host's emulator.
- [space.md](space.md) — a space: main starting a worker through a detached space host, its record and states, the listing, closing and restore.
- [remote-control.md](remote-control.md) — `rewake compact` and `rewake interrupt`: who may call them, refusals, the control directory.

Designs not built:

- [permission-requests.md](permission-requests.md) — a worker's permission request or question carried to main, answered with `rewake answer`, the first answer winning.

The rules and the code:

- [rules/README.md](rules/README.md) — the numbered rules of 2.0 in full wording, a reference of behaviour, by group.
- [code.md](code.md) — the source tree package by package, with the harness interface every adapter implements.
- [legacy.md](legacy.md) — the mark on code kept for an old harness version or old records, and the table of current marks.

## Facts about the harnesses

Each fact carries where it was verified; they age with harness versions.

- [research.md](research.md) — what only a running Claude Code session shows: the socket line, waking an idle session, the inbound gate.
- [research-claude-mod.md](research-claude-mod.md) — the dated records of stage 4's live probe of the mod on Claude Code.
- [research-pi.md](research-pi.md) — pi 1.2.0 launch and extension facts, mock probes, telemetry and control, the adapter protocol and its limits.
- [research-launch.md](research-launch.md) — what an installed binary answers: model and effort catalogues, repeatable flags, npm packages.
- [research-worktree.md](research-worktree.md) — Claude Code's own worktree beside rewake's: continuations, trust, `.worktreeinclude`.
- [prior-art.md](prior-art.md) — how other tools deliver messages between agents, what to borrow and what to avoid.

## Testing

- [testing.md](testing.md) — the entry point: the tiers, the full set before a publication, reading a summary and a red case, adding a scenario or a column.
- [testing-cases.md](testing-cases.md) — what each end-to-end case claims, with its sessions, the fixture's play and its controls.

## When something behaves unexpectedly

- [traps.md](traps.md) — what behaves other than expected, by symptom, before assuming a new defect.

## History

- [archive/README.md](archive/README.md) — the records of 1.x, the closed stages of 2.0, old reviews and research; bytes kept as they were.

## Names in records and addresses

No model or provider is named in code, configuration or text that explains how the system
works: that keeps the project from depending on any one of them. Two kinds of text are outside
that rule. A verbatim record of an observation is never edited afterwards, so it keeps the
names it saw — a model's, for one; striking a name would make the evidence untrue and leave an
acceptance document empty. And an artifact address — the registry name of a harness package,
the path of the binary inside it, the command that starts it — is what the code fetches and
runs, so it stays exactly as it is; replaced by a variable, the same string only moves to
wherever the variable is set. A model or provider name passed as a value inside such a command
or example is not an address, and the ordinary rule applies to it.
