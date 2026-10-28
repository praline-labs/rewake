---
description: herdr, the terminal manager main may run in: how it finds an agent, its reporting command and its rules, what makes it drop a report, and the live checks of rewake's reports.
---

# Research: herdr

herdr is a terminal workspace manager for coding agents; the owner runs rewake inside it. What
rewake does with it is in [launch.md](launch.md#herdr). These are the facts it relies on, each
with the version and how it was obtained.

## How herdr finds an agent

Checked October 27, 2026 on herdr 0.9.5 (`herdr --version`), live:

- herdr detects an agent by its pane's foreground process. A pane running main in rewake's shell
  has `rewake-shell` as its foreground process, with the harness inside the shell's own pty.
  `herdr pane process-info` shows only `rewake-shell`, and the pane is not detected; a plain
  harness in the next pane is.
- Every process in a pane inherits `HERDR_ENV=1`, `HERDR_PANE_ID`, `HERDR_BIN_PATH`,
  `HERDR_SOCKET_PATH`, `HERDR_TAB_ID` and `HERDR_WORKSPACE_ID`. A worker's wrapper, under a space
  host outside main's process tree, still carries them: it inherits main's environment.

## The reporting command

From herdr's page "Add Herdr support to your agent"
(`docs/next/website/src/content/docs/add-herdr-support.mdx` at tag `v0.9.5`), its Socket API
section (`socket-api.mdx`, "Agent state reporting"), and `herdr pane report-agent --help`, read
October 27, 2026:

- `"$HERDR_BIN_PATH" pane report-agent <pane> --source <id> --agent <label> --state
  idle|working|blocked|unknown [--message <text>] [--seq <n>] [--agent-session-id <id>] [--
  <resume argv>]`. `pane release-agent <pane> --source --agent [--seq]` gives the pane up.
  The page calls the CLI the portable choice; the socket's `pane.report_agent` is equivalent.
- `--seq` must rise with every report of a source, across restarts. herdr ignores a report whose
  number is not above the last it accepted. The page suggests a timestamp.
- A resume command's first word is a plain command name on PATH, not a path. No argument may
  hold an apostrophe or a control character, and the whole is at most 64 arguments and 8 KiB.
  A command that breaks these rules fails the report with `invalid_resume_argv`, and the report
  is not applied. The source must hold the pane first, by a state report accepted earlier;
  otherwise herdr answers `resume_not_accepted`. A state report in the same call does not hold
  it (checked on 0.9.5, below). Resume commands need herdr 0.9.2 or later.
- A source starting with `herdr:` is reserved for herdr's own integrations.
- herdr clears a pane whose agent never released it once the pane is back at an idle shell
  prompt, a second or two later; the page calls this a fallback, not a replacement for release.

## Live check (October 27, 2026, herdr 0.9.5)

Setup: a scratch main of this change in a new herdr workspace, closed after; its own
`REWAKE_DIR` under `/tmp`; the cheapest model; the build first on PATH as `rewake`.
`HERDR_BIN_PATH` pointed at a shim that logged each call with its caller's pid before running
herdr.

- `herdr agent list` showed the pane as the harness's id. The state read `unknown` until the
  first telemetry, then `idle`; one prompt took it to `working` and back. herdr shows an idle
  agent that finished in an unfocused pane as `done`.
- herdr's `session.json` kept the pane's `agent_resume` as `{"source": "rewake", "argv":
  ["rewake", "<harness>", "--resume", "<id>"]}`. `pane get` and `agent get` do not show it for
  a custom source in this version.
- With a worker raised through `rewake space open`, every logged call still came from main's
  wrapper: the worker, carrying every `HERDR_*` variable, made none.
- On `/exit`, main's wrapper ran `release-agent`, and herdr cleared the pane within a quarter of
  a second.
- Restore after a herdr server restart was not tried: it would stop the owner's session.

## A resume in the first report (October 28, 2026, herdr 0.9.5)

A main started with `--resume` knew its conversation by the first tick, so its first report
carried a resume command, and herdr refused it and every report after it. The pane was never
held, `pane get` showed no agent and the state `unknown`, and herdr's log had `pane.report_agent`
failing with `outcome="error"` every five seconds. The same call by hand:

```
herdr pane report-agent <pane> --source rewake --agent claude --state idle --seq <µs> \
  --agent-session-id <id> -- rewake claude --resume <id>
{"error":{"code":"resume_not_accepted","message":"resume_argv requires the reporter to hold the pane; report its state with pane.report_agent first"}}
```

So a report with a resume does not hold the pane itself: the state report must have been accepted
before. `internal/host/herdr` therefore reports state alone until one report of the run is accepted,
and adds the resume on the next tick; a failed report puts it back to state alone. The plain report
that exited 0 yet did not take the pane is the next section: a record of the pane, not the caller's
process.

## What makes herdr drop a report (October 28, 2026, herdr 0.9.5)

A main in rewake's shell in pane `wF:p1` was never held: each state report exited 0 and the pane
stayed without an agent, and the resume after it failed with `resume_not_accepted`, every five
seconds. Read in herdr's source at tag `v0.9.5` (`src/app/api/panes.rs`, `handle_pane_report_agent`;
`src/terminal/state.rs`, `set_hook_authority_at`; `src/platform/unix_common.rs`) and probed live in a
herdr workspace of the probe's own, closed after:

