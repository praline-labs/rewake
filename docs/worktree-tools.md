# Worktree tools of both harnesses

The shared worktree system: rewake's own worktree core ([worktree.md](worktree.md)) behind the
same model tools in Claude Code's mod and rewake's pi extension, one Go implementation and thin
front ends that call the CLI. The owner accepted the design on October 20, 2026; its first
slice, landed that day, is the template field and `worktree_status`; its second, the same day, is
main's `worktree_list` and `worktree_land`; its third, the same day, a Claude Code worker's entry
into a checkout of its own mid-session (`enter_worktree = true`, below). Leaving a checkout from
inside a session, or holding more than one, is not built.

## A template's `worktree = true`

An alias of `~/.config/rewake/aliases.toml` (or `.rewake/aliases.toml`) may say `worktree = true`: its
worker starts in a new worktree of its own, as `space open --worktree` does. Refused on a
harness rewake makes no worktree for, and beside the harness's worktree flag in the alias's
`args`, which would be a second answer; `rewake workers` shows it.

`space open` resolves the worker's worktree once, and the same answer goes to the worker's
launch and to the space host, which captures the checkout and releases it at close:

- a typed `--worktree` or `--worktree=<name>`, on `space open` or as the harness's flag after
  `--`, wins: over `worktree = true` it only names the checkout;
- else `--no-worktree` leaves the template's checkout out;
- else the template's `worktree = true`, or the worktree flag in its `args`, stands;
- `--worktree` given both on `space open` and after `--`, or beside `--no-worktree`, is refused.

The flag is taken out of both argument lists before the launch is built, so a launch never
carries two.

## `rewake worktree status`

A view for the session it runs in: no lease, no lock file, no pruning, no write. It knows the
session by `ownRunIn` — the room directory, `REWAKE_SESSION` and `REWAKE_EPOCH` — never by
its working directory, nor by whoever holds the name now, and answers:

- `checkout`: the one whose record names this run as its owner — `ref`, `path`, `branch`,
  `base` (the commit it was made at) and `source` — or `null`;
- `space`: the space whose worker this run is, when one not yet over is;
- `close`: what that space's close would do with the checkout if it closed now. `outcome` is
  `remove`; `keep`, with `reasons`; `unknown`, when the look failed, as a close keeps what it
  cannot judge; or `none` — main's checkout, a session that is no space's worker, a worker
  without a checkout: no close removes anything there.

The forecast is `worktree.CloseForecast`, `Release`'s own policy (SP7 of
[rules/space.md](rules/space.md)): what rm keeps a checkout for but files git ignores, and a
commit no other local branch holds. Only the worker's own run is not counted as working there —
the close comes after it ends — while every other live session still is. It is a look, under
no lock: later edits, commits and sessions change the answer, and it promises nothing.

```json
{"session":"api-claude","room":"rewake-28c6be7d4d60","role":"write",
 "checkout":{"ref":"rewake-3f9a1c/wt-d219d1","path":"/home/me/.local/share/rewake/worktrees/rewake-3f9a1c/wt-d219d1",
  "branch":"wt-d219d1","base":"24f3e8e…","source":"/home/me/code/rewake"},
 "space":"api",
 "close":{"outcome":"keep","reasons":["its commits up to 5b1e2c0a9d3f are on no local branch but its own"],
  "note":"if this space closed now, its close would keep the checkout and its branch; …"}}
```

## The `worktree_status` tool

