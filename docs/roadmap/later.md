# Later, as needed

- pi, opencode, grok — their delivery paths are already covered in
  `docs/research.md`.
- A busy/idle signal for the recipient, and choosing delivery priority from it.
- A status column in `list` sourced from the Claude Code registry.
- macOS: replacements for `/proc` (`lsof`, `ps -o lstart`).
- Read receipts: `rewake inbox` already writes a `read` status; `send` does not
  report it yet.
- Permissions on request (owner idea, September 17, 2026): "grant permissions
  for actions on request — say review cannot reach a folder in /tmp", and
  "restrict the worker and grant it rights dynamically". The mechanism already
  exists for Git metadata: `turn/start.runtimeWorkspaceRoots` travels with a
  delivered task. A general form would be `rewake send <name> --grant <path>`,
  accepted only from the main role, with paths checked (existing, no symlink
  escape) and the thread's roots only ever extended; a general session would
  start with its working directory alone. Limits: Codex through its app-server
  only (Claude Code needs its own research), a turn the person starts in the
  TUI uses the stored roots, and the sandbox has to be workspace-write.
