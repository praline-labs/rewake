# A directory granted to a Claude Code session, stage 2 — September 27, 2026

Stage 1 ([entry](2026-09-27-grant-dir.md)) refused `--grant-dir` to a Claude Code
session. The owner settled the route the same day, after a live probe on 2.1.280
([research](../research-claude-actions.md#a-directory-given-to-a-running-session)):
the PermissionRequest hook in rewake's `--settings` layer. The stage was paused for a
grant a worker cannot forge ([entry](2026-09-27-grant-unforgeable.md)) and resumed on
that scheme.

## What was done

- **The hook.** `rewake grant-hook`, hidden, runs as a PreToolUse hook on the file tools
  and as a PermissionRequest hook, five seconds each. It reads nothing on disk: it
  hands the call, less any content, to its own session's wrapper and prints the answer.
  Any error or timeout is silence, never an allow.
- **The keeper.** The worker's wrapper keeps the grants it confirmed in memory
  (`grantauth.Keeper`, `@rewake/keep/<hash>`), answers only a process below itself and
  mirrors the journal to `grants/` for `rewake list`. The hook believes only the
  process its run names.
- **Delivery** of a grant task waits for the session to be idle, as on Codex.
- **Decisions** (`internal/harness/claude/permission.go`): a write inside a live grant is
  allowed with `addDirectories` for the grant's root; `.git`, `.claude`, `.codex` and
  `.agents` inside it are never allowed; after the task is settled, a read in the working
  directory is forced to a question whose answer carries `removeDirectories`, and a file
  tool writing into that directory is denied.
- The refusal of `--grant-dir` to Claude Code is gone; the help and main's briefing say
  that on Claude Code a grant spares prompts and draws no line.

- **The shielded part** is asked about before it is written: once the grant is a
  working directory the harness runs a file tool anywhere in it unasked, so PreToolUse
  answers `ask` for a file tool writing into `.git`, `.claude`, `.codex` or `.agents`
  inside a live grant. Main decided the same day that `--grant-git` does not change this.
- **The fixture** calls tools its mail names and passes them through the grant hooks the
  way the harness was seen to; its settings parser takes the grant hooks, which it had
  refused, so every Claude Code case would have failed at launch. The workflow case
  `claude-grant-dir` with three mutants: `claude-grant-silent`, `claude-grant-unshielded`,
  `claude-grant-kept` ([testing-cases.md](../testing-cases.md#a-directory-granted-with-a-task)).

## After review

review-claude's round on the stage, fixed the same day:

- **Medium: a command allowed by its suggestions.** The hook allowed a shell command when
  every directory the harness suggested lay in a grant, which rests on the harness naming
  every reason a compound command needs; that is not verified. Main decided the hook
  allows file tools only; a command gets the directory once a file tool has added it.
- **Taking back missed commands.** Until a read came, the directory stayed a working one
  and a command wrote there unasked. `Bash` joined the PreToolUse matcher: while a grant
  is being taken back every command is asked about, a plain `rewake` command's question
  answered with the removal and any other's left to the person. `grants.md` now names
  the modes where taking back works.
- **The hook read the registry** — and tidied it — on every tool call. It takes the
  session and run from the environment alone.
- **The keeper did not check namespaces**, as registration does; it does now, with a
  test from a helper in its own user namespace.
- `grants.md`: the idle wait holds only while telemetry says a turn runs.
- `docs/flow.md`, `docs/delivery.md` and `docs/research-codex.md` had grown past 400
  lines with stage 1 and are split by subject.

## What stays open

- A cold resume starts without the grant, while the owner's route restored it with
  `--add-dir`. Main decided on September 27, 2026 that the way is to have main's wrapper
  confirm the grant again, later.
- A shell command inside a granted directory reaches its `.git` unasked: Claude Code has
  no sandbox, and the hook cannot read what a command writes.
