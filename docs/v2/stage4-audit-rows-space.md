# Stage 4 audit: the rows of the space and the rest

Rows 13–22 of [the verdict table](stage4-audit.md#the-verdicts) argued — the worktree and the
tabs [the space](stage4-audit.md#the-target-a-space) needs, the person's UI, headless workers,
and the features that stay rewake's. Rows 1–12 are in
[stage4-audit-rows.md](stage4-audit-rows.md), whose opening also says which readings of pi
and Codex the paragraphs on the other harnesses fold in and how they are cited (PI1–PI14,
CX1–CX8). Facts carry the marks set in [the audit's opening](stage4-audit.md): *help*,
*skill*, *docs*, *rewake*.

## 13. Worktree — SPLIT, P12, P12b

**Why.** The owner's direction, October 13, 2026: the trouble is the worktree's lifecycle,
which every harness does its own way, so the whole lifecycle goes to Claude Code's
implementation, even when another harness runs in the worktree. rewake keeps no worktree
lifecycle of its own once that holds.

**What Claude Code does.** A worktree's life is tied to a Claude Code session: made at its
start by `-w` or by its `EnterWorktree` tool, removed at its exit, or by `claude rm` for a
background session (*help*; main's reading of the tool's text, 2.1.296). `-w` makes
`.claude/worktrees/<name>/` on branch `worktree-<name>`, copying what `.worktreeinclude`
names. Its base is the origin's default branch, not the current HEAD: `worktree.baseRef`
is "fresh" by default, and a launch can set another with `--settings` without editing the
person's files (*docs*, *help*). A clean checkout of an unnamed session is removed at exit,
a named or dirty one is asked about, and a background sweep removes old ones of background
sessions (*docs*). `claude rm` refuses a checkout with uncommitted changes or unpushed
commits unless told otherwise (`--discard-unpushed`, `--force-remove-worktree`), and
reports a removal the `WorktreeRemove` hook or Git could not finish (*help*).

**Who makes the checkout.**

- **A Claude Code worker** makes its own, started in its space with `-w`.
- **A worker of another harness** is seated by cwd in a checkout a Claude Code **keeper**
  holds: *a subagent of main* whose isolation is "worktree", of an agent type rewake's mod
  declares with `$.agent.register` (*skill*). Main observed two such subagents live on Claude
  Code 2.1.296, October 13, 2026: the checkout is made at `.claude/worktrees/agent-<id>` on a new branch
    `worktree-agent-<id>`, from `origin/<default>` rather than the current HEAD; it is
    Git-locked while the subagent runs; an unchanged one is removed with its branch when
    the subagent ends; a changed one is kept, unlocked, and its path and branch come back to
    main. A subagent kept alive while its space works holds the checkout under Claude Code's
    lock; when the space closes, the subagent ends and Claude Code removes or keeps the
    checkout by its own rule. The same subagent can stand for the space in the tasks list
    (row 16). The other candidate, a background session started with `claude --bg -w`, is
    dropped: workers close with main by the owner's decision of the same day
    ([answer 7](design.md#the-owners-answers)), so a keeper outside main has no purpose.

**P12b's first run**, October 13, 2026 ([the record](../research-claude-mod.md#p12b-a-subagent-of-main-as-the-worktrees-keeper)),
chose the subagent and how it is held: the mod spawns it itself with `$.agent.spawn`, with no
turn of main's model, and holds it by its blocking tool as a loop of polls, since one
`$.http.fetch` gave up after 30 s. The checkout came from `origin/main` and was locked with
Claude Code's reason; when the keeper ended, the unchanged checkout went with its branch.
When main exited with the keeper still running and a stand-in worker inside, the checkout
stayed, still locked under the dead harness pid, and nothing unlocked it; Git keeps one lock
per worktree, so rewake could not add its own. A named spawn gave an ordinary subagent, no
teammate. The background session refused to start in an untrusted directory.

**The veto stays conditional on P12.** A worker must not lose its checkout while it sits
there. Closing a space stops its worker before it releases the keeper, so the veto guards
the routes the space does not drive. The generic result of a classic hook carries `block` (*skill*), and the documentation
says a failing command hook fails the removal and leaves the directory (*docs*); neither
shows that every route that deletes honours a mod's answer — the same documentation says the
hook's JSON output is discarded, and background deletion distinguishes its routes (*docs*).
It also says a checkout made without a hook gets no `WorktreeRemove` event, while
`claude rm`'s help names that hook. Whether the mod's refusal holds on each route — exit,
`ExitWorktree`, the clean unnamed exit, `claude rm`, the sweep — and whether a
removal respects a `git worktree lock` Claude Code did not take, is P12.

**Still open after that run**: the base, fresh against the branch the work needs; what
sweeps a checkout an exit left locked or a keeper kept with changes; how rewake learns the
finished path, since a creation request names no final path; the cost of a long hold; and
the seated worker's Git metadata writes.

**A kept checkout is not forgotten** (the owner, October 13, 2026). Claude Code keeps a
checkout with changes or commits when its keeper ends; rewake's record of the space keeps it
too, main is reminded of it at its start and on a restore, and the room view lists it until
it is merged or removed ([the target](stage4-audit.md#the-target-a-space)).

**If P12b fails**, rewake keeps one worktree lifecycle of its own that follows Claude Code's
conventions — `.claude/worktrees`, the base reference, `.worktreeinclude` — and a Claude Code
worker shares it by having its `-w` answered through `classic.WorktreeCreate` with that
checkout, so there is still one lifecycle, not two.

**Code.** Most of `internal/adapter/claude/worktree.go` goes either way: rewake no longer
takes `--worktree` from Claude Code's arguments or refuses `-w`, `--tmux` and
continuations. `internal/host/worktree` goes once a keeper holds; under the fallback it
stays, reshaped to Claude Code's conventions, and its `land` and `finish` are candidates to
drop in favour of Git's and Claude Code's own means.

**Other harnesses.** pi: no worktree of its own; it sits by cwd in a checkout another owner
made, and loads that checkout's context files (*pi reading*; PI12). *Codex reading* sets
the seating requirements whoever owns the checkout:
the actual canonical cwd passed to the process and its registration, both Git metadata
directories granted for commits, a fresh environment without the parent's identity or
socket, the worker's own configuration home, and the owner's removal aware of the live
worker. It warns that a creation event names a request, not a finished path, which P12b
asks; that ownership metadata is not occupancy; and that the removal veto must be shown on
every native removal route, not one synthetic event (CX6) — the condition P12 keeps.

**Property.** The person's configuration unedited (a hook in rewake's own plugin, loaded
for its run, and Claude Code's checkouts under its own rules); the core knows no harness;
"a checkout of its own" holds for every worker.

## 14. The room view — KEEP

`rewake list` is the room: names, roles, owing, waiting, every harness. `claude agents
--json` lists Claude Code's own sessions with `state`, `status`, `waitingFor`, `pid` and
`cwd` (*docs*) — a source of a Claude Code session's activity the mod gives anyway, never
the room. **Other harnesses.** pi: none in its help. *Codex reading*: `codex agents` and
its thread list lack the room's other harnesses and obligations.

## 15. Permissions and the person's UI — MOD, stage 6

As designed: `classic.PermissionRequest`, `$.ui.status`, panes. The person's own status
line is never replaced (answer 4 of [design.md](design.md#the-owners-answers)). **Other
harnesses.** pi: `ctx.ui.setStatus`, widgets, notices and a message renderer; part of
them no-ops under RPC (*pi reading*). *Codex reading*: no
loadable pane or render hook; a plain CLI view where needed.

## 16. Worker tabs and buttons — SPLIT, P15

**Why.** ideas §1 and §2: a worker main launches appears as a button in main's Claude Code,
and inside it a pseudo-terminal runs the worker. The mod cannot own that terminal:
`$.process.spawn` gives no pseudo-terminal, a one-time stdin and a child that dies when
the module unloads (*skill*). So the terminal is rewake's — the space's terminal process
owns a pseudo-terminal and runs the harness in it, while the mod only draws its screen and
forwards keys. Whether that process is the worker's own wrapper, the detached process of
row 1 or one of its own per space is open until P11 and P15 decide
([the target](stage4-audit.md#the-target-a-space)). Any harness can then be hosted, pi and
Codex included; ideas §1's "Later" note that Codex starts by a command line is the same: its
command line starts it inside the terminal.

**The tab is Claude Code's** (owner's idea, October 13, 2026). A subagent gets an entry in
Claude Code's tasks list, and opening it opens the agent's transcript. With the worktree's
keeper a subagent of main (row 13), that entry is the space's tab and rewake draws no tab bar
of its own. Opening it replaces the subagent's view with rewake's own: a pseudo-terminal in
which the real harness runs with its own screen, not another harness's transcript shown by
Claude Code's means. The terminal is drawn in one of two places (*skill*, the render sites and
elements): first, the keeper's own transcript, which holds little but its pending call of
the mod's blocking tool — the mod redraws that tool's row as a cell grid repainted by
`$.ui.blit`, with a client element taking keys; second, the pane or the band, whose hooks run
again with the agent in view when the person switches to it, the pane docked beside the
transcript in fullscreen from 110 columns, the band taking at most half the terminal. A tab
of the mod's own drawing is the fallback.

**What happens when.** A reload of main's mod loses the view and the key path for a moment;
the worker and its terminal live on in the space's terminal process, and the redrawn view
reattaches. Main exiting closes its spaces, by the owner's decision of October 13, 2026
([answer 7](design.md#the-owners-answers)): the mod stops the workers at `session.end` and
releases the keepers, and the space's record keeps each worker's session id, so main can
restore the workers it needs later. The button
appears when main launches a worker, not only on a "+". A launch by main is a new authority
(main starting processes); the owner granted it on October 13, 2026 as the target — main
creates a space and seats a worker in it ([the target](stage4-audit.md#the-target-a-space))
— and its limits are designed in stage 4 and stage 6.

**The probe.** P15 (keys, Escape, focus, paste, size, the redraw cost) was stage 6's by
ideas.md; it runs beside stage 4's probe by main's decision of October 13, 2026, because a
rewake process that owns a pseudo-terminal changes every adapter's launch, and stage 4 should
not build one that rules it out. It now asks first whether a transcript row may carry the
cell grid and the client element and take keys, its height in the agent's view with
fullscreen on and off, its repaint by `$.ui.blit`, scroll and focus; then whether the pane
follows the view switch, may be opened on it, and the size it gets. Its first half, P15a,
ran on October 13, 2026 ([the record](../research-claude-mod.md#p15a-the-spaces-terminal-in-the-keepers-own-tool-row)):
the keeper's tool row carried the grid and the client element and was repainted there; the
client element got letters, Alt, function keys, the arrows and Tab, and not Escape, Ctrl+C,
Ctrl+X or paste. A prefix key the client element receives carries those to the seated
harness ([two levels of UI](stage4-audit.md#two-levels-of-ui)); P15b checks it end to end
with a real pseudo-terminal. *pi reading*: interactive pi runs in a pseudo-terminal rewake owns with no
cooperation, regular or fullscreen (PI11). *Codex reading*, same conclusion: a pane is not an interactive terminal,
and a reload must not be assumed to keep the worker (CX7).

**Property.** A session the person asks rewake to host is allowed; no service beyond the
space's terminal process, which serves its space alone.

## 17. The room panel — MOD, stage 6

ideas §2, unchanged: a pane over `rewake list` and `rewake inbox --owed`, buttons for
what main does. **Other harnesses.** pi: widgets above or below its editor (*pi reading*).
Codex: none.

## 18. Headless and background workers — DROPPED

Dropped by the owner's decision of October 13, 2026 that workers close with main
([answer 7](design.md#the-owners-answers)): no worker runs under a supervisor that outlives
it, and P16 goes. The reading that stood before:

Claude Code's background sessions run under a supervisor service (`claude daemon`), each
its own process, surviving the terminal but not shutdown; `claude agents`, `attach`,
`logs`, `stop`, `rm` manage them; dispatched ones open a draft pull request (*docs*,
*help*). A rewake worker's life is its wrapper's, and a supervisor that outlives it breaks
"lives as long as the session"; `--bg --exec` runs any command as a background task with
output in memory only (*docs*). A background session as a worktree's keeper, running no
worker of its own, was P12b's other candidate (row 13), dropped by the same decision.
**Other harnesses.** pi: `--mode rpc`, JSON lines over stdin and stdout, carries wake,
steer, abort, compaction, a new session and telemetry, with the extension still loaded for
tools; a person cannot type into such a worker (*pi reading*; PI13). *Codex reading*: `exec` is one-shot
with stdin read to its end; a persistent headless worker on the app-server is CX7's
second half.

## 19. Timers, monitors, idle notices — KEEP

`ScheduleWakeup`, `Monitor`, cron tools and `notify_when_idle` (*docs*: one notice, twelve
hours) are a session's own; rewake's notices, pending and reports carry the room's.

## 20. `$.store` as room state — KEEP

`$.store` is per machine for the plugin (*skill*), not per room or build; the state
directory stays the only room state ([state.md](../rules/state.md)).

## 21. Variables a session exports to children — KEEP

Claude Code exports the messaging socket and other markers to its children (*docs*); a
launch from inside a session scrubs them (`childMarkers` in `claude.go`). *Codex reading*
asks the same of every mixed launch: the parent's socket, token, tool identity and
conversation are never the worker's own. The *pi reading* adds that a person's extension
that spawns child `pi` processes passes `REWAKE_*` on, so the extension identifies its own
process rather than trusting the environment.

## 22. The version gate — KEEP

The adapter's minimum (2.1.287, mods on by default) stays; the mod's hello adds
`$.session.version()` (*skill*) so a plugin loaded into another version says so. pi moved
through about nineteen releases in five weeks, several breaking extensions (*pi reading*):
its adapter pins a version and carries a minimum as this one does.
