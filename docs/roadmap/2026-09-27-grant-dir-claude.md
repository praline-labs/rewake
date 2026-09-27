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

## What stays open

- The fixture learning the permission hooks, and a workflow case `claude-grant-dir` with
  mutants.
- A cold resume starts without the grant, while the owner's route restored it with
  `--add-dir`; a restore needs a source a worker cannot write.
- `.git` inside a grant stays with the person even beside `--grant-git`.
