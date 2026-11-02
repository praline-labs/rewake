# Changelog

What each release of rewake changed, by part: `rewake`, `shell`, `face`, a harness rewake ships,
or `docs`. `rewake update` shows the entries between the release a room runs and the newest one;
what a release restarts is computed from its manifest, never read from here
([docs/install.md](docs/install.md#updating-from-inside-rewake)).

## 3.0.0

### rewake
- `rewake update` shows a newer release on npm, its changes by part and what installing it
  restarts, and `rewake update <version>` installs exactly that release; installing restarts
  nothing.
- Every install lists its parts in a manifest beside it; `rewake restart` with no part says what
  the room must restart to run what is installed, and `rewake restart shell` or `face` restarts
  one part while main and the workers run on.
- Main's pseudo-terminal is held by a host in the person's pane, so a restart or one crash of the
  shell leaves main and its workers running.
- `rewake restart rewake` moves a running room to a newly installed rewake without ending a
  session: main's host, the space hosts and every wrapper exec the new build, keeping their
  processes, terminals and mail. Run from main's own turn, the switch waits until every session
  is idle, for up to ten minutes, and its outcome comes to main as a note. A new rewake that
  changes a contract the room holds still needs the room's restart, and the plan says which.
- A session's mail is one handle in the command line: reading the inbox, marking a turn pending,
  sending, editing and withdrawing a letter each take their mailbox lock themselves.
- A worker and what it starts carry none of main's herdr variables.
- Every Claude Code run rewake starts has the person's own status line turned off.
- The mod reports a session's directory and the account's 5h and 7d limits.

### shell
- The hints row shows a newer release on npm and what installing it restarts, then which restart
  the installed parts still need.
- Every pane has a top row with where its session works now and its branch; main's strip shows
  the account's limits; the context stands at each strip's right edge.
- A run's room work — its briefs with their ticks and the reports it awaits — shows at its input.
- The bar's briefs button stands at its right edge; only main's title goes to the outer terminal.

### face
- Every pi worker is drawn by rewake's face, and a dev worker on Claude Code can be.
- The face draws as Claude Code does: styled spans, folded runs of reads and commands, edits
  marked, tables boxed, a working line with phase and tokens, thinking faint and italic.
- Its input is pi's editor: prompts queued while pi works, pastes masked and kept across a shell
  restart, Esc stops a pi worker's run.
- It scrolls as a native pane does, copies by a drag or a click through OSC 52, and opens folds
  with ctrl+o or a click.
- A pi worker's compaction and its retries show on the turn line.
- A reload keeps the running face when the new one speaks another contract until the new one
  answers.

### docs
- docs/install.md: the parts, the plan, and updating from inside rewake.
