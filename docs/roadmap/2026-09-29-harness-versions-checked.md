# Codex 0.159.0 and Claude Code 2.1.284 checked before the update

On September 29, 2026, before the owner updated from Codex 0.155.1 and Claude Code 2.1.280,
both new versions were checked without a model run: the workflow suite against the
protocol schema generated from Codex 0.159.0, a source diff of Codex `rust-v0.157.1..
rust-v0.159.0`, and a comparison of the Claude Code 2.1.280 and 2.1.284 bundles. Both
were found safe to update with no change in rewake.

## Codex 0.159.0

- The workflow suite with `REWAKE_CODEX_VERSION=0.159.0`: 137 scenarios, 163 cases, 160
  pass, 3 unsupported as before.
- From the source: delivery through `turn/start` (start or steer), the compaction refusal
  text rewake matches (`ActiveTurnNotSteerable { turn_kind: Compact }`, formatted in
  `turn_processor.rs:679`), `thread/compact/start`, `turn/interrupt`,
  `runtimeWorkspaceRoots`, approval requests and `serverRequest/resolved`, token usage and
  settings events, and the TUI's thread start and resume are unchanged for rewake.
  `turn/completed` with status `interrupted` may now carry `error`; reconnects retry every
  8 s up to 120 s; the sandbox now also protects `.aws` and a resolved Git directory inside
  another writable root, while an explicit grant of that exact path still applies.
- One new path rewake does not follow: the TUI keeps an empty remote conversation after
  startup or `/new`, and returning to it through the agents overview does `thread/read`
  and swaps the view locally, without `thread/resume` (`tui/src/app/session_lifecycle.rs`,
  `app/agents_overview.rs`). The gateway treats a read of another thread as no selection
  and lets the binding go, so delivery waits until `/new` or an explicit resume. Taking
  any `thread/read` for a selection would be wrong.

## Claude Code 2.1.284

- The function-hooks host and the calls rewake's plugin makes, the settings layer's hooks
  and their input and output fields, the inbox socket (priority, `skipSlashCommands`,
  `isMeta`, the delivery receipts) and the launch flags rewake passes or refuses are
  unchanged. `--help` differs by `--agents` taking a file with `--print` and a new
  `--client-data-url`.
- New: on a cold resume Claude Code may ask "Resume this conversation?" — how long the
  conversation was idle, its tokens and the share of the five-hour limit a resume would
  take — with Resume and Start a new conversation. Its switch is server-side and it shows
  only on a limited plan with a cold cache and a share of 5% or more. Start a new
  conversation runs `/clear`. The dialog is modal: an unattended worker resumed with
  `--resume` waits on it, which belongs with the stalled-prompt design
  ([permission-requests.md](../permission-requests.md)), and a grant restored through
  `--add-dir` at such a launch may land in a new conversation.
- A new refusal of `--append-system-prompt` beside `--append-system-prompt-file` applies
  only to the carrier process a Remote Control daemon spawns, which `rewake claude` never is.

## After the update

The owner updated both and restarted the workers on rewake 1.0.3 the same evening. A task
sent to a Claude Code 2.1.284 write session through its inbox socket and to a Codex 0.159.0
session through the app-server was delivered and reported back on both; each named its
harness version and the sections of the 1.0.3 briefing it was launched with, in order, and
quoted a rule from it.

## What stays open

- The rest of the live check after the update: a mid-turn addition, `/new` then an
  explicit resume on Codex, compaction and interrupt on both, a directory grant and Git
  metadata on Codex, the plugin loaded and telemetry in `rewake list` on Claude Code, and a
  `--resume` of an old worker to see whether the dialog shows.
