---
description: herdr, the terminal manager main may run in: how it finds an agent, its reporting command and its rules, and the live check of rewake's reports.
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
and adds the resume on the next tick; a failed report puts it back to state alone. Not verified: a
plain report from a shell outside the pane exited 0 yet seemed not to take the pane, which would tie
holding to the caller's process tree.
