# A worker's permission request answered by main

**Design, not built; open choices at the end.** A worker that asks its harness for a permission nobody at its
keyboard will give stops the task, and main is not told
([work-queue.md](work-queue.md#now-after-100)). This is the contract for holding such a
request while main decides, on both harnesses, and falling back to a refusal that names
the next step when main does not answer in time. Nothing here is code; the facts it rests
on are marked with where they were seen, with file and line in
[permission-requests-sources.md](permission-requests-sources.md), and what is still a
guess is listed at the end.

## What the owner decided

On September 28, 2026 the owner decided:

- only a worker's requests are held — a session of the roles `general` and `write` —
  never main's own;
- what a grant or the workspace already allows passes as today, with no question;
  only a request beyond that reaches main;
- main allows only what it may do itself; anything dangerous — deleting, a force push,
  writing another tool's settings — goes to the owner;
- a held request waits for main within rewake's own limit, and past it the worker is
  refused with the next step: end the turn with `rewake pending "needs <what>"`, and
  main answers with a task carrying `--grant-dir`;
- a person at the worker's keyboard can still answer the prompt first.

## The contract

1. The harness raises a permission request it would put to the person.
2. The worker's wrapper decides at once what it can: a write inside a grant is allowed as
   today ([grants-claude.md](grants-claude.md)); a request reserved for the person
   ([what goes to the owner](#what-goes-to-the-owner)) is left to the person, and main is
   told of it; everything else is **held**.
3. A held request goes to the room's current verified main as a letter of the new kind
   `permission`, which wakes an idle main. The prompt stays on the worker's screen.
4. Main answers with `rewake permission <id> allow`, `deny ["reason"]` or `owner`.
5. The first of these settles the request: main's answer, the person's answer at the
   keyboard, an interrupt, the limit, the end of the worker's or main's run. What settled
   it is recorded; a later answer is refused and says what came first.
6. The limit is **120 seconds** from the request. Past it the worker is refused with the
   next step, and main's answer is refused with exit 1.

A request allowed by main is allowed once, for that call only. Nothing widens the
session's permissions for later calls: a directory the work keeps needing is a grant,
sent with a task after the turn ends.

## What is held, and what passes

**Passes with no question**, as today: whatever the permission mode and the rules let
through never reaches a permission request; a file-tool write inside a live grant is
allowed by the grant hook; the question rewake forces to take a grant back is answered by
it at once.

**Left to the person, and main told** — see [what goes to the owner](#what-goes-to-the-owner).

**Held for main** — every other request of a worker:

| Harness | Requests held | What main is shown |
|---|---|---|
| Claude Code | `Write`, `Edit`, `MultiEdit`, `NotebookEdit`, `Bash`, `WebFetch`, and MCP tools | the tool, the path, the whole command or the URL, the working directory |
| Codex | `item/commandExecution/requestApproval`, `item/fileChange/requestApproval` | the command and its directory, or the files the patch changes |

`ExitPlanMode` and `AskUserQuestion` on Claude Code are questions for the person, not
permissions, and are never held; 2.1.280 ignores a hook's `allow` for them anyway unless
it rewrites their input. On Codex, `item/permissions/requestApproval` (the granular
categories) and `mcpServer/elicitation/request`, through which an MCP tool call is
approved, are left to the person until someone needs them.

What never becomes a permission request is not held either: in auto mode a call the
classifier refuses is denied outright and reported to a `PermissionDenied` hook, and only
after three refusals in a row or twenty in the session does the harness fall back to the
prompt; `dontAsk` turns every prompt into a refusal before any hook; Codex under
`--ask-for-approval never` asks nothing ([where this was read](permission-requests-sources.md)).

Today the grant hook hands the wrapper only a path or a command
(`claude.GrantCall`); a `WebFetch` URL and an MCP tool's arguments have to be added to what
it passes for main to be shown them.

**Main's own requests** are never held, and neither is a session with no verified main in
its room, or one whose main is a Codex session: nobody could answer, so such a request
behaves as today; whether it should be refused at once instead is an
[open question](#open-questions-for-the-owner).

### What goes to the owner

Rewake cannot tell a dangerous command from a safe one, and does not try to judge
commands in general. It marks what it can recognize, and a marked request is not offered
to main for `allow`:

- a file write into the hard tier of [grants.md](grants.md#the-hard-tier), or into a
  `.git`, `.claude`, `.codex` or `.agents` directory at any depth;
- a command whose first word is `rm`, `rmdir`, `shred` or `dd`, or that runs
  `git push` with `-f`/`--force`/`--force-with-lease`, `git reset --hard`, `git clean`,
  `git branch -D` or `git checkout -- `, anywhere in a compound line.

A marked request is still a letter to main, saying it is the owner's, and main can
answer only `deny` or `owner`. Anything unmarked is main's judgment, within what main may
do itself: its role's text says so. The list is a floor, not a fence — main reading a
command it would not run itself answers `owner`.

## On Claude Code

Seen live on 2.1.280 ([the probe](#the-live-probe-on-claude-code)): the PermissionRequest
hook runs while the dialog is already on the screen, and its answer closes the dialog; the
hook's own `timeout` above the 600 s default holds the call that long. A deny's message
reaches the model as the tool's error, and the turn goes on. Read in the 2.1.280 binary:
a hook that times out, fails or exits non-zero without JSON gives no decision, and the
prompt stays with the person; an answer that comes after the person's is dropped.

That race holds on the main thread and in a foreground subagent. A background subagent
of an interactive session runs its PermissionRequest hooks first and shows the dialog
only when they decide nothing, so while its request is held nobody sees a prompt; its
payload carries `agent_id` and `agent_type`, so the wrapper can tell such a request.

1. The settings layer gives PermissionRequest an entry of its own:
   `rewake grant-hook` with a timeout of **180 s** — rewake's limit plus room to answer —
   instead of the five seconds it shares with PreToolUse today. The hook stays one
   process; PreToolUse keeps its five seconds.
2. The hook hands the call to the worker's wrapper. The wrapper answers a grant at once;
   for a held request it records it, writes the letter to main, starts the limit and waits
   for main's decision ([security](#security)) while the hook's connection stays open.
   That is a new operation, not the keeper's `decide`: `decide` runs under the keeper's
   lock and within the two-second `exchangeLimit`, and a held call must hold neither
   (`internal/grantauth/keeper.go`). `DecideGrant` is silent for a session with no grant,
   so holding starts after it, not inside it.
3. Main's `allow` answers `{"behavior":"allow"}` with no `updatedPermissions`; `deny`
   answers deny with `rewake: main refused <what>: <reason>`; `owner` closes the hook
   with no output, so the dialog stays for the person, unbounded as today.
4. At the limit the wrapper answers deny with the next step:

   ```text
   rewake: main did not answer within 2m whether you may write
   ~/src/shared-lib/client.go. Do not retry it. End the turn with: rewake pending "needs
   write access to ~/src/shared-lib" — main answers with a task that grants it.
   ```

   For a command, `needs to run: <the first 80 characters>`; for a fetch, the domain.
   A directory named in the next step is the file's parent, the one `--grant-dir` would
   take; a parent in the hard tier is marked for the owner instead
   ([what goes to the owner](#what-goes-to-the-owner)).

While the hook holds the call the turn is working; six seconds into the dialog the
harness's `permission_prompt` notification sets `waiting: approval` in the telemetry
already ([session-state.md](session-state.md)), unless
`CLAUDE_CODE_DISABLE_PERMISSION_PROMPT_NOTIFY_HOOKS` is set. A task carrying a grant is not
delivered while a turn runs (`inbox.CarriesGrant`) — so main answers the held request
itself, not with a grant task. That a notice arriving meanwhile is not read before the
call resolves is inferred, not seen.

## On Codex

Read in the source of 0.155.1 and 0.157.1: approval requests go from the app-server to
the client; the relay forwards them to the terminal today and passes the terminal's
replies straight upstream, even while a queued request of its own waits
(`gateway/event_stream.go:89`, `gateway/gateway.go:133`). Any
connection may answer; the server takes the first decision for a request id, drops a later
one with a warning, and sends `serverRequest/resolved` to the thread's subscribers, which
closes the terminal's prompt. It waits with no limit. `turn/steer` does not resolve a
request; a turn that starts, completes or is aborted — `turn/interrupt` — clears every
open request, and `serverRequest/resolved` follows. A decline tells the model only
`exec command rejected by user` or `patch rejected by user`: the reply carries no text.

1. The relay forwards the request to the terminal as now, so the person sees it, and
   gives it to the wrapper, which records it and writes the letter to main.
2. Main's `allow` becomes the reply `accept`, never `acceptForSession`; `deny` and the
   limit become `decline`; `owner` sends nothing, leaving the prompt to the person.
   The relay writes the reply upstream with the request's own id, the way the terminal's
   replies go today.
3. The model learns the next step two ways, since a decline carries no text: the worker's
   briefing says what `rejected by user` means under rewake while the worker is
   unattended, and before the decline the relay steers the next step into the running
   turn, the same text as on Claude Code. In the source a steer is accepted into a
   regular turn whatever is open in it, and pending input is drained before the next
   model request, so the model reads it right after the decline — not yet seen live.
4. `serverRequest/resolved` for a request main has not answered settles it as answered
   at the keyboard, or cleared by an interrupt when a `turn/interrupt` preceded it.

**The other way on Codex: its own PermissionRequest hook.** Both versions run
PermissionRequest hooks, in the Claude Code format, before a request reaches guardian or
the person, for every approval that goes through the core's `request_approval`: commands,
patches, MCP tool calls. A hook's deny carries its message to the model verbatim, so the
next step needs no steer; an allow approves once; `updatedInput`, `updatedPermissions`
and `interrupt` are refused; a hook that fails or times out (600 s by default, its own
`timeout` above that) decides nothing. Hooks are on by default. The price: the hook runs
*before* the prompt, so while it holds a request nobody at the keyboard sees one. And a
hook the owner has not reviewed runs only as trusted: its hash under `hooks.state` in the
user's configuration or in the session's `-c` flags, or `--dangerously-bypass-hook-trust`,
which trusts every hook of the run. Passing the hook and its hash with `-c` for one launch
would leave the owner's configuration untouched — read in the source, not tried. Which
way Codex goes is an [open question](#open-questions-for-the-owner).

## What main sees

A letter from the worker, written by its wrapper:

```text
from write-claude · permission · 23:40:02
write-claude asks to Write ~/src/shared-lib/client.go — outside its workspace
while working on 1780000000000000000-012345abcdef "Bump the client in shared-lib"
answer within 2m: rewake permission 1780000000000000001-0abc allow | deny "why" | owner
```

A request marked for the owner reads `the owner's to allow: <why marked>` and offers
only `deny` and `owner`. Reading the letter after it was settled shows the outcome
instead of the ready command. `--json` carries the tool, the path or the command, the
working directory, the deadline and the task ids.

While the request waits, `rewake list` shows it on the worker's row
(`permission: <id> write ~/src/shared-lib/client.go, 1m40s left`; `permissionRequests` in
`--json`),
and `rewake inbox --awaited` shows the task's state as held on that request.

### The kind, and every place it touches

`permission` is a new `inbox.Kind`. It asks for no work and owes no report, and it is
never sent with `rewake send`: only a worker's wrapper writes it. Following
[AGENTS.md](../AGENTS.md#adding-a-role-or-a-message-kind), a kind a place does not know
reads as a task, so each of these decides it:

- `internal/inbox/inbox.go` — the constant, and `AsksForWork`, which must list it, or
  `Owed` would make main owe the worker a report; `IsReport` and `Settles` leave it out:
  it is no turn's outcome and answers no question;
- `internal/inbox/window.go` — `canWait` lets whatever asks for no work wait up to four
  seconds for company; this kind is announced at once, since its limit is running;
- `internal/inbox/serve.go`, `batch.go`, `reservation.go` — through `IsReport`: a letter
  that cannot be delivered is not kept readable as a report is; it fails, and the
  worker's wrapper falls back at once;
- `internal/cli/send_kinds.go`, `send.go` — no `messageKind` and no line in `sendKinds`:
  `send` cannot write it;
- `internal/cli/send_to.go` refuses an addendum to it already, as to any kind that asks for
  no work (exit 1); `edit.go` and `withdraw.go` refuse it too, with exit 1 — the worker is
  its sender on paper, but its wrapper wrote it; `internal/inbox/withdraw.go` records a
  withdrawn message's kind and needs nothing;
- `internal/cli/send_question.go` — a blocked `--question` takes only a report that
  settles it (`inbox.AwaitAnswer`), so the letter is never mistaken for the answer; but
  the wait runs up to ten minutes, past the limit, so a main blocked on a question to this
  worker is told of the request: the wait ends with exit 3, printing the request and its
  command; the answer to the question still arrives as a report;
- `internal/cli/turn_reports.go`, `completion.go`,
  `internal/harness/claude/telemetry/collector_turns.go` — a turn's outcome is never this
  kind; nothing to add;
- `internal/harness/notice.go`, `claude/notice.go`, `codex/notice.go` — the notice line
  names a permission request, not a task;
- `internal/harness/codex/server_delivery.go` — not work: steered into a running turn as a
  heads-up is;
- `internal/inbox/awaited.go`, `held.go`, `sent.go`, `recall.go` and
  `internal/cli/inbox*.go`, `sent_ref.go`, `sent_current.go` — rendered with its live
  state; never listed as awaited or owed; not delayed while main is in a turn.

Letters a wrapper already writes by itself — a departure, a compaction, availability
(`internal/wrap`) — are notes with a field of their own, not kinds. The request could be
one too: a note owes nothing and is no report without touching the places above, but it
would wait in the coalescing window and read as a note in every list that does not look
at the field. A kind was chosen for the notice line and the rendering; either works.

## The command

```text
rewake permission <id> allow
rewake permission <id> deny ["reason"]
rewake permission <id> owner
```

In the "Steer a session" group, main only. It asks main's own wrapper to register the
decision and waits up to 5 seconds for the worker's wrapper to take it.

- **Exit 0** — the worker's wrapper took it: the harness got allow or deny, or the prompt
  was left to the person.
- **Exit 1** — refused: the request was settled first (answered at the keyboard, by the
  limit, by an interrupt, the worker's run ended — the refusal names which, and for the
  limit, the next step: a task with `--grant-dir`); the worker's wrapper did not take it
  in time; the decision was not confirmed.
- **Exit 2** — a wrong call: not a verified main, a Codex main, no such request, a
  decision word missing or unknown, a reason on `allow`, `allow` on a request marked for
  the owner.

## Races

- **The person answers first.** Seen on 2.1.280: the dialog closes and the call runs as
  the person chose; the hook is not stopped and its later answer is ignored. Nothing
  tells the hook, and the payload carries no tool-use id, so the wrapper learns it from
  what follows, by tool and input. An allow runs the call, and its `PostToolUse` or
  `PostToolUseFailure` says so — events rewake's telemetry does not register today
  (`telemetry.HookEvents`), so building this adds them. A plain "no" aborts the turn,
  which ends the hook as Esc does. A "no" with feedback lets the turn go on and fires no
  hook for that call; the next event of the session or the turn's end is the latest
  point it shows. Until then an answer from main is taken and changes nothing. On Codex
  the server's `serverRequest/resolved` says it at once.
- **Esc or `rewake interrupt`.** Seen on 2.1.280: Esc on the dialog rejects the call,
  ends the turn and sends the hook SIGTERM within a tenth of a second; the wrapper sees
  the connection close and settles the request as interrupted. `rewake interrupt` aborts
  the turn through the plugin, the same abort — inferred, not seen. On Codex
  `turn/interrupt` clears the request and `serverRequest/resolved` follows.
- **A second request while one waits.** Each has its own id, letter and limit. At most
  four are held per session; a fifth is refused at once with the next step.
- **The worker's wrapper ends.** The hook's connection fails, the hook is silent, and the
  prompt stays for whoever is at the keyboard; main's answer is refused, naming it.
- **Main's run ends**, or the room's main changes, while a request waits: the worker's
  wait on main's wrapper breaks, and the request falls back at once with the next step.
  The report of that turn is `pending` and reaches whoever is main next.
- **Main answers after the limit**: refused with exit 1; the worker has been told to end
  the turn with `rewake pending`, and a grant task is the way on.

## What is recorded where

- The request, its deadline and its outcome live in the worker's wrapper's memory; that
  is what decides.
- A copy under the room's `permissions/<session>/` is what `rewake list` and the letter's
  rendering show, as the grants copy is: a worker can rewrite it, and changes only what is
  shown.
- Main's decision lives in main's wrapper's memory until the worker's wrapper takes it.
- The letter to main is ordinary mail; the wrapper's event log records each request and
  its outcome.

## Security

A decision is one the worker cannot forge, by the scheme grants already use
([grants-authority.md](grants-authority.md)). `rewake permission` registers the decision
with main's wrapper, which takes it only from a process below itself; the worker's wrapper
waits on main's wrapper for it and believes only the process main's run names, in its own
namespaces. Nothing written to the state directory counts: a sandboxed Codex worker can
write its own control files, and an answer read from a file would let it approve itself.
The worker's wrapper takes a decision only for a request it raised, from the main that was
current when it raised it.

Main answers within its rights. `rewake permission` runs under the `Bash(rewake:*)` rule
main's launch adds when its caller passed no `--allowedTools` of their own, so main's
permission mode and classifier never see what the worker asked for: main's role text carries the rule — allow only what main would do itself, and
answer `owner` for the rest. A worker with no sandbox has no boundary to hold here either:
it can do what it asked for another way, as with grants.

## The live probe on Claude Code

Run on September 28, 2026 on 2.1.280, by a session with no person: an interactive session
in tmux, working directory `/tmp/rwprobe/ws`, `--permission-mode default`, a `--settings`
layer with only a PermissionRequest command hook that logs its payload, sleeps and
answers deny with `probe: ask main`. Four model turns on the cheapest model; nothing of
rewake ran. A write to `/tmp/rwprobe/out` was asked for each time.

- **Hook sleeping 25 s, timeout 60.** The dialog showed at once, with the hook already
  running. At 25 s the dialog closed by itself; the tool line read `Error: probe: ask
  main` and `Denied by PermissionRequest hook`; the model quoted the message and ended the
  turn.
- **The person answers first.** Option 1 pressed three seconds into the hook's sleep: the
  file was written and the turn ended. The hook was not stopped; it answered deny at
  25 s, and nothing changed.
- **Esc three seconds in.** The call was rejected (`User rejected write to ...`), the turn
  ended, and the hook got SIGTERM 90 ms after the key.
- **The payload** carries `session_id`, `prompt_id`, `cwd`, `permission_mode`,
  `tool_name`, `tool_input` and `permission_suggestions`
  (`setMode acceptEdits`, `addDirectories` of the file's parent), but no tool-use id.
- **Hook sleeping 660 s, timeout 900.** At 610 s, past the 600 s default, the hook was
  still running and the dialog still on the screen. The hook answered 680 s after it
  started — its `sleep 660` ran late by 20 s — and the dialog closed with the same
  `Error: probe: ask main`, which the model quoted. A hook's own `timeout` above 600 s
  holds the call that long.

## Open questions for the owner

What the sources settle is said with each; what is left is the owner's choice.

- **The limit**: 120 s proposed, with a hook timeout of 180 s. Nothing in either harness
  bounds it — Claude Code holds a hook for its own `timeout`, Codex waits for a reply
  without end — so the number is only rewake's.
- **`owner`** leaves the prompt with the person unbounded, as today; with nobody at the
  keyboard the task stalls again, only with main knowing why. The other way is a deny
  whose next step is `rewake pending "needs the owner to allow <what>"`.
- **The owner's list**: the recognized deletions, force pushes and protected paths above,
  or a shorter one with main's judgment for the rest.
- **A worker under a Codex main**: settled that a Codex main cannot answer — its sandbox
  refuses the socket ([grants-authority.md](grants-authority.md)). Left: its workers'
  requests behave as today, or are refused at once with the next step.
- **Auto mode**: settled that a classifier refusal never becomes a PermissionRequest; a
  worker in auto mode reaches main only through the fallbacks above. Left: whether
  rewake does anything with a `PermissionDenied` hook — tell main after the fact, say — or
  leaves auto mode as it is.
- **MCP tools and `WebFetch`**: settled that both reach PermissionRequest on Claude Code;
  on Codex an MCP call is approved through an elicitation, left to the person either
  way. Left: held for main as proposed, or left to the person.
- **Codex: the relay's reply or Codex's own hook.** The reply keeps the prompt on the
  screen, so the person can answer first, as the owner decided, but a decline says
  nothing, and the next step rides on a steer. The hook gives the model the next step in
  the deny itself, but hides the prompt while it holds, and runs only as a trusted hook.
  The same trade on Claude Code already applies to a background subagent's requests:
  hold them, hiding the prompt, or leave them to the person.
