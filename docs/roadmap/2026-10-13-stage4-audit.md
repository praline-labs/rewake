# Stage 4 of 2.0: the audit before the probe

Stage 4 opens with an audit of what the installed Claude Code and a mod already give, before
the probe and before any adapter code (the owner's decision of October 12, 2026). Written on
October 13, 2026, on Claude Code 2.1.296 with the `mods` skill of the same version, on branch
`s4-audit` from `v2` at `d0091d6`: [stage4-audit.md](../archive/v2/stage4-audit.md), its rows in
[stage4-audit-rows.md](../archive/v2/stage4-audit-rows.md) and
[stage4-audit-rows-space.md](../archive/v2/stage4-audit-rows-space.md), and the dated readings of pi
([stage4-reading-pi.md](../v2/stage4-reading-pi.md)) and Codex
([stage4-reading-codex.md](../archive/v2/stage4-reading-codex.md)) it folds in. Documents only: no
code changed, and the audit's writer ran nothing live.

## What the audit found

- **Twenty-two features weighed.** Kept as rewake's: the mail, the registry, the room view,
  the shell allowance, grant authority, the version gate, the scrubbing of children's
  variables, rewake's notices and the state directory as the only room state. Native
  messaging keeps nothing for a session that is not running and knows no kinds or reports,
  so it stays the wake carrier only. Built on the mod: reading by tool, the turn boundary,
  telemetry, control, the conversation binding and a guard that keeps a report off native
  messaging. Split: launch, grants, the worktree and the worker tabs. Open: joining (P11) and
  headless workers (P16, not scheduled).
- **The central proposal.** Work between Claude Code sessions through the mod alone, with
  `rewake claude` reduced to a launcher: the harness process anchors the run's ancestry, and
  a detached process the mod starts serves the run as endpoint, lease and final writer. That
  split loses the operator decision's same-process proof, so three authority obligations
  stay open — the caller-to-final-writer proof, a late opt-in after model commands ran, and
  the harness's death during an admitted decision. The wrapper stays for Claude Code unless a
  replacement authority contract is accepted; it stays for Codex in any case, and for pi
  until its own probe.
- **New probe rows** in [design-claude.md](../v2/design-claude.md#the-stage-4-probe): P11–P16
  and P12b, with P11 first and P12, P12b and P15 beside it.

## The owner's decisions it records

All of October 13, 2026, each written where it applies:

- pi is the next harness after Claude Code, as stage 4b; pi carries the models the room's
  Claude Code sessions do not already serve, and Claude Code's work stays on Claude Code
  sessions ([the stage plan](../v2/design.md#the-stage-plan-from-here)).
- Codex gets only an entry point before 2.0, a fully described catalogue entry offering
  Launch alone; its adapter comes after the release, and the release's mixed pair is Claude
  Code with pi.
- The space is stage 4's target: main creates a pseudo-terminal rewake owns, shown as a tab
  in its Claude Code, a worktree may be made in it, and a worker is seated there
  ([the target](../archive/v2/stage4-audit.md#the-target-a-space)); main may create spaces and seat
  workers in them, with the limits designed in stages 4 and 6.
- The worktree's lifecycle goes whole to Claude Code, even with another harness inside: a
  Claude Code worker makes its own, and a worker of another harness is seated in a checkout a
  Claude Code keeper holds, a background session or a subagent of main (P12b).
- The space's tab is the keeper subagent's entry in Claude Code's tasks list; opening it
  replaces the subagent's view with a pseudo-terminal in which the real harness runs with its
  own screen. The two levels of UI this makes are a proposal for stage 4's design.

Main's decisions of the same day are marked in the audit: P11 first, P15 beside stage 4's
probe, P16 unscheduled, and the shape of Codex's entry point.

## The review

One review on the Codex side over the whole branch found four blockers, all in the audit's
facts and its authority contract, and two notes; all were corrected in new commits and the
fix round was accepted at `feb770c` with no new blocker.

- **B1** — the join presented a moved anchor field as preserving the decision's authority;
  now one anchor/writer split with the three obligations named as P11's and the wrapper as
  the fallback.
- **B2** — the prompt hooks were called per request; both are cached, and their effect on a
  resumed briefing is left to P14.
- **B3** — Claude Code was said to lack an explicit session id; it has `--session-id`.
- **B4** — the Codex stub was said to launch a worker that takes no task; a reporting role is
  refused at launch, so the stub launches main alone.
- The notes: the mod's removal veto is conditional on P12, and the space's process has one
  name until P11 and P15 decide. After acceptance the rows document was split by subject
  under the 400-line rule, and the audit's "nothing was run live" names main's dated
  observation as the one live fact it cites.

## What stays open

- The probes: P11–P16 and P12b of stage 4, CX1–CX8 for Codex's adapter after the release,
  PI1–PI14 for stage 4b. None has run.
- The replacement authority contract, if the mod's join is to replace the wrapper for
  Claude Code; until one is accepted the wrapper stays.
- The space's UI between its two levels, and the choice of keeper, for stage 4's design
  after the probe.
