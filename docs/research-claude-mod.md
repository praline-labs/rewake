# Research: what a probe mod observed inside a running Claude Code session

The live records of stage 4's probe ([design-claude.md](v2/design-claude.md#the-stage-4-probe)):
what a mod loaded for one run was seen to do inside an interactive Claude Code session —
start processes, hold a subagent with a worktree of its own, draw into that subagent's
transcript and take keys there, and show a real pseudo-terminal's screen in it with a
harness running inside. Kept apart from
[research-claude-control.md](research-claude-control.md), which records 2.1.280's
function-hooks plugin, because these are records of the mod API's era and of one probe.

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
([design-claude.md](v2/design-claude.md#how-a-tool-call-runs)). Two runs, both on October 13,
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
authority obligations of the mod's join ([stage4-audit.md](v2/stage4-audit.md#a-what-the-wrapper-does-today-and-whether-the-mod-can-take-it-over)).

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
