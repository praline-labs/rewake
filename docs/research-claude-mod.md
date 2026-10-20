# Research: what a probe mod observed inside a running Claude Code session

The live records of stage 4's probe ([design-claude.md](v2/design-claude.md#the-stage-4-probe)):
what a mod loaded for one run was seen to do inside an interactive Claude Code session —
start processes, hold a subagent with a worktree of its own, draw into that subagent's
transcript and take keys there, and show a real pseudo-terminal's screen in it with a
harness running inside. Kept apart from the facts of 2.1.280's function-hooks plugin
([research.md](research.md#interrupting-a-turn-commands-and-the-stop-hook)), because these are
records of the mod API's era and of one probe.

Each section is a dated record of what was seen, with the run it was seen in. It is not a
statement of the mod API: the `mods` skill holds that, and where a record and the skill
differ, the record says so and the skill is what to read for the current API.

## How the first probe ran

October 13, 2026, Claude Code 2.1.296, the `mods` skill taken from the same version. Main
ran every probe live; the write session built the kit and ran only its dry mode and the
validator. The kit stayed outside the repository, in the ignored `.scratch/v2-s4-probe/`:
a probe mod (one module, switched per question by environment variables, passing
`claude plugin validate`), a helper process, a small HTTP server on a Unix socket, and a
driver that ran `claude` with `--plugin-dir` in a pseudo-terminal of 200 by 50, kept the
screen with a VT emulator, typed keys and dumped the screen at each step.

Every run had its own directory, named below by its suffix (`/tmp/rewake-probe-<suffix>`):
a scratch repository on a branch `work` one commit ahead of `origin/main`, its bare
`origin` beside it, the mod's copy, the logs, the screen dumps. The environment of each
run had the calling session's rewake and Claude Code variables removed and a private
`REWAKE_DIR`; the cheapest model was used, and nothing was installed. **[live]** throughout.

On 2.1.296 the folder-trust question of a first start reads "Quick safety check: Is this a
project you created or one you trust?" with "No, exit" preselected: the first run stopped
there, and the driver then answered Down, Enter, one second after the text appeared (a
key sent sooner was lost).

## P4: the peer of the mod's requests

Whether `$.http.fetch` with `socketPath` connects from the harness process itself, which
every check of the mod's endpoint rests on
([design-claude.md](v2/design-claude.md#the-socket-and-its-peer)). Two runs, both on October 13,
2026, Claude Code 2.1.296.

### The probe's kit, extended (`Na99qK`)

The kit's server read `SO_PEERCRED` on every connection and compared its pid and the start
ticks of that pid with the `claude` process the driver started. **Every** request the mod
made named the harness process itself, same pid and same start: at `session.start`; in
turns; before and after a model's Bash call; after `/reload-plugins`, where no new module
load was observed, so this was not a hooks worker respawned by force; at the keeper's spawn,
its hold, the duplicate hold about 30 seconds later and every poll of the hold. A model's
Bash child, `curl --unix-socket` to the same socket, showed its own pid, with `comm` `curl`,
and was refused.

**Not seen**: a hooks worker that crashed and was respawned; a shell child started before a
reload, trying the endpoint after it.

### rewake's own mod (S4-5)

Main ran a built rewake with the embedded mod, `rewake --main claude` in `-p` mode with the
cheapest model, in a private `REWAKE_DIR`. The mod's hello was admitted on the run's mod socket
with its load's id, the session's id and `harnessVersion` 2.1.296, and `hello.json` in the run
directory carried it. A model's Bash child sending the same request with `curl` was answered
403, naming its process as not the harness's own.

### rewake's own mod, its module checked before it loads (S4-6)

October 16, 2026, Claude Code 2.1.296, in `claude plugin test`: every test failed with `hooks
module did not load: ... $ itself is passed as an argument (bound, passed, spread, returned or
read); $ is always spelled $.noun.event(...) at the call site`, once the module handed a hook's
`$` to its outbox. The engine checks a module before it loads it: `$` may only be called
through. The outbox takes closures a hook makes over its own `$`, each a `$.noun.event(...)`
call, and its timers call those; a closure called from a `$.clock.after` callback after the
hook returned works. The `mods` skill taken from 2.1.296 does not state the rule.

Since its republication on October 16, 2026 the `mods` skill states the rule, finer than this
record: `$` may also be passed to a function declared at the top of the same file, never across
an import, into a method or a class, or kept in a variable (`background.md`, its section on
where `$` may go, 24 variants checked on 2.1.296).

## P11: a process the mod starts

### `$.process.run`, the child detaching (`nugmw7`)

- The mod's first `session.start` fired before any prompt; none was typed in the run at
  all. `$.process.run` of the helper returned in 64 ms.
- The child's parent was the harness process itself — `comm` `claude`, the pid Claude
  Code runs as — in the harness's own process group and session, not a separate worker
  process.
- A double fork with `setsid` detached it: parent pid 1, a session and group of its own.
- `/reload-plugins` ran `session.start` again under a new module load. The detached helper
  lived through it; the new load saw the helper's pid file and started no second one.
  Changing the mod's file in the plugin directory did not reload the module in this run;
  only the command did.
- `/exit` gave `session.end` with reason `prompt_input_exit`, and the harness process
  ended. The helper, checking the harness pid every second, saw it gone within a second
  and exited, after 113 s of life.
- `$.env.get("CLAUDE_CODE_MESSAGING_SOCKET")` gave the harness's own inbox socket, a path
  under the user's runtime directory named by the harness pid.

### `$.process.spawn`, the child staying (`b19RdW`)

The child's output came back line by line, one a second. At `/reload-plugins` the child got
SIGTERM, and the new load started a new one; at `/exit` the child ended with the loop. As the
skill says: a spawned child dies with its module load.

### `$.process.spawn`, the child detaching (`jOyBP5`)

A spawned child that double-forked and called `setsid` lived through the reload; the stream
ended at once, and the detached process stayed, parent pid 1, until the harness pid was gone.
This variant checked no pid file, so the reload started a second helper; both ended with the
harness.

**What it answers.** A process the mod starts at the first `session.start`, before any
prompt, can detach, outlive a module reload and end with the harness process, and its
parent before detaching is the harness process. Not seen: `session.end` without an exit
(clear, resume), a respawn of the hooks worker, the harness killed outright, and the
authority obligations of the mod's join ([stage4-audit.md](archive/v2/stage4-audit.md#a-what-the-wrapper-does-today-and-whether-the-mod-can-take-it-over)).

## P12b: a subagent of main as the worktree's keeper

### Spawned by the mod, held and released (`woP6c8`)

- `$.agent.register` declared the agent type `rewake-probe:keeper` with worktree isolation.
  `$.agent.spawn` from a slash command of the mod started it with no turn of main's model;
  main took one short turn later, when the keeper's completion notice arrived.
- Claude Code made `.claude/worktrees/agent-<id>` on a new branch `worktree-agent-<id>`
  from `origin/main`, not from the checked-out `work`, and Git-locked it with the reason
  `claude agent agent-<id> (pid <harness pid> start <ticks>)`.
- The keeper called the mod's `space_hold` tool, and the mod held the call by long-polling
  the kit's socket through `$.http.fetch`. Each fetch was aborted after 30 s with "no
  complete answer within 30000ms", which the skill does not state; a hold is therefore a
  loop of polls shorter than that. The keeper's call stayed held for about 90 s across
  aborted polls. Its first call ended at the third aborted poll, which fell in the same
  second as the release, so its answer was the mod's "lost its server" text; a second
  `space_hold` call, made 31 s after the first under another agent id that
  `$.agent.list()` never listed, returned on the release itself. The second id is
  unexplained.
- Found afterwards in the kit, not in Claude Code: in this run each poll asked the kit's
  server to wait 50 s, longer than the 30 s after which `$.http.fetch` gave up, so every
  poll failed, and the kit's loop gave up after three failures in a row. That is why the
  first call ended with "lost its server" at the release. The later runs waited at most
  25 s per poll and saw no failure.
- `$.agent.list()` showed the keeper as running, with `spawnedBy` naming the mod's plugin.
- When the keeper ended, Claude Code removed its unchanged worktree and the branch.

### Main's exit with a worker inside

Runs `tZXlTa`, without rewake's lock, and `SNr8JH`, with it. Main typed `/exit` while the keeper held its call and a stand-in worker (a `sleep` whose
working directory was the checkout) sat inside. After the exit the worktree and its branch
were still there, unchanged, still Git-locked with Claude Code's reason naming the harness
pid that had ended; the stand-in kept running. Claude Code did not remove the checkout of a
keeper that was still running when main exited, and nothing unlocked it.

In the second run a `git worktree lock` of rewake's own was tried first and refused: Git
keeps one lock per worktree, and Claude Code's was there ("already locked, reason: claude
agent agent-<id> …"). Once main has exited that lock names a dead process, and what sweeps
such a checkout was not seen.

### A named spawn (`jFP9oG`)

`$.agent.spawn` with a `name` started an ordinary named subagent of the keeper type — a
worktree made and locked as before, the name shown by `$.agent.list()` — and no teammate:
no `teammateId` came back. Nothing in the spawn's arguments asks for a teammate.

### A background session as the keeper (`FKFkzX`)

`claude --bg -w keeper1` in the run's fresh repository refused, with and without a
prompt: "Workspace not trusted. Run `claude` in <dir> once and accept the trust prompt,
then retry". No background session started, and this keeper was not tried further.

## P15a: the space's terminal in the keeper's own tool row

### The tab, the grid, the keys (`K7pYUU`; earlier tries `EMheCZ`, `lIxo7H`)

- The keeper showed in Claude Code's tasks list under the prompt as
  `◯ rewake-probe:keeper  space s1`. From the prompt, Down selected `main`, a second Down
  the keeper ("Enter to view · x to stop"), and Enter opened its view: the keeper's
  transcript in place of main's, the prompt reading `Message @rewake-probe:keeper…`.
- In that view the mod's `ui.render` hook on the keeper's `space_hold` row drew a Raster
  test grid, 60 by 10 cells with a live counter, and a Client line under it. `$.ui.blit`
  repainted the grid once a second. Before the view was open `blit` was refused, "no Raster
  of its own is mounted under key grid": the row is mounted only while its transcript is on
  screen.
- After a click on the Client these keys reached it: a plain letter; Alt, as the letter with
  `meta` set; F1 as its raw sequence `\x1bOP`; the four arrows; Tab. These did not: a
  bracketed paste, which went into the agent's prompt; Ctrl+X; Ctrl+C, after which the
  agent kept running, so it was no interrupt either; Escape, which returned focus to the
  prompt.
- The Client reported its size as 200 by 2: the terminal's width and two rows of its own.
- A Client text holding a control character fails validation ("a text child holds a control
  character"); the kit's display of a raw F1 hit that and now escapes control characters.
- After the release the row returned to Claude Code's own drawing,
  `space_hold (MCP)(space: "s1") ⎿ space s1 closed`.

### The largest grid tried (`UqTei1`)

In a terminal of 200 by 50, fullscreen off, a Raster of 196 by 44 cells in the keeper's row
rendered and repainted. The transcript scrolled so that the screen showed the grid's last 34
rows above Claude Code's own chrome — the Client line, the spinner, the prompt box, the status
line and the tasks list, about 16 rows. In this layout a space's screen gets the terminal's
height less that chrome.

**Not seen yet**: the largest grid with fullscreen on, the pane and the band as the second
place, the cost of a redraw at 200 by 50, and a real pseudo-terminal with its screen; the
last was P15b, below.

## P15b: a real pseudo-terminal in the keeper's row

October 13, 2026, Claude Code 2.1.296, in a terminal of 200 by 50 with fullscreen off. The
kit's helper owned a real pseudo-terminal of 160 by 30, ran a program in it and kept its
screen with a VT emulator as cells. The mod long-polled the screen through `$.http.fetch`,
each poll under 25 s, and blitted it into the Raster of the keeper's `space_hold` row. The
Client under it turned each key into the bytes a terminal sends and posted them to the
helper. The prefix was Alt+a, then one key: `c` for Ctrl+C, `e` or `[` for Escape, `x` for
Ctrl+X, Alt+a again for Alt+a itself; any other key cancelled it.

### A shell and `cat -v` (`rRg9nU`)

- Every key typed in the Client reached the pseudo-terminal as the right bytes: letters,
  Alt+x as ESC x, F1 as ESC O P, the arrows, Tab, Shift+Tab as ESC [ Z, Backspace as 0x7f,
  Enter as CR.
- The prefix delivered 0x03, ESC, ESC, 0x18 and ESC a for `c`, `e`, `[`, `x` and Alt+a.
- From a key the driver typed to its echo on the outer screen — through the outer Claude
  Code, the Client, the mod, the helper, the pseudo-terminal, the emulator, the long poll
  and the blit — ten samples took 31 ms at the median and 45 ms at most.
- A scan of the Ctrl keys reached the pseudo-terminal for Ctrl+A and Ctrl+B only; after
  that the focus left the Client, and the later keys and a paste went into main's prompt,
  where main's model answered them. Which Ctrl keys reach the Client beyond A and B was not
  seen.

### vi and less (`R9hqiI`, `lFafj9`)

`vim -u NONE` drew full-screen; `Go`, a line of text, Escape through the prefix and `:wq`
left the file with the new line, so Escape reached vim through the prefix. In `less`, paging,
`G`, `g`, a search and `q` worked, and it exited normally.

### A nested Claude Code (`A3TDXM`, then `vI91jc`)

- `claude` started in the pseudo-terminal, in the keeper's worktree, with no folder-trust
  question: the checkout lies under the repository already trusted. Its own screen
  ("Claude Code v2.1.296 … agent-<id>") was drawn in the keeper's row on the outer screen.
  "say ok", typed through the Client, was answered "ok".
- In `A3TDXM` the outer repaint stopped after the first frames. The keeper called
  `space_hold` a second time, and the kit's loop followed the second call, whose row was not
  drawn, so each blit was refused with "no Raster of its own is mounted under key pty". Two
  prefixed Ctrl+C reached the pseudo-terminal 1.95 s apart and did not end the nested
  session; it ended only when the kit cleaned up (SIGHUP, exit 129). The outer harness used
  4.2 % CPU while it drew.
- In `vI91jc`, with the kit fixed to answer a second call at once and to blit only into the
  drawn row, it ran end to end. In the keeper's view the outer screen showed the nested
  session's own screen live — "❯ say ok", "● ok", "✻ Crunched for 1s" — with up to 13 blits
  in a window of 5 s while the nested session drew. Alt+a c twice, about 0.3 s apart, ended
  the nested session with exit code 0.

### The keeper's second call

In every run that held the keeper long enough a second `space_hold` call came: 31 s after the
first in `woP6c8`, 33 s in `A3TDXM`, 31 s in `vI91jc`. Each came under an agent id that
`$.agent.list()` never listed — the list held the keeper alone — with the same input keys as
the first (`agentId`, `space`, `tool`, `tool_use_id`). In `vI91jc` the kit answered it at
once, "space s1 is already held", and the first call went on holding unharmed. Main's
hypothesis, not proven: Claude Code retries a subagent's tool call that shows no progress for
about 30 s as a new attempt.

### Fullscreen: not run

Fullscreen has no flag; it is switched by `/tui fullscreen` or by `CLAUDE_CODE_NO_FLICKER=1`
(the 2.1.296 binary's strings, not run). Its start arms a boot canary, and a fullscreen launch
that dies before it is healthy disables fullscreen "on this machine"; that record lives in the
configuration the probe's runs share with the person's own sessions, so the probe did not
start one. The largest grid with fullscreen on stays unmeasured.

**Not seen**: which Ctrl keys reach the Client beyond A and B; paste through the Client; the
cause of the keeper's second call; fullscreen; the pane and the band.

## A worker acts on the notice's preview

October 16, 2026, Claude Code 2.1.296, main's live runs of S4-6 part A
(`.scratch/v2-s4-build/live-s4-6.sh`), the worker on haiku and on sonnet alike. **[live]**

The worker's Claude Code showed the notice as `● Rewake: lead-claude task, 1 new message`
with `↳ <the task's whole first line>` under it. The task was short, so its first line was
the whole of it. The model read `word.txt` and answered, and never ran `rewake inbox`: the
message stayed in the worker's `unread/`, the turn's end (the journal's `.done`) answered
nothing, and main got no report. Claude Code frames the notice as a teammate's request, and
the worker's playbook line "rewake inbox: read the mail a notice announced" did not carry
the model past a preview that already held the task.

What followed: a task's or a question's preview now ends by saying it is only a preview,
and the worker's playbook says a preview is never the task
([delivery.md](delivery.md#the-notice)).

### After the fix

October 16, 2026, Claude Code 2.1.296, main's live run of part A on the fixed build
(`.scratch/v2-s4-build/live-evidence/run-pass/`). **[live]** The worker ran `rewake inbox`,
read the question and answered it; the answer came back to main through the mod's
`turn/end`.

## S4-6: a worker's turns through the mod

October 16, 2026, Claude Code 2.1.296, the worker on the cheapest model, each run in a private
`REWAKE_DIR` and a fresh repository under `/tmp`, the worker an interactive session in a
pseudo-terminal whose keys the script wrote. Main's runs: part A
(`.scratch/v2-s4-build/live-evidence/run-pass/`) and part C
(`.scratch/v2-s4-build/live-s4-6c.sh`, `.scratch/v2-s4-build/live-evidence/c-main/`); the
writer's run of part C's `hello`, `lapse` and `reload`
(`.scratch/v2-s4-build/live-evidence/c-lapse-reload/`). **[live]**

### The hello after the harness starts

Measured from the harness's start ticks in the session record to the first moment the record
listed `turn-boundary`, polled every 0.2 s: 1.6 s in a folder already trusted, five launches
between 1.6 and 2.0 s; 3.1 and 3.3 s in launches whose folder-trust prompt the script answered,
which the time includes; 4 s in main's part A run.

### A print-mode main and a plain task

A main started with `claude -p` sent a plain task; the worker read it and answered on its
screen, and no report reached main: main's run had ended with its one turn before the
worker's ended, and a report to an ended run is moot by rule
([traps.md](traps.md#a-task-sent-from-a-main-in-print-mode-claude--p-gets-no-report)). The
same run with a `--question --wait 100` in main's one shell call got its answer.

### A mod that stops polling, and a reload

The run's module, `<run directory>/mod/hooks/rewake.ts`, was replaced by one that registers
nothing and `/reload-plugins` typed. Saving the file under `--plugin-dir` reloaded the mod by
itself before the command was typed: the screen showed `rewake: reloaded (0 hooks)`, then the
command's own `Reloaded: 1 plugin`. The boundary left the record 29 s after the reload, and a
task sent then was refused, exit 1, "worker-claude's turn boundary is not live". With the
module restored the screen showed `rewake: reloaded (3 hooks: session.start, turn.start,
turn.complete)`, the boundary was back within the script's half-second poll, and a task was
taken. Rewriting the unchanged module with one newline added reloaded it the same way, and the
boundary stayed live through 45 s of watching.

### Between turns

After a report, the worker sat idle for 45 s, past the 30 s lapse: the boundary stayed live, so
the heartbeat's chain kept polling between turns.

### A `--command` wrapper

A wrapper script ending in `exec claude "$@"` was admitted, its boundary live 1.6 s after the
start, and a question's answer came back. One running `claude "$@"` as its child was never
admitted: no boundary in 45 s, and a task to it refused, exit 1.

### An interrupt

Escape typed into the worker about 8 s after it read a question asking it to run `sleep 60`:
main's `--question --wait` printed `stopped from worker-claude:` and exited 1.

### A subagent

The worker was asked to have a subagent read the file and then to answer with `DONE` and the
word. Main's answer was `DONE <word>`, the main loop's; the worker marked its turn pending while
its subagent ran, and one finished report followed — one report per task, none for the
subagent's turn.

**Not seen**: whether the engine runs `turn.complete`'s hooks before the next turn starts; it
is not observable from outside the harness.

## The mod on Claude Code 2.1.302

October 17, 2026, Claude Code 2.1.302, the `mods` skill republished from it that day: the
writer's check on branch `mod-302` against the skill's changelog "2.1.296 → 2.1.302" and its
report `references/reports/2.1.296-to-2.1.302.md`, with a reading on the Codex side
(`.scratch/v2-s4-build/read-mod-302.md`). Not live; that is main's. **[kit]** marks what ran in
`claude plugin test`, **[read]** what was read in the report, the types or rewake's source.

- **A subagent's `turn.start`** (2.1.299): raised for subagents' and teammates' turns too, the
  main loop's without `agentId` **[read]**. The mod's hook did not filter it, so each subagent
  turn reached the host as a `turn/start` of the session. Harmless: the host passes a start on
  with no time, the core records only a timed one, and no mail tool opens turns on Claude Code;
  it cost an awaited request **[read]**. S4-6's part C ran on 2.1.296, before the change. Fixed:
  only a start without `agentId` is sent; a child's start and end, under an id and an empty one,
  beside a main pair, send only the main pair, and the test fails without the filter **[kit]**.
- **`$` into an imported `function`** (2.1.299) **[read]**: the helpers keep a `Host` of
  closures. `Outbox` and `Heartbeat` are classes, which `$` reaches on no build; the heartbeat's
  tests drive a hand-made `Host`; and such an import does not load on 2.1.287–2.1.298.
- **`.catch` not asked on re-entry** (2.1.301) **[read]**: the mod has no `.catch`.
- **The test kit** (2.1.299): a served `$` method's ten seconds are real time, not
  `mock.clock`'s — a fetch held 12 s of it was in flight at 11.5 s and answered at 12 s, so the
  heartbeat tests' 15 s holds mean what they did; the 31 earlier tests pass unchanged **[kit]**.
  The mod renders nothing and serves no method, so `part: { plugin }` and the budget change
  touch nothing **[read]**.
- **A save of the same bytes no longer reloads** (2.1.299) **[read]**. The reload scenario of
  `.scratch/v2-s4-build/live-s4-6c.sh` wrote back `$(cat <module>)` plus a newline, which is
  the file's own bytes: the record above shows 2.1.296 reloading on such a save. The script
  now adds a comment line.
- **Blockable actions refused when a hook is unreachable** (2.1.299) **[read]**: the mod's three
  hooks observe and block nothing; the rest of the report is nothing the mod hooks or calls.
- `claude plugin validate --strict` and `claude plugin test`, 32 tests, passed **[kit]**.

**Live on 2.1.302** (October 17, 2026, main's run of `.scratch/v2-s4-build/live-s4-6c.sh` on the
branch's build, a haiku worker; `.scratch/v2-s4-build/live-evidence/302/`) **[live]**: a report
came back and the heartbeat kept the boundary live through 45 s idle; an `exec` wrapper was
admitted and a child one refused a task; a module that stopped polling lost its boundary 30 s
after the reload and got it back at once when restored; the reload by a comment line kept the
boundary; the hello went live 1.4–3.1 s after the start; a subagent the worker started (seen on
its screen) left one report, the main loop's. Two scenarios needed new wording first. A
standalone `sleep 60` is refused in the foreground on this build ("Blocked: standalone sleep
60 … use Monitor … or run_in_background"); the worker ran it in the background, marked the task
`rewake pending`, and answered once it ended, so there was no turn to interrupt — with a Python
sleep in its place an Escape typed 17 s in reached main as `stopped`. And on the first run the
worker read the file itself instead of starting a subagent, which the script's answer check
could not tell; it now asks for the Agent tool and checks the screen for its call.

## The workers' pane (October 18, 2026, 2.1.302)

What building [the workers' pane](archive/v2/workers-panel.md) showed of the mod API:

- A `Client` drawn by a function in a file the hooks module imports (`hooks/pane.ts`) is found:
  `claude plugin validate` lists `hooks/keys.ts` among the surface modules, and it loads live.
- The test kit refuses a whole pane over one `Raster` cell outside the BMP, naming the cell's
  index ("cell 0 holds code point …, beyond the Basic Multilingual Plane"); a no-break space drawn
  as itself was not tried, the host draws every space as U+0020.
- Live, main at 200 by 50: the pane the mod opened by itself docked beside the transcript; a
  `Raster` of the dock's `bodyColumns` by its `bodyRows` less two drew a nested Claude Code, and
  `$.ui.blit` repainted it as the worker drew. After a click on the `Client`, letters, Enter and
  Alt+a reached it; Alt+a then `c` delivered Ctrl+C to the worker, clearing its prompt.
- A `Client` posting one key per post lost a letter of text typed 30 ms apart: one post a frame
  is delivered, and a later one replaces one not yet delivered (as the `ui.message` type says).
  Posting every unacknowledged key, acknowledged through the answer's `props`, lost none at 10 ms.

### The pane's focus, closing and frames (later the same day, 2.1.302)

Driven through a pseudo-terminal of 200 by 50, fullscreen on (`CLAUDE_CODE_NO_FLICKER=1`),
clicks sent as SGR mouse sequences; first a probe mod, then the pane with a real Claude Code
worker on haiku:

- A `Client` in an absolutely placed `Box` drawn after a `Raster`, the size of the `Raster` and
  drawing only its last line, leaves the `Raster` visible and takes a click on the `Raster`'s
  cells (`onPointer` saw it, in region cells); typing then reached the `Client`, and the worker.
- No way gives a `Client` the keyboard without a click: `ClientProps` has no `autoFocus`, and
  `$.ui.focus` names only a `Button`, `Input` or `Select`. A pane opened with `focus` read
  `isFocused` true, yet letters typed then went to the prompt.
- `closeOnEscape`: Escape right after the open closed the pane. With a `Client` holding the keys,
  Escape returned them to the prompt (`isFocused` false) and the pane stayed; a second Escape at
  the idle, empty prompt closed it. Ctrl+X then X closed the pane only while it held the keyboard
  (after the open, or Ctrl+X Tab); with a `Client` holding it, both keys vanished — the `Client`
  got neither — and with the prompt holding it nothing closed. The ✕ mark closed it always.
- An idle Claude Code worker drew no frame in 10 s. A working one drew about 9 frames a second,
  each as two screens a few milliseconds apart — cursor hidden while drawing, then shown — and a
  view taking the screen at the first change sent both, so the inverse cursor cell in the pane
  blinked at that rate. Taken 30 ms after the change, every frame of a working worker had the
  cursor shown, and the frames halved. With the worker working, each closing way above held in
  repeated tries.

### The pane's prefix, size and colors (later the same day, 2.1.302)

Driven through a pseudo-terminal of 200 by 50 (and 120 by 50), fullscreen on and off, main and
two Claude Code workers on haiku opened from main's shell mode:

- Alt+a reaches a `Client` holding the keyboard. `$.ui.close` of the pane's own id called from the
  `ui.message` hook closes it, and the keyboard returns to the prompt: letters typed next showed
  in main's composer.
- `$.ui.open` of an open id with other `rows` and `columns` and no `focus` resizes the pane and
  leaves the keyboard with the `Client`. `columns: 1000` docked the pane at 175 of 200 columns
  and 95 of 120, the transcript keeping 24; omitted, the dock took the share (88 of 200). Inline
  (fullscreen off) `rows: 1000` gave 38 body rows of 50.
- Fullscreen off, a click on the pane gave the `Client` no keyboard: the keys stayed the prompt's.
- Colors: the worker's `claude` ran with `TERM=xterm-256color` and `COLORTERM=truecolor`. With
  the outer Claude Code under `COLORTERM=truecolor` (as the owner's main runs), the worker's RGB
  `d77757` and `ffc107` were drawn `dd7755` and `ffbb00` — a Raster's color on a step of `0x11`
  per channel; with no `COLORTERM` there, `d7875f` and `ffaf00`, the 256-color palette. After
  dim was drawn as color, the status line's `5h` went from `999999` to `666666`; bold red and
  blue became `ff0000` and `5555ff`; the bold default `Claude Code v2.1.302` stays the default.
- Fullscreen off, an `Input` with `autoFocus` under a `Raster` took the keys after `/workers`
  opened the pane with `focus`; Enter ran `onSubmit`, a drawing with `value: ''` emptied the
  field, Tab moved the focus to the plain `Button`s and their hotkeys pressed them. The `Input`'s
  `label` is drawn with `: ` after it.
- `command.run`'s `presentation` carried `{ isFullscreen: true, columns: 200 }`; a pane drawing's
  `viewport.columns` is not the terminal's width (it read 108 docked).
- A `dockColumns` kept in `~/.claude.json` (`pluginPanes`) held the dock at 92 columns: `columns`
  of 100, 1000, 150 and 60 and none, asked through `$.ui.open` of the open pane, each answered
  `{ isPlaced: true }` and none moved it. Before it was kept, the same reopens moved the dock.

## Effort

October 19, 2026, Claude Code 2.1.302, a probe mod under `claude -p --plugin-dir … --model haiku
--effort low`, then rewake's mod live in a private state directory:

- `$.config.list()` has no effort row (it lists `switchModelsOnFlag`, `thinking`, `model`), and
  `$.settings.read().effortLevel` is the settings file's value, not the session's.
- A `turn.step` of the main loop (no `agentId`) carries `effort: "low"`, and a classic `Stop`
  `effort: {"level": "low"}`: the value the request runs at. The mod takes `turn.step`'s, the
  first point of a turn; it may also be an integer budget, sent as its decimal text.
- Live: a main launched with `--effort low` listed `"low"` after its first turn; a worker
  launched with `--effort high` listed `"high" (launch)` before any turn; a worker launched
  without one listed unknown, then `"medium"` after a turn, and after `/effort max` typed in
  its terminal still `"medium"` until its next turn, then `"max"`. Nothing the mod hears
  marks the change itself, so between them `list` lags by a turn.

## The end of the process, as the mod hears it

October 19, 2026, Claude Code 2.1.302, fullscreen (the alternate screen), a probe mod under
`claude --model haiku --plugin-dir …` in a stand-in terminal whose clock also stamped each of
the mod's posts to a socket of the driver's:

- `/exit`'s Enter: `command.run` with `command: "exit"` at +46 ms, then the harness leaves the
  alternate screen at +54 ms, `classic.SessionEnd` and `session.end` (`prompt_input_exit`) at
  +119 and +124 ms, the process gone at +650 ms. The hook awaited its post before `next(e)`,
  so the screen is left only after a `command.run` hook returns.
- The second Ctrl+C, or Ctrl+D: the screen left at +3 ms, `classic.SessionEnd` at +63–68 ms and
  `session.end` at +66–73 ms (`prompt_input_exit` both), the process gone at about +600 ms.
  Nothing the mod hooks fires earlier: the first press runs no command and ends nothing, and a
  `Button`'s `action` presses only where no engine handler of the action is mounted, which
  the exit's always is (the types' text).
- `/exit` while work runs — a background shell, or a tool in the foreground — opens Claude
  Code's dialog "Background work is running" (exit and stop the tasks, move them to the
  background and exit, or stay) after `command.run`, and Ctrl+C twice opens the same dialog with
  no `session.end`. Confirmed with Enter, the screen is left at +5 ms and `session.end` follows
  only once the tasks are stopped: +123 ms with one shell loop (October 20, 2026, 2.1.302, rewake's
  own mod in the shell). A `command.run` of `exit` is therefore no sign of the end.
- rewake uses `session.end` alone since October 20, 2026: main's mod sends `session/closing` for
  `prompt_input_exit` and `other`, and the shell starts its closing screen at the screen's leave,
  which precedes it (docs/v2/shell.md, "The closing screen"). From October 19 the mod sent it at
  `/exit`'s `command.run` too, and the closing screen hid the dialog above for five seconds.
