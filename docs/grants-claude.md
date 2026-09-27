# A directory granted to a Claude Code session

How a directory grant ([grants.md](grants.md)) reaches a Claude Code session, which has
no sandbox to add a root to: through its permission hooks, sparing the worker prompts
rather than fencing it in.

## The hooks

Nothing adds a working directory to a running Claude Code session from outside, but its
permission hooks can ([research-claude-actions.md](research-claude-actions.md#a-directory-given-to-a-running-session)).
Rewake's settings layer adds `rewake grant-hook` as a PreToolUse hook on the file tools
and `Bash`, and as a PermissionRequest hook, each with a five-second timeout. The command
carries `--rewake-allowed` when the launch added its own `Bash(rewake:*)` rule, which it
does only without the person's own `--allowedTools` ([launch.md](launch.md#claude-code)).

- **Where the grant lives.** Once the grant is confirmed ([grants-authority.md](grants-authority.md)),
  the recipient's wrapper keeps it in memory. The hook holds nothing: it is a child of
  the harness, so it runs below that wrapper, and it asks the wrapper over an abstract
  socket named from the room, the session and its run (`@rewake/keep/<hash>`), taking
  the session and the run from the environment the wrapper gave the harness. The wrapper
  answers only a process below itself and in its namespaces, checked as registration
  is; the hook believes only the process its run names. It sends the call's path fields
  and a command, never the content a Write carries. No record in the state directory
  decides anything, since a worker can write one; the copy under `grants/` is only what
  `rewake list` shows.
- **Delivery waits for an idle session** while its telemetry says a turn is running:
  a grant task stays pending, retried every two seconds, as on Codex, since a grant that
  arrived mid-turn would be taken back by a report the turn had not made. Telemetry that
  has gone stale counts as idle.
- **Giving.** Only a file tool is allowed — Write, Edit, MultiEdit, NotebookEdit —
  judged by its path. When it first writes inside a grant, the PermissionRequest hook
  answers allow and adds the grant's root as a working directory for the session. A
  shell command is never allowed (main's decision of September 27, 2026): an approved
  command runs whole, and whether the harness names every reason a compound one needs
  — `&&`, `;`, a redirection, `$(...)` — in what it suggests is not known. So a command
  inside a grant goes to the person until a file tool has written there, and runs
  unasked after, the directory then being a working one.
- **What stays with the person.** `.git`, `.claude`, `.codex` and `.agents` at any depth
  inside a grant are never allowed, `--grant-git` or not (main's decision of September
  27, 2026): whatever reads them next runs what the worker wrote, and on Claude Code
  nothing but the prompt stands in the way. Once the grant is a working directory the
  harness would run a file tool anywhere in it unasked, so PreToolUse answers `ask` for a
  file tool writing there; a shell command inside the grant reaches them unasked all the
  same. A write outside every grant hears silence, and silence is also the answer to any
  error or timeout: the hook never allows on a guess.
- **Taking back** happens in `default` and `acceptEdits` only. After the task is settled,
  PreToolUse answers `ask` on the first read inside the working directory, and on every
  shell command, while a grant is being taken back. The PermissionRequest that follows
  a read, or a plain `rewake` command with nothing but words after it, is allowed with
  the grant's root removed: those run unasked anyway, so the answer approves nothing new.
  A plain `rewake` command counts only under the rule the launch added
  (`--rewake-allowed`); where the person gave their own `--allowedTools`, it may be one
  they are asked about, and it goes to them like any other command. Any other command's
  PermissionRequest hears silence: the person is asked, and the directory is not taken
  out even if they approve. So until the first read or plain `rewake` command, every
  shell command asks the person; for a worker under orchestration the first
  `rewake inbox` of its next turn ends that. A file tool writing into a directory being
  taken back is denied with a line naming why.
  A directory another live task holds stays. A grant never used — never added — ends at
  the report. In any other mode — `plan`, `bypassPermissions`, auto — nothing is forced,
  and a grant once added stays until the session ends.
- **After a cold resume** the new run's wrapper asks main to confirm the grant again and
  starts the harness with the directory ([grants-resume.md](grants-resume.md)).

## A courtesy, not a boundary

This is a courtesy, not a boundary. Claude Code runs approved shell commands as the user,
so a worker writes wherever the user can if a prompt is approved, and can forge a grant
([grants-authority.md](grants-authority.md)). What the hook adds is fewer prompts inside the
directory main named, and a prompt again once the task is done. What the worker's own
`rewake` rule runs unasked includes no launch: a harness started from inside a session
is refused ([launch.md](launch.md#no-session-inside-a-session)).