- **The caller does not matter.** herdr reads the calling process's id from the socket only for its
  log. A report made from a process of another pane's terminal took a fresh pane: `pane get` showed
  `claude idle`, then `claude working`.
- **A recorded exit drops the label.** When herdr sees an agent's process leave a pane's foreground,
  it records that agent's exit for the pane. From then on a state report from any source but herdr's
  own integrations whose `--agent` names that agent (`claude`, and `pi` the same) is dropped without
  an error: the command exits 0, the pane does not change, and a resume in the next report fails with
  `resume_not_accepted`, since nothing holds the pane. The record has no expiry; it is cleared only
  when herdr sees that agent's process in the pane's foreground again, or the pane is respawned. In
  rewake's shell the foreground is `rewake-shell` and the harness runs on the shell's own pty, so
  herdr never sees it, and the pane stays refused as long as it lives. Reproduced: a process named
  `claude` ran in the probe pane and exited, and then rewake's own two calls gave exactly `wF:p1`'s
  answers.
- **What leaves the record.** herdr matches a process by its name, so neither rewake's launcher nor
  its shell is taken for the harness: the record comes from the harness itself run plainly in that
  pane before, as a person may do before starting rewake there. herdr keeps no readable history of a
  pane, so `wF:p1`'s earlier run is inferred, not seen. The live check of October 27 used a new
  workspace, which is why its reports were taken.
- **A label herdr does not know is taken.** In the same refused pane a report as `rewake-claude` held
  the pane, its resume was accepted, and `pane report-metadata --source rewake --agent rewake-claude
  --display-agent claude` made herdr show `claude`: `agent list` gave `agent: rewake-claude`,
  `display_agent: claude`. herdr frees a pane held by a label it does not know once the pane is back
  at its shell prompt.
- Other drops in the same function, not met here: a report naming a known agent other than the one
  herdr detects in the pane, and one from a source other than the owner of the pane's current
  session.

So rewake reports as `rewake-<harness>` and names the harness through `--display-agent` (main's
decision, October 28, 2026; [launch.md](launch.md#herdr)): one path that no record of herdr's own
detection can refuse. A restore after a herdr restart plans a saved resume command whatever its
agent's name (`src/persist/restore.rs`, read, not tried).

Checked live the same day, in the probe's own workspace, closed after: the pane was first left
refusing `claude` (a process of that name ran and exited, and a `claude` report was dropped), then a
main of this change was started in rewake's shell there, with its own `REWAKE_DIR` under `/tmp` and
the cheapest model. The shim on `HERDR_BIN_PATH` logged, every call exiting 0:

```
pane report-agent wH:p1 --source rewake --agent rewake-claude --state unknown --seq …
pane report-metadata wH:p1 --source rewake --agent rewake-claude --display-agent claude --seq …
pane report-agent wH:p1 --source rewake --agent rewake-claude --state idle --seq … --agent-session-id <id> -- rewake claude --resume <id>
pane report-agent … --state working … -- rewake claude --resume <id>
pane report-agent … --state idle … -- rewake claude --resume <id>
pane release-agent wH:p1 --source rewake --agent rewake-claude --seq …
```

`herdr agent list` showed one agent in the pane throughout, `rewake-claude` shown as `claude`: idle,
working through one prompt while Claude Code's spinner title ran, then `done`. herdr's own title rule
showed no second agent, and `agent explain` found no detected agent there. `session.json` kept
`agent_resume` as `{"source": "rewake", "agent": "rewake-claude", "argv": ["rewake", "claude",
"--resume", "<id>"]}`. On `/exit` the pane was cleared within half a second.
