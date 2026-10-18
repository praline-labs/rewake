# The harness version a launch takes

Part of the mail tool's launch ([mail-bridge-launch.md](mail-bridge-launch.md)): a gate
is closed in code only for the harness versions its check ran against
([gates](mail-bridge-launch.md#gates)), and the output limits L5 calibrated hold per
version too, so a launch needs its harness's version. This document says when the
launch takes it, from where, and what an unknown or unconfirmed one means. Codex's
confirmation at start is in [mail-bridge-launch-codex.md](mail-bridge-launch-codex.md#the-version-confirmed-at-start).

*Revised after the live checks, October 4, 2026, on main's direction, and built the same
day.* Before the revision every launch of a harness the table names ran `<program>
--version` before the claim, `--no-mail-tool` included, so a `--command` wrapper ran
once more than the harness needed (`wrapped-launch` counted it). The rule:

- **The version is taken only where a closed gate could change the choice.** It is not
  taken under `--no-mail-tool`, nor where the launch's rule 4 already leaves the tool out (`-w`,
  `--bare`, an unmergeable `--settings`, a managed MCP file), nor for a harness the table
  names no version of: the gates are then not consulted at all. Codex's transport read
  below still runs on every launch, as it did before the table existed.
- **No program is run only to learn it.** The launch takes a version it already
  observes, or none.
  - *Codex*: the launch ran `<program> --version` once for the transport's
    compatibility note at the server's start. That one read is now taken before the
    claim, bounded and reaped as a check, and serves both; the server's start uses its result and runs
    nothing more. The owned app-server's `initialize` answer carries the version too, in
    `userAgent` (*live*, 0.159.0), and the startup probe compares the two before the
    gateway listens; what each outcome does to the injection, the gates and the L5 bound
    is in [the version confirmed at
    start](mail-bridge-launch-codex.md#the-version-confirmed-at-start).
  - *Claude Code*: the version is read from the path of the `claude` found on the
    launch's `PATH`, resolved without running it: the native installer's
    `…/claude/versions/<version>` (*live*, 2.1.284). The same holds under a `--command`
    wrapper (the owner's decision of October 4, 2026): the owner runs one Claude Code
    version for all sessions, and a wrapper that starts a different binary is not a
    supported setup, so nothing after the start compares the two. The version is unknown
    only when `PATH` holds no `claude` or its resolved path names no version, as an npm
    install's does.
- **A read that gives no version is unknown only once its cleanup is proven.** Codex's
  read is a check under the launch's rule 6 ([the rules](mail-bridge-launch.md#the-rules)).
  No answer, an exit other than 0, no version in the
  output or the bound passing give an unknown version once the holder has confirmed
  that every process of its tree ended and was reaped; a process that cannot be ended,
  or a holder lost or unable to say how its tree ended, refuses the launch under rule 6,
  before the claim, naming the check `--version` and its outcome class. The read
  answers one of three — a version, unknown after cleanup, cleanup failed — never an
  empty string for all of them.
- **An unknown version is no version the table names.** Every gate is open for that
  launch, apart from those `REWAKE_GATES_ASSUMED` names, and each takes its action: on
  Codex no tool (G2), on Claude Code no tool (G7). A launch note names the cause —
  `harness version unknown`, with `no claude on PATH`, `path names no version` or `not
  read` — and the channel starts as "no tool" with it. The unknown version itself is
  never a refusal; only rule 6 is.
- **The bounds are per version too.** The smallest output limits L5 calibrated sit in a
  table beside `closedGates`, by harness and version; a version without an entry has no
  bound, and any limit set then counts as lowered ([the output
  limit](mail-bridge-launch-codex.md#the-output-limit); on Claude Code, [the limits per call](mail-bridge-launch.md#claude-code)).

## As built

- `harness.ReadVersion` (`internal/harness/gates_version.go`) is the read under rule 6:
  it answers a `Version` — a value, or the cause of an unknown one — or the check's
  `CheckFailedError` with label `--version` and outcome `not ended`, which the launch
  returns before the claim. A check that could not start and whose holder cannot show
  its tree ended counts as not ended (`ErrCheckNotEnded`), not as unknown.
- Codex is the one `harness.LaunchVersionReader`. `chooseTool` (`internal/wrap/mailtool_launch.go`)
  reads first, `--no-mail-tool` included, and hands the version to the check and to the
  plan (`LaunchRequest.Version`); the owned server's start takes its compatibility note
  from it and runs nothing (`codex/server_version.go`).
- `harness.ClaudeVersion` resolves the first executable `claude` on the launch's `PATH`
  and reads the version from `…/claude/versions/<version>`. The Claude Code check takes
  it only after `-w`, `--bare`, `--settings` and the managed MCP file have had their say
  (`claude/mailtool_check.go`), so a launch they leave out reads nothing.
- The bounds are `outputBounds` beside `closedGates` (`internal/harness/gates.go`);
  `Gates.OutputBound` answers the launch's. Codex's injection check takes a set
  `tool_output_token_limit` against it (`readsOffLimit`, `readsOffNoBound`); Claude
  Code's endpoint takes `MAX_MCP_OUTPUT_TOKENS` against it (`endpoint.Config.OutputBound`).
- Tests: `harness/gates_version_test.go` (the three answers, a holder lost mid-read, the
  path forms, gates and bounds per version), `wrap/mailtool_version_test.go` (one read,
  `--no-mail-tool` included; not ended refuses before the claim), the G7 dimension of
  `claude/mailtool_check_test.go`, and the `wrapped-launch` scenario, which counts three
  wrapper runs on Codex and one on Claude Code.
