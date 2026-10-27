# Glossary

The words this project gives a meaning of its own. One article per term; a new term gets its
article when it settles, a changed meaning rewrites its article.

## Room
Variants: rooms
Means: the sessions started from one launch directory: one main and the workers it raised.
Where: the room's state lives under `v2/rooms/<room>` of the state directory.

## main
Means: the orchestrator session of a room: it hands out tasks, reads the reports and decides; it writes no product code itself. `rewake claude` or `rewake pi` started by a person always starts a main.

## Worker
Variants: workers
Means: a session main raised from a worker template to do one kind of work; a person never starts one from a terminal.

## Worker template
Variants: template, templates
Means: a named recipe for a worker — harness, model, effort and role — raised with `rewake space open --alias`. The owner's room has three: two writers and one reviewer-advisor.
Where: the owner says «шаблон воркера».

## Shell
Means: the terminal screen of a main started at a terminal: main's pane and a worker's side by side, the strip under each, the bar and the hints.
Avoid: host mode
Where: `shell/`, on OpenTUI and Solid.

## Task
Variants: tasks
Means: a letter sent with `rewake send` without a flag; it binds the recipient to a report.
Where: the owner says «задача».

## Brief
Variants: briefs, `rewake brief`, `--brief`
Means: a markdown document for one task that main keeps in the room — its goal, its checkbox items, anything markdown holds; main alone writes it, a worker it was given to with `send --brief` ticks its items and comments, a reviewer given it with `--brief --review` reads and comments but never ticks, the person reads it in the shell. Not the briefing, the text a session gets at its launch; a brief in `.scratch/` is a file a task names, which may hold the same.
Where: [docs/briefs.md](docs/briefs.md); the records under `briefs/` of the room.

## Notify
Variants: `--notify`
Means: a letter that points to something or informs; it owes no report.
Where: the owner says «уведомление».

## finished
Means: the report of a worker's turn that ended normally; it reaches whoever awaits the report.

## error
Means: the report of a turn that broke off — a policy filter, the network, a limit — with the reason as it came; the task may be sent again.

## stopped
Means: the report of a turn a person stopped with Ctrl+C or Esc; only those awaiting a report get it, nothing is owed, and the task is not sent again — the person decides.

## `rewake.next`
Means: a new build installed beside the running `rewake2`; main swaps it in right after landing and green checks, and the owner restarts the room to run it.

## `unread/`
Means: the folder of a session's mailbox holding the letters not yet read, one JSON file each with `from`, `kind` and `text`.

## `done/`
Means: the folder of a session's mailbox holding the letters already read.

## `.scratch/`
Means: the git-ignored folder for the working files of a delegation or a review — briefs, reports, decision files; `.scratch/process/` is main's and survives a compaction and a reboot.
