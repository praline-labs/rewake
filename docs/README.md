# The documentation, mapped

New to the project: read [project.md](project.md) first; the order to start a session in is
in `AGENTS.md`.

One line per living document, grouped by what a reader wants from it. A subdirectory with an
index of its own is linked here rather than repeated: [rules/](rules/README.md),
[v2/](v2/README.md), [roadmap/](roadmap/README.md) and [archive/](archive/README.md). The map
is kept by hand: a change that adds, removes or renames a document, or changes what one is
for, updates this file in the same commit; no test checks it.

## Start here

- [project.md](project.md) — what rewake is, its principles, what the tool does and what to read next; changes only by the owner's decision.
- [flow.md](flow.md) — the whole path of one message, Acts 1–5: start, send, announce, read, report back.
- [flow-endings.md](flow-endings.md) — the rest of the flow: a blocking question, a notify, the session's end, where the flow can stall.
- [design.md](design.md) — the specification: scope, process model, state directory, rooms, environment, CLI contract, dated decisions.
- [roadmap/README.md](roadmap/README.md) — what is done and the order of work from here; the list of roadmap entries lives only there.
- [v2/README.md](v2/README.md) — rewake 2.0: the design, the stage plan, the living documents of stages 3 and 4.

## How it works

Sessions and launches:

- [session-record.md](session-record.md) — the session record: every field, the boot, how a record is published, updated and pruned.
- [roles.md](roles.md) — roles and names: the catalogue, what each role is told and reports, how a role and a name are chosen.
- [launch.md](launch.md) — the wrapper's launch sequence, signals, room and role flags, the harness flags, the intro.
- [launch-defaults.md](launch-defaults.md) — model and effort defaults nobody typed, launch aliases and which flags a typed one replaces; the worker catalogue and `space open --alias` ([the step](roadmap/2026-10-18-worker-aliases.md)).
- [worktree.md](worktree.md) — the checkout `--worktree` gives a launch, `.worktreeinclude`, and `rewake worktree land`, `finish`, `ls`, `rm`.
- [install.md](install.md) — building and installing locally, the npm dry run, the upgrade from 1.x, the release gate.

Mail and reports:

- [delivery.md](delivery.md) — sending and reading at the level of files: the message record, task, question, notify, the lock, the servicing loop.
- [delivery-turn-end.md](delivery-turn-end.md) — a finished turn becoming a report: the order of its steps, the report's derived id, what a retry finds.
- [delivery-owed.md](delivery-owed.md) — `rewake inbox --owed` and `--awaited`: what a session still owes and what a run still waits on.
- [delivery-sent.md](delivery-sent.md) — `rewake withdraw`, `rewake edit` and `send --to`: what a sender can still do with a sent message.
- [delivery-adapters.md](delivery-adapters.md) — how a notice reaches a harness: the Claude Code socket line and receipts, the pi extension, startup gates.
- [inbox-groups.md](inbox-groups.md) — grouped incoming mail: the session table, `--peek`, `--message <id>`, the window that fixes a group.
- [mailbox-records.md](mailbox-records.md) — every kind of file a mailbox holds, a reference kept by hand.
- [turn-outcomes.md](turn-outcomes.md) — turn ends that are not an ordinary report: a failed turn, a keyboard stop, `rewake pending`.
- [report-publication.md](report-publication.md) — capturing and publishing a finished turn's report, the sequence word, receipt keys.
- [turn-end-recovery.md](turn-end-recovery.md) — how a turn end is recovered: the operation's identity, the read clock, pending marks, what a retry finds.
- [turn-end-recovery-findings.md](turn-end-recovery-findings.md) — what acceptances and reviews of the turn-end journals found, a record of mechanics removed on October 18, 2026.

Main and its workers:

- [session-state.md](session-state.md) — the telemetry line main sees for its workers: context, model and effort, compactions, staleness.
- [v2/workers-panel.md](v2/workers-panel.md) — the workers' pane in main's Claude Code: the space host's view, the `workers/*` endpoints, the pane and its keys.
- [session-activity.md](session-activity.md) — activity labels, compaction notices, a worker's departure or replacement announced to main.
- [remote-control.md](remote-control.md) — `rewake compact` and `rewake interrupt`: who may call them, refusals, the control directory.
- [remote-control-letter.md](remote-control-letter.md) — the letter that ends a compaction main asked for and the record that owes it.
- [remote-control-tests.md](remote-control-tests.md) — the tests of `compact` and `interrupt`, file by file.

Designs not built:

- [permission-requests.md](permission-requests.md) — a worker's permission request or question carried to main, answered with `rewake answer`, the first answer winning.
- [permission-requests-sources.md](permission-requests-sources.md) — the live probe on Claude Code 2.1.302 and the pi facts behind it.
- [conversation-reset.md](conversation-reset.md) — `rewake clear` from main and a conversation marker in `list` and notices.

The rules and the code:

- [rules/README.md](rules/README.md) — the numbered rules of 2.0 in full wording, a reference of behaviour, by group.
- [code.md](code.md) — the source tree package by package, with the harness interface every adapter implements.
- [legacy.md](legacy.md) — the mark on code kept for an old harness version or old records, and the table of current marks.

## Facts about the harnesses

Each fact carries where it was verified; they age with harness versions.

- [research.md](research.md) — what only a running Claude Code session shows: the socket line, waking an idle session, the inbound gate.
- [research-claude-control.md](research-claude-control.md) — what reaches a running Claude Code session from outside besides a message.
- [research-claude-parity.md](research-claude-parity.md) — context, compaction and interrupt through the mod: checks on 2.1.302 and live acceptance still open.
- [research-claude-actions.md](research-claude-actions.md) — what a function-hooks plugin can do to its own session: compact, abort, reload.
- [research-claude-mod.md](research-claude-mod.md) — the dated records of stage 4's live probe of the mod on Claude Code.
- [research-pi.md](research-pi.md) — pi 1.2.0 launch and extension facts, mock probes, telemetry and control, the adapter protocol and its limits.
- [research-launch.md](research-launch.md) — what an installed binary answers: model and effort catalogues, repeatable flags, npm packages.
- [research-worktree.md](research-worktree.md) — Claude Code's own worktree beside rewake's: continuations, trust, `.worktreeinclude`.
- [prior-art.md](prior-art.md) — how other tools deliver messages between agents, what to borrow and what to avoid.

## Testing

- [testing.md](testing.md) — the entry point: the tiers, the full set before a publication, reading a summary and a red case, adding a scenario or a column.
- [testing-cases.md](testing-cases.md) — what each end-to-end case claims, with its sessions, the fixture's play and its controls.
- [testing-pool.md](testing-pool.md) — how the workflow suite runs cases side by side: `-parallel`, the pool, owner labels, the sweep.
- [check-runner.md](check-runner.md) — the requirements the workflow suite answers: one command, evidence tiers, the output contract.

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
