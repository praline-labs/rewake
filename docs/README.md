---
description: The reading order, the conventions every document keeps, and where each kind of document goes.
---

# The documentation

Each document says what it holds in its `description:` at the top; `SHIFT.md` gathers them into
a map. This file says what no description can: the order to read in, the conventions, and where a
new fact goes.

## The reading order

A person using rewake reads [user-guide.md](user-guide.md) alone. An agent working on rewake:

1. [project.md](project.md), then `GLOSSARY.md` at the root.
2. [flow.md](flow.md), then [flow-endings.md](flow-endings.md).
3. [design.md](design.md), then [code.md](code.md).
4. [roadmap/README.md](roadmap/README.md) — what is open.
5. Then by the work:
   - **mail and reports**: [delivery.md](delivery.md) → [delivery-turn-end.md](delivery-turn-end.md)
     → [turn-outcomes.md](turn-outcomes.md) → [delivery-owed.md](delivery-owed.md) →
     [delivery-sent.md](delivery-sent.md) → [mailbox-records.md](mailbox-records.md) →
     [rules/effects.md](rules/effects.md), [rules/turns.md](rules/turns.md);
   - **adding a harness, a role or a message kind**: [adding.md](adding.md);
   - **building a guard or a test of a failure**: [build-for-failures.md](build-for-failures.md);
   - **an adapter**: [rules/adapter-api.md](rules/adapter-api.md) → [claude.md](claude.md),
     [claude-turns.md](claude-turns.md) or [research-pi.md](research-pi.md) →
     [delivery-adapters.md](delivery-adapters.md) → the research records, with the `mods` skill
     for the mod;
   - **launch, spaces, the shell**: [launch.md](launch.md), [rules/launch.md](rules/launch.md) →
     [launch-defaults.md](launch-defaults.md)
     → [worker-profile.md](worker-profile.md) → [worker-types.md](worker-types.md) →
     [space.md](space.md) → [rules/space.md](rules/space.md) → [shell.md](shell.md) →
     [worktree.md](worktree.md), [worktree-tools.md](worktree-tools.md), [briefs.md](briefs.md),
     [research-herdr.md](research-herdr.md) for the terminal manager main may run in;
   - **main's view**: [session-state.md](session-state.md) → [remote-control.md](remote-control.md)
     → [permission-requests.md](permission-requests.md);
   - **state and the layout**: [rules/state.md](rules/state.md), [rules/layout.md](rules/layout.md);
   - **roles**: [roles.md](roles.md);
   - **tests**: [testing.md](testing.md) → [testing-cases.md](testing-cases.md),
     [legacy.md](legacy.md);
   - **installing and releasing**: [install.md](install.md);
   - **when something misbehaves**: [traps.md](traps.md).

## Where each kind of document goes

- **How rewake works** — a document per subject at the top of `docs/`, stating the system as it
  is: no step plans, no history of how it came to be, no annotations of what is no longer true;
  what is no longer true goes. An owner decision is a plain statement, dated, where it applies.
- **Rules** — [rules/](rules/README.md): the guarantees in full wording, numbered where a group
  is, independent of the code.
- **Facts about a harness** — the research documents (`research*.md`): dated records, each fact
  with the version and the way it was obtained, re-checked before an adapter is touched. Only
  they keep dates and observations.
- **What surprised** — [traps.md](traps.md), by symptom.
- **The open work and its order** — [roadmap/README.md](roadmap/README.md); a landed step leaves
  it, and its commit's subject is its record.
- **A design page** — for a step that changes the protocol or the state (`AGENTS.md`, "Delegation
  and review"), `roadmap/<step>.md`, named by the step's entry: what will be built, not yet true.
  When the step lands, what it settled goes into the living documents and the page leaves the
  roadmap. None is open now.
- **The project's goal** — [project.md](project.md), changed only by the owner's decision; the
  project's words in `GLOSSARY.md`; how the project is built and its checks in `AGENTS.md`.

A document stays under 400 lines, split by subject; a change that adds, removes or renames one,
or changes what it is for, updates its description and this file in the same commit.

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
