---
description: Dated records of what probe mods and rewake's mod were seen to do inside Claude Code, by version.
---

# Research: what a probe mod observed inside a running Claude Code session

The live records of stage 4's probe ([research-claude-mod.md](research-claude-mod.md)):
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
key sent sooner was lost). To find the dialog in a screen dump, look for "Enter to confirm" in
the text with the escape codes removed (checked October 16, 2026). A worker's profile presets
the trust of the repository it runs in (`markReady` in `internal/adapter/claude/claude_json.go`),
so the dialog is met only in a directory outside it or in a probe's private HOME.

## P4: the peer of the mod's requests

Whether `$.http.fetch` with `socketPath` connects from the harness process itself, which
every check of the mod's endpoint rests on
([claude.md](claude.md#the-peer)). Two runs, both on October 13,
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
authority obligations of the mod's join.

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

What building the workers' pane (since removed for the shell) showed of the mod API:

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

## Compactions counted

October 18, 2026, Claude Code 2.1.302, a source and test-kit check with the `mods` skill of that
version:

- A compaction the mod asked for (`$.session.compact`) is counted from the result the call
  returns; a native one, manual or automatic, from the `session.compact` event. The kit
  counts a remote completion once.
- `claude plugin validate --strict` warns that an organization-managed installation skips a
  locally installed plugin's `session.compact` observer: there a native compaction is not
  counted, while one main asked for still is, from its result.

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

October 25, 2026, Claude Code 2.1.310, a probe mod under `claude --plugin-dir … --model haiku
--effort high` in a pty, a private config directory, no login and no request:

- `$.settings.read({ key: "effortLevel" })` stays `undefined` before and after `/effort max`
  ("Set effort level to max (this session only)"); the engine's own `settings.read` does not
  fire on it, and no settings file is written. The bare `/effort` picker confirmed with Enter
  saves `modelSettings.<model id>.effortLevel` to the user layer ("saved as your default for new
  sessions") and fires the engine's read; its `s` (this session only) leaves no trace. A
  worker's user layer is the service folder, so a picker's Enter there sets the effort of every
  later worker on that model.
- `command.run` fires `{ command: "effort", args: "max" }` for a typed level, `args: ""` for the
  picker, whose choice it does not carry.
- The status line's input carries `effort.level`: `"high"` from the flag at start, `"max"` at the
  next run after `/effort max`, with no request between, and the picker's choice at once, its `s`
  and its Enter alike. rewake takes this source
  ([claude.md](claude.md#telemetry)).

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
  `prompt_input_exit`, `other` and, since October 23, 2026, `logout` (`/logout` lagged the closing
  screen; the reason is read from 2.1.302's reason list, not seen live), and the shell starts its closing screen at the screen's leave,
  which precedes it (docs/shell.md, "The closing screen"). From October 19 the mod sent it at
  `/exit`'s `command.run` too, and the closing screen hid the dialog above for five seconds.

## Entering a checkout mid-session (October 20, 2026, 2.1.302)

`claude -p` on the cheapest model in a private repository with a probe mod logging the worktree
events and `next.origin`, then a live room in a private `HOME` and `REWAKE_DIR` with two workers.
What [worktree-tools.md](worktree-tools.md#a-workers-entry-enter_worktree--true-and-worktree_enter)
builds on:

- `EnterWorktree {name}` takes the path a `classic.WorktreeCreate` hook returns (the hook's `cwd`
  is the source repository, even from a linked worktree); `{path}` of a checkout in `git worktree
  list` fires no hook, only `tool.call`, and works from the main checkout, from a linked worktree
  and after an exit. While entered, a second entry is refused by Claude Code.
- A checkout entered by `{path}` is never removed by Claude Code ("not the owner"), even with
  `discard_changes`; a hook-made one honours a `WorktreeRemove` block; a native one in
  `.claude/worktrees` goes with no `WorktreeRemove` event. `Agent {isolation: "worktree"}` fires
  `WorktreeCreate`; `-w` in `-p` did not.
- After entry `$.session.root()`, `$.session.cwd()`, Bash's directory and the `Stop` hook's `cwd`
  move to the checkout; the transcript, `REWAKE_*` and the mod's socket do not: mail, a wake and
  the turn's report reached main from inside it. rewake's registry keeps the launch directory.
- A whole-tool `permissions.deny` removes the tool for everyone: a mod's `tool.check` allow does
  not bring it back, and its `$.tool.call` rejects "no tool named \"EnterWorktree\" in this
  session".
- The model declined `EnterWorktree` on main's task — "only for explicit user or project
  instructions", "main's message is a peer request" — so a rewake tool whose text invites the
  call is needed.
- `next.origin` of the model's call is `{plugin: "engine", tier: "core"}`, of a mod's
  `$.tool.call` `{plugin: "<mod>", tier: "user"}`: a guard tells them apart.
- `claude plugin validate`: `classic.WorktreeCreate`/`WorktreeRemove` hooks do not run in a
  plugin a person installs where an organization manages Claude Code; a `tool.call` guard has no
  such limit.
- Live in rewake's room, the same day: `EnterWorktree {path}` from the mod's `$.tool.call` in an
  interactive worker, `EnterWorktree` in `permissions.allow`, still opened Claude Code's dialog
  "permission-root relocation to … — a model-supplied worktree outside .claude/worktrees/" in the
  worker's own pane; a `tool.check` hook answering `allow` for the mod's call let it through with
  no dialog. The engine's verdict there is `{decision: "ask"}` with no `hook` field; a PreToolUse
  deny comes back to `$.tool.call` as `{deny: "PreToolUse:EnterWorktree hook error: …"}`, and a
  `tool.check` hook that answers without `next(e)` overrides it (`claude -p` with a plugin
  dropping the prompt, no model turn, 2.1.302). The entered worker's transcript was under the checkout's project directory
  (`~/.claude/projects/<checkout's slug>/<session id>.jsonl`), not the launch directory's.

## Permission requests and questions through the mod

October 18, 2026, Claude Code 2.1.302: the facts [permission-requests.md](permission-requests.md)
rests on. An isolated interactive `claude --plugin-dir` in a pseudo-terminal the probe owned, not
through rewake: `--permission-mode manual`, `--setting-sources ''`, `--strict-mcp-config`, a per-launch
settings layer asking for `Write`, a private home with a private copy of the login, a cheap
model; most cases were mod commands without a model turn, three were model turns. No setting of
the person's was changed. Its mod, driver and evidence were kept in the room's scratch directory,
not in the repository.

- **The check.** `tool.check` awaits `next(e)`, the engine's verdict, which runs nothing; a
  genuine ask from the per-launch rule reached it. The 2.1.302 CLI calls the manual mode
  `manual`, not `default`.
- **The question.** A direct `$.tool.call` of `AskUserQuestion` is refused by the host
  (`runs the AskUserQuestion tool: that is $.ui.ask`). `$.ui.ask` opens the engine's own dialog
  as a `tool.call` of `AskUserQuestion` through the mod's hook on it, and resolves to a string:
  the label, labels joined with a comma and a space, or the text under Other. It rejects when
  the hook answers `{ deny }`.
- **Holding.** The hook started the dialog with `next(e)` and awaited successive host-held
  `$.http.fetch` polls: one permission call was held 42 seconds, polls answered pending at 15
  and 30 seconds, with no own-time skip and no fetch cut. No sleep or timer promise was in the
  hook.
- **The first answer.** A remote answer returned as `{ result: { questions, answers } }`, keyed
  by question text, closed the shown dialog, and the pending `next` then settled as a native
  rejection that did not replace the chosen result. A keyboard answer at 2 seconds — Allow once,
  and Refuse — won over a remote answer scheduled for 12, and the native result's event woke the
  host's poll. A model's own `AskUserQuestion` answered remotely reached the model as its
  result, descriptions and all.
- **The deadline.** A shortened 8-second deadline answered `{ deny }`, `$.ui.ask` rejected, the
  check's handler refused, the dialog closed, and nothing was written; the same for a model's
  own write.
- **What it did not test.** Mail, `rewake answer`, the identity check, session allowances, the
  full 120 seconds, restarts, subagents, other modes, races under load. The mode a check runs in
  is read from the harness's classic events (`permission_mode`), as the 2.1.302 declarations
  give it, not seen live.

## A worker's dialogs in auto mode

October 23, 2026, Claude Code 2.1.302. The probe of [research-launch.md](research-launch.md#a-read-outside-the-working-directories), with
a mod (`--plugin-dir`) logging `tool.check`'s verdict and the classic events; a cheap model; ms
from the start, one run per case.

- Auto mode is the default: `classic.SessionStart` and `PermissionRequest` carry
  `permission_mode: "auto"`, so a worker launched with no mode runs in it.
- The read-outside prompt's check: `{decision: "ask", reason: "Claude requested permissions to
  read from <path>, but you haven't granted it yet."}`, with no `setting`, `rule` or `hook` — not
  the read block's setting-named ask. 7–15 ms later `classic.PermissionRequest` fired, with
  `tool_name`, `tool_input` and a `permission_suggestions` entry `addRules` `Read(//<dir>/**)`,
  destination `session`, and no `tool_use_id`; the dialog was drawn. `classic.Notification`
  `{notification_type: "permission_prompt", message: "Claude needs your permission"}` came 6.0 s
  after `PermissionRequest`, in all three runs that left the dialog up.
- A `PermissionRequest` hook that held 15 s, waiting inside `$.process.run`, left the engine's
  dialog on the screen meanwhile, and its `{decision: {behavior: "allow"}}` closed it: the tool
  row read "Allowed by PermissionRequest hook" and the read ran. No budget skip.
- The person answering first, 5 s into such a hold (choice 4): the read ran at once,
  `classic.PostToolUse` with the call's `tool_use_id`; the hook was not aborted
  (`next.signal.aborted` false at its end) and its allow 10 s later changed nothing.

### The notifications of a dialog

Read from the installed 2.1.302 binary, not probed; review-pi found the informational ones
(October 23, 2026). `classic.Notification` is raised for every notification the engine sends, of
the types its settings list: `permission_prompt`, `idle_prompt`, `auth_success`,
`elicitation_dialog`, `agent_needs_input`, `agent_completed`, `elicitation_url_dialog`,
`worker_permission_prompt`, `push_notification`, `computer_use_enter`, `computer_use_exit`, three
`quota_auto_resume_*`, `model_refusal_fallback`, `auth_storage_failure`, `plugin_notification`.

- A dialog the engine draws in the session's screen, left unanswered for the idle delay, sends
  its entry's notification with its type or, with none, `permission_prompt` — permission dialogs,
  managed settings, a sandbox's network request, plan mode. An MCP server's input dialog names
  `elicitation_dialog` or `elicitation_url_dialog`. The probe's "Claude needs your permission" is
  this path.
- Two dialogs, the teammate setup and auto mode's classifier billing notice, name
  `agent_needs_input`, which a background agent's "needs your input" also sends.
- The rest name no dialog of this screen: `plugin_notification` is any plugin's `$.ui.notify`,
  `agent_completed` a background agent's end, `worker_permission_prompt` a teammate's request
  shown to its leader, `computer_use_exit` the end of computer use, `push_notification` the
  model's push.

## The prompt cache (October 23, 2026, 2.1.302)

- `turn.step`'s `usage` (`TurnUsage`) carries the API's `input_tokens`, `cache_read_input_tokens` and `cache_creation_input_tokens` of each response: the source of the hit percent. It has no ttl or expiry; `PreModelSwitch` (`prompt_cache_warm`, `cache_ttl`) and a resume's `SessionStart` (`prompt_cache_likely_expired`) mention it only at their moments.
- The status-line input has `prompt_cache` once a request was made: `warm`, `caching_observed`, `ttl` (`5m`/`1h`), `expires_at` (epoch seconds), `requests`, `misses`, `hit_ratio`, `cache_write_tokens` and more — read from the strings of the installed binary, not from a live sample (it needs a login). `statusLine.refreshInterval` is seconds, at least 1, beside the event-driven runs; the command inherits the harness's environment, `REWAKE_MOD_SOCKET` among it, and is a child of the harness process. The run's `--settings` `statusLine` beats the user settings'. See [claude.md](claude.md#the-cache-ttl).

## The model's name and thinking

October 26, 2026, Claude Code 2.1.310, the `mods` skill taken from the same version and the
strings of the installed binary:

- The mod has the model's id alone: `$.session.model()` answers the main loop's model "as
  `/model` shows it", which for main was `claude-opus-5-5[1m]` in `rewake list`, and
  `turn.step`'s `model` is the id the request names. No call of the mod gives a display name.
- The status line's input builds `model: {id, display_name}` with `display_name` from the
  engine's own name of that id — `Haiku 5.5`, and `Opus 5.5 (1M context)` for a 1M variant,
  whose parenthesis names the native window, not what `--autocompact` keeps the conversation to.
- It also carries `thinking: {enabled}` for every model: on unless the session's thinking was
  turned off, and on for a model that rejects disabled thinking whatever the setting. A model
  without thinking reads `true` as well, so `enabled` alone does not say a model thinks.
- `$.config.list()` has a `thinking` row (2.1.302, above), the setting rather than the session.
- Nothing of the mod says what a request asked of thinking: `turn.step` carries `agentId`,
  `effort`, `index`, `messageCount`, `model` and `turnId`, and its response streams thinking
  chunks, which say thinking happened, not that it is on. A review's probe on an old model saw
  `thinking.enabled` `true`.
- Project and local settings: `<project>/.claude/settings.json`, and `settings.local.json` at the
  repository's canonical root — the main checkout for a linked worktree — when the session starts
  below it and the root, its `.git` and `.claude` are the user's, else in the session's own
  directory. A project-only `statusLine` was drawn before rewake's overlay and gone after it, until
  `rewake statusline` read the project's layers (review, October 26, 2026).

So main's launch passes rewake's status line as a worker's does, and the host takes the name and
`thinking.enabled` from it ([claude.md](claude.md#telemetry)). Live the same day, a main of a dev
build in a scratch repository under `--model haiku`, no request: its `rewake list` row read
`"Haiku 5.5"`, `"medium"`, Thinking `on`; the person's own status line drew in main's pane as
before, and the strip read `Haiku 5.5 medium thinking`.

## The mod on Claude Code 2.1.310 (October 25, 2026)

- Read against the `mods` skill's changelog "2.1.302 → 2.1.310": an `ask` a plugin's `tool.check` makes is marked `plugin` in the verdict (2.1.304; `decider` is gone), so the EnterWorktree check keeps a verdict with `plugin` as the person's policy (`hooks/enter.ts`). After Esc or Stop the engine refuses a mod's own new work — `$.prompt.submit`, `$.command.run`, `$.agent.spawn`, `$.session.send`, a background `$.tool.call` — until the person's next input; the mod calls none of them.
- Live on 2.1.310: the mod's tests pass; a worker's `worktree_enter` created and entered its checkout with no dialog; a worker's `AskUserQuestion` reached main as a request and `rewake answer` was taken. A worker stopped with Esc — main got `stopped` — was woken by a task through the inbound socket about ten seconds later and answered it: the rule above does not hold the socket's line.

## A mirror of a live session for an interface of rewake's own (October 28, 2026, 2.1.310)

Whether a mod can feed a view of rewake's own while Claude Code keeps running as the terminal
program ([research-launch.md](research-launch.md#whether-claude-code-has-a-client-server-split-to-sit-between)).
The `mods` skill was taken from 2.1.310, the installed version. One interactive `claude
--plugin-dir` ran in a terminal tab of the write session's own, in `/tmp/ui-probe-34`, with the
workers' service folder as `CLAUDE_CONFIG_DIR` and an environment cleared to HOME, PATH, TERM and
the machine's proxy variables — without them every request was refused with 403 `Request not
allowed` and "Please run /login". The cheapest model, low effort, auto mode, two prompts. The
probe mod posted every event it saw through `$.http.fetch` to a small server on a Unix socket,
stamped with the module's own `performance.now()`; its source, the server, the launch script, a
log viewer and the evidence are kept in the ignored `.scratch/ui-probe/` (`mod/hooks/register.ts`,
`server.ts`, `launch.sh`, `show.py`, `log.jsonl`, `debug.log`, `screens/`). Times are from the
prompt's `prompt.submit`. **[live]**

- **Streaming.** `ui.render` on `AssistantMessage` fires when a text block is settled, never per
  delta. A reply of 2024 characters streamed as 195 `turn.step` text chunks from 2.34 to 6.17 s
  (a median of 9 characters 20 ms apart, the longest gap 75 ms), and no `AssistantMessage` render
  came in that span. It rendered at 6.23 s, as the next block (a tool call) began, with the 1681
  characters of whole lines shown so far, and at 6.62 s with all 2024, after the last flush; a
  four-character reply rendered once with `""` at its start and once whole. Every later render
  was a redraw with the same text (a scroll, ctrl+o, a reload). `classic.MessageDisplay` lies
  between the two: five flushes of whole lines 0.7 to 0.9 s apart (491, 393, 433 and 364
  characters, then a final 343). The props are the block's `text`, markdown, with
  `isFirstOfReply`, `isSummary` and `onScreen`; thinking is not among them, and reached the mod
  only as `turn.step` `thinking` chunks, five, all of empty text. `Spinner` rendered on each
  change of `mode` (`responding`, `thinking`, `tool-input`, `tool-use`) with `word`, `message`
  and `suffix`, not on its elapsed time or tokens: two renders in the ten minutes of the longest
  hold. `ToolProgress` never fired: no call offered the run-in-background pill. A live view takes
  the text from `turn.step` chunks, or `MessageDisplay` for line batches, and the settled block
  from `AssistantMessage`.
- **A long permission wait.** A `tool.check` hook on Bash read the engine's verdict with
  `await next(e)` — `ask`, "Claude requested permissions to use Bash, but you haven't granted it
  yet.", `approver: "person"` — and held the call on successive 25-second long polls of
  `$.http.fetch` to the local server: 60.0 s (3 polls) then allow, 180.0 s (8 polls) then deny,
  600.0 s (24 polls) then allow. No skip, no fetch cut, `next.signal` never aborted. An allowed
  call ran 0.4 to 0.5 s after the answer; the denied one did not run, the debug log read `ask ->
  deny by plugin ui-probe`, and the model read "Permission to use Bash denied by plugin ui-probe:
  <reason>" and went on. The same hold made of `$.clock.sleep(5000)` was cut at its budget: at
  10.0 s the hook's `.catch` was called with `{kind: "timeout"}` and its deny refused the call,
  while the hook's loop ran on and was logged as overrunning 5 s later. A `$.clock` wait counts
  against the 10-second budget; a `$` call such as a fetch does not. Meanwhile the terminal
  showed the call's row (`● Echo probe-hold-10`, `⎿ $ echo probe-hold-10`) under the spinner
  (`Perambulating… (9m 54s · ↓ 1.5k tokens)`, `esc to interrupt`), no permission dialog — the
  hook answers before the engine would draw one — and a free prompt box.
- **What else a mirror would need.** `UserMessage` carries the prompt's whole `text`, its
  `origin` (`{kind: "composer"}`) and `isExpanded`; a slash command's echo came first under the
  `requestId` `placeholder`. In the folded transcript a run of calls, the shell commands with
  them, is one `ToolGroup` whose `calls` give each call's `tool_use_id`, `tool`, `input` (`{}`
  while its arguments stream), `isRunning`, `isErrored` and `isInterrupted`, without output.
  `ToolUse` and `ToolResult` render per call only where the view unfolds them: in the ctrl+o
  transcript every call did, with its whole `input` and `output` (`{stdout, stderr, interrupted,
  …}` for Bash, the file for Read, the error text for a refused call). The model's
  `AskUserQuestion` rendered as a `ToolUse` (`input: {}`, then 0.26 s later the questions) and as
  the `AskUserQuestion` site with `questions` whole — question, header, options with their
  descriptions, `multiSelect`; its `ToolResult` carried `answers` keyed by question.
  `CommandOutput` and `TurnDuration` (`word`, `durationMs`) render too. `$.session.messages()`
  gave plain `{role, text, toolUses}` rows.
- **A prompt from the mod.** `$.prompt.submit` inside a `command.run` hook is refused: "called
  from a command.run hook, it would wait on the turn this hook is holding; submit from a later
  event". From a `$.clock.after` timer it started a turn 51 ms later, drawn as "Prompt from the
  ui-probe plugin" above the text even with `asUser`, and the model answered it as a typed
  prompt. The submitting plugin's own hooks do not see what the submit raises: its
  `prompt.submit` hook and four `ui.render` calls in that dispatch were skipped as re-entry (the
  debug log), so the prompt's `UserMessage` never reached it; the reply's `AssistantMessage` did.
- **Not tested.** `ToolProgress`, a subagent, a person answering or pressing Esc during a hold,
  a hold past ten minutes, other surfaces, a run under load.

So a view of rewake's own can take the reply as it streams only from `turn.step`, not from the
render sites; it can hold a permission request at `tool.check` for at least ten minutes and
answer it from outside, with no dialog in the terminal meanwhile; and it can rebuild the
transcript's rows from the render props, the calls' output from the unfolded rows or the events.