The same name and contract in both harnesses: no input; it runs
`$REWAKE_EXECUTABLE worktree status --json` by argv — no shell, nothing of the model's on the
command line — and answers its JSON, or an error for a nonzero exit, a timeout, a cut answer or
a command that could not start. `REWAKE_EXECUTABLE` is the path of the rewake that started the
run, set by every launch over an inherited value
([environment](design.md#environment-the-harness-receives)); without it no tool is registered.
The bound is 60 seconds: the view reads records and git, and holds no lock to wait on.

- **Claude Code**: the mod registers it at `session.start` as `mcp__rewake__worktree_status`
  and serves it from a `tool.call` hook that answers in place of the engine's core; it calls
  `$.process.run(argv, {cwd, env, timeoutMs})` with the run's `REWAKE_DIR`, `REWAKE_ROOM`,
  `REWAKE_SESSION` and `REWAKE_EPOCH` set over the harness's environment, each named literally,
  as the engine requires. A `.catch` turns a failed hook into an error result. Mod API of
  2.1.302, from the `mods` skill.
- **pi**: the extension calls `pi.registerTool` and `pi.exec(executable, args, {cwd, signal,
  timeout})`, whose child inherits pi's environment. pi 1.3.0 reports a child it killed at the
  timeout with exit code 0, so `killed` is read first.

The tests: `internal/cli/worktree_status_test.go` (the forecast and the view),
`internal/cli/space_worktree_test.go` (the resolution), the mod's `tests/worktree.test.ts`,
pi's `worktree.test.ts`, and the workflow scenario `fixture-space-worktree`.

## Main's `worktree_list` and `worktree_land`

Two tools registered only in main's session, by the role the launch names in `REWAKE_ROLE`
(`launch.RoleEnv`, set at every launch over an inherited value, none for a probe; a run without
it, or of another role, is offered `worktree_status` alone). That chooses what a session is
shown; it is not authorization: `rewake worktree ls` and `land` stay open to any session, as the
CLI was. The same names and contracts in both harnesses (`mcp__rewake__worktree_list` and
`rewake_worktree_list` in pi, and so for land), run by argv through `REWAKE_EXECUTABLE`.

- **`worktree_list`**, no input: `rewake worktree ls --this-room --json`. `ls` lists every
  worktree under the root, of every repository and room, because the root is the owner's;
  `--this-room` keeps those whose record names a session of the current room, the one it was made
  for, ended or not. Per checkout, what `ls` has: the session and whether any runs there
  (`runningSessions`), and `check` — changes, ignored files, commits on no other branch. It does
  not forecast a close: that needs a space and a worker, which `worktree_status` answers for the
  session itself. A view: no lease, no lock file.
- **`worktree_land`**, `name` and optional `into`: `rewake worktree land <name> [--into=<branch>]
  --json`, answering the landing (`worktree`, `branch`, `target`, `old`, `new`, `commits`,
  `checkout`) or the command's refusal as an error — a target that moved on says to rebase in the
  worktree, a target checked out elsewhere or with changes says so. A change: the room's lease as
  for any land. A `name` or `into` that is empty or begins with a dash is refused by the tool
  before any process starts, since it would be read as a flag. The bound is 120 seconds, not
  60: land waits up to a minute for a hook, and a shorter bound would kill it while it waits.
  The tool's text asks main to land into the integration branch only on the owner's or a review's
  word; the tool does not enforce it.

The tests: the mod's `tests/worktree.test.ts` and pi's `worktree.test.ts` (offer by role, argv,
bounds, refused input), `TestWorktreeLsThisRoomKeepsTheRoomsCheckouts`, `TestARunNamesItsRole`.

## A worker's entry: `enter_worktree = true` and `worktree_enter`

Main's decisions of October 20, 2026, on the live probe of Claude Code 2.1.302
([research](research-claude-mod.md#entering-a-checkout-mid-session-october-20-2026-21302)):
**one checkout per worker, ever**; entry only for a worker launched without one; no exit. The
space's singular `Checkout` then holds it, and its close releases it after the worker is gone,
by the same capture as a launch's checkout (SP7): nothing is released mid-session, no state
format or protocol changes.

- **The field.** An alias may say `enter_worktree = true` (off by default): its worker starts
  where main works and may enter a checkout rewake makes for it. Refused on a harness without
  entry (`adapter.WorktreeEntryHarness`; pi has none), and beside `worktree = true` or the
  harness's worktree flag in `args`: a worker's one checkout is made at its start or entered
  later. A typed `--worktree` on `space open` starts the worker in one instead, entry off.
  `rewake workers` shows it. `space open` carries it to the space host (`Spec.Enter`), which
  passes `REWAKE_SPACE_ENTER=1` to the worker's launch and captures a claimed checkout while the
  record names none, as for `worktree = true`; the launch hands it to the adapter as
  `LaunchRequest.WorktreeEntry`.
- **The run.** Claude Code's adapter writes the worker's settings copy in the run's directory
  even with memory off, `EnterWorktree` taken out of `permissions.deny` and added to
  `permissions.allow`; the person's `settings.json` is not changed, and a deny they wrote in
  another form (a rule with a specifier) stays and wins. A whole-tool deny would hide the tool
  from the mod's own call too. Only then the run gets `REWAKE_WORKTREE_ENTER=1`
  (`adapter.WorktreeEnterEnv`), which no run inherits; under `--command`, whose wrapper decides
  the settings, the variable is set and the deny is the wrapper's. It is not a boundary — a
  session's own command could set it — only the run's way to say what its template allows.
- **`rewake worktree enter [<name>] --json`**, a change (the room's lease), knows the run by
  `ownRunIn` and refuses without `REWAKE_WORKTREE_ENTER=1`. With no checkout claimed by this
  run, it makes one from the commit the session is at, from its working directory, on a new
  branch (`worktree.Create`, the name generated when none is given), and claims it for the run:
  `created`. Called again it answers the same one — `reused`, or `already` when the run was
  started in it — and refuses another name: "one checkout per worker". A claim that fails takes
  the checkout back as a failed launch's is. The answer is `outcome`, `checkout` (as status
  has it) and `skipped`.
- **The tool** `mcp__rewake__worktree_enter` (`name?`), registered only where
  `REWAKE_WORKTREE_ENTER` is `1`: runs that command by argv through `REWAKE_EXECUTABLE`, cwd
  the session's, bound 120 seconds; then, unless the session's root already lies in the
  checkout, calls `$.tool.call({tool: "EnterWorktree", path})` and answers rewake's JSON with
  Claude Code's message. Claude Code asks a person before it moves a session into a checkout
  outside `.claude/worktrees`, whatever the allow rules say (seen live, 2.1.302), and nobody may be
  at a worker's screen: a `tool.check` hook on `EnterWorktree` asks the engine first and, for
  rewake's own call (`next.origin.plugin === "rewake"`) in a run that may enter, turns only the
  engine's own `ask` into `allow`; a deny, and an ask a PreToolUse hook, a rule or a setting
  gave, stand (a person's policy outranks the entry, found in review), and every other call keeps
  the engine's verdict. A refused or failed entry is an error that says the checkout stays this
  worker's and a second call enters it. The tool's text says the session starts in main's checkout
  and asks for the call before the first change, whether or not main's task names a worktree: the
  probe saw a worker decline the native tool on a peer's request.
- **The briefing.** A run that may enter — the adapter's answer, not the template's wish: no
  run directory means no entry — gets a section YOUR CHECKOUT after its role's (`entryBrief`,
  `brief.Context.Harness`): it starts in main's own checkout, changes nothing there — no edits,
  commits, branch switches, stashes or resets — calls `worktree_enter` before its first change to
  a file of the repository, and tells main if the call fails rather than fall back. Nothing said
  so before October 21, 2026: a worker of such a template never called the tool, ran
  `git switch -c` in main's checkout, and main's merge and push went to the wrong branch. The
  briefing is what the model reads every time; the tool's text only when it weighs the tool.
  No guard refuses an edit outside a checkout: the live case was a shell command, which a guard on
  the edit tools would not have stopped, and a guard on git commands is not simple.
- **The guard.** A `tool.call` hook on `EnterWorktree`, in a run where `REWAKE_WORKTREE_ENTER`
  is `1`, passes only rewake's own call (`next.origin.plugin === "rewake"`) and refuses the
  model's, which would make a checkout rewake does not know; its `.catch` refuses unless the
  tool already ran (fail closed). Elsewhere the call goes on untouched: main's `-w`,
  `EnterWorktree` and `Agent` isolation stay Claude Code's. `ExitWorktree` stays denied: Claude
  Code never removes a checkout entered by path, and there is no way back.
- **Known limits.** Two `worktree_enter` calls at the same moment may each make one; the close
  captures the first claim, and the other stays under the root for `rm`. A worker cannot go
  back to main's directory, nor hold two checkouts; a worker that needs another is a new space.
  rewake's registry keeps the launch directory as the session's `cwd`.

The tests: `internal/cli/worktree_enter_test.go`, `TestATemplatesEntryIsItsOneCheckout`,
`TestLaunchSettingsWithEntry`, `TestTheBriefingFollowsTheRunsEntry`,
`TestACheckoutClaimedBeforeAnyPollIsKeptWhenMainEndsFirst` (both
flags), the mod's `tests/enter.test.ts`, and the workflow scenario
`fixture-space-worktree-enter` (the fixture offers entry by running the command).
