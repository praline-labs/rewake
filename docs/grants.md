# Directory grants

`rewake send <worker> --grant-dir <dir> "task"` lets a worker write a directory outside
its workspace for one task. It is `--grant-git`'s sibling ([git-grants.md](git-grants.md)):
main decides per task, rewake carries the decision with the message, and the harness
adapter applies it when the task is delivered. Codex takes a grant as a workspace root
of its sandbox; Claude Code, which has no sandbox, takes it through its permission hooks
and is spared the prompts, not fenced in ([Claude Code](#claude-code)). Main's wrapper confirms every grant at delivery, so a worker inside the
Codex sandbox cannot grant itself one ([who can grant](#who-can-grant)).

## What the owner decided

On September 27, 2026 the owner decided:

- the flag is `--grant-dir`, repeatable; `--grant-git` stays a flag of its own, and the
  two combine for a worker that writes and commits in a neighbouring checkout;
- every role may receive a grant, general included; only a verified main sends one;
- a grant lives at least until the task is reported on, and rewake takes it back at the
  next delivery after that; a person's own turn in the terminal drops it at once, and a
  cold resume of the conversation can bring it back;
- a task with a grant that arrives during a turn waits until the worker is idle, so the
  grant holds from its first turn; the same holds for `--grant-git`;
- two tiers of refusal, below: a hard tier nothing grants, and a broad tier granted only
  when main confirms it by name with `--grant-dir-broad`;
- a grant is for writing; what a session may read stays its harness's decision;
- `rewake codex --add-dir` and a `writable_roots` override are refused at launch, with a
  pointer to `--grant-dir`.

Later on September 27, 2026, after a review found that a worker could write a grant in
main's name into the shared state directory, the owner decided:

- a grant is one the worker cannot forge: main's wrapper registers it and confirms it
  at delivery, and nothing on disk counts as proof ([who can grant](#who-can-grant));
- a Codex main cannot grant for now, `--grant-dir` and `--grant-git` both, since its
  sandbox cannot reach its wrapper; how it could is queued
  ([work-queue.md](work-queue.md#also-queued-not-scheduled));
- no subreaper in the wrapper: the namespace check holds a sandboxed Codex worker, and a
  session run with Codex's legacy Landlock backend, which drops those namespaces, takes
  no grant at all;
- the model is written down as it is: it holds against a worker in the Codex sandbox,
  and a worker with no sandbox has no boundary to hold.

## Sending

```text
rewake send writer-codex --grant-dir ../shared-lib "Bump the client in shared-lib as well"
rewake send writer-codex --grant-dir-broad /mnt/d "Sort the exports on the D: drive"
rewake send writer-codex --grant-dir ../shared-lib --grant-git "Commit the bump there too"
```

Both flags repeat, at most 8 directories together. A grant goes only on a task or a
question, sent by a verified current main; a heads-up, a report, an addendum (`--to`),
an empty value and a sender that is not main are refused with exit 2. A recipient whose
harness cannot take a directory into a running session is refused with exit 1 and told
what to do instead. So is a send whose own wrapper does not register the grant — a
Codex main, a command not run below main's wrapper — and nothing is written then.

A task carrying a grant waits at most 30 minutes, its time to live, for the worker to be
idle; past that it expires and main is told. While it waits, `--to` refuses to add to
it — the addition would be read first and worked on without the grant — and points to
`rewake edit <id>`, which replaces the task and registers its grant again.

Each directory is resolved the way the harness will see it: a relative path from the
sender's directory, cleaned, links resolved. It must exist and be a directory, or the
send fails with exit 1. The resolved path is what the message carries. A directory that
lies in the recipient's workspace already is not carried and is named in the output as
already writable (`alreadyWritable` in `--json`); a directory inside another granted one
goes with it, so the message carries only the outermost.

Paths are compared by elements through `filepath.Rel`, never as string prefixes, and a
protected directory is resolved as far as it exists, so a link does not hide it. Under
`/mnt/<letter>` the comparison ignores case, as a Windows drive does.

### The hard tier

A directory that is, lies inside or contains one of these is refused with exit 2,
whatever flag names it:

- rewake's state directory, `~/.config/rewake` and the directory of the rewake binary;
- each harness's own configuration, named by the harness through `ProtectedDirs()`:
  `$CODEX_HOME` or `~/.codex` for Codex; `$CLAUDE_CONFIG_DIR`, `~/.claude*` and
  `~/.local/share/claude` for Claude Code;
- `~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.kube`, `~/.docker`, `~/.password-store`;
- every directory on the sender's `PATH`;
- the shared temporary directories, `/tmp` and `$TMPDIR`: the Codex sandbox lets its
  commands write both by default, so the worker could swap a directory granted there for
  a link ([below](#what-a-grant-does-not-stop));
- `~/.config/git`, `~/.config/systemd`, `~/.config/autostart`, `~/.config/environment.d`;
- the system directories: `/etc`, `/usr`, `/bin`, `/sbin`, `/lib*`, `/boot`, `/dev`,
  `/proc`, `/sys`, `/run`, `/var`, `/opt`, `/root`, `/srv`, `/snap`; and `/` itself, which
  every path lies inside, only as itself and as what it contains.

"Contains" is what keeps `$HOME` and `/home` out: each holds `~/.ssh`. A worker that
truly needs one of these gets it from the owner — for Claude Code, `--add-dir` at its
launch or `/add-dir` in its terminal; for Codex, a launch in that directory — or main
does that part of the work itself.

"Contains" also decides the Windows drives under WSL. WSL appends the Windows `PATH` to
the Linux one, so directories such as `/mnt/c/Windows/System32` and the user's
`AppData/Local/Microsoft/WindowsApps` are on it: `/mnt/c` and the Windows user's home
contain them and fall in the hard tier, not the broad one. A directory beside those —
a project under `/mnt/c/Users/<name>/src`, say — is granted as any other.

### The broad tier

Matched exactly, and granted only with `--grant-dir-broad <the same path>`:

- `/home`, `/mnt`, `/media`;
- each mount point under `/mnt` and `/media`, read from `/proc/self/mountinfo`;
- each directory directly in `$HOME`;
- each `~/.config/<app>` with a `credentials` file directly inside. Writing there does
  not raise an agent's own powers, so the owner chose to confirm it rather than refuse;
- a directory where a live rewake session works, or one that holds it — main's own
  checkout among them. Its `.claude/`, `.mcp.json` and `.rewake.toml` tell that session's
  harness and rewake what to run, so a grant there reaches past the task into the other
  session. Unlike the rest of the tier this one matches what contains the directory
  too. Read from the registry at send and again at delivery; a directory confirmed when
  sent is not refused at delivery because its session has ended since. The registry is
  a file a sandboxed worker can rewrite, so this guards against main's mistake, not
  against the worker.

The same files in a directory no session works in are not recognized: a grant of a
checkout nobody runs in opens its `.claude/` and `.mcp.json` for writing, and the next
session started there runs what the worker put in them. Grant the directory the task
needs, not the checkout above it, when the checkout is one a session will be started in.

A directory of the broad tier under `--grant-dir` is refused with exit 2, the refusal
naming the `--grant-dir-broad` call; `--grant-dir-broad` on a directory that is not broad
is refused the same way, so a confirmation always names what it confirms. The hard tier
wins where both match.

## Who can grant

Everything a message carries is written by whoever wrote the file, and the state
directory — registry, mailboxes, journals — is writable by a Codex worker in
`workspace-write`, since it lies in `/tmp`. So nothing on disk says who sent a grant.
What does is main's wrapper, a process outside every sandbox, asked over an abstract
unix socket named from the room and main's run (`@rewake/grant/<hash>`, no file to
replace or leave behind). The wrapper binds it before it writes its record, so from the
moment the address can be worked out it is taken; a `rewake send` registers only with a
listener that runs above it, so a listener there first would not take a grant and have
send report it as registered.

1. **At send.** `rewake send` or `rewake edit` with a grant registers it with its own
   session's wrapper before the letter is written. The wrapper takes it only from a
   process below itself: the caller's uid and pid from `SO_PEERCRED`, then its chain of
   parents up to the wrapper, each no younger than its child, so a pid taken again on the
   way ends the walk; and it must share the wrapper's mount, user and PID namespaces. A
   worker's process runs below its own wrapper and not main's, unless main started it;
   one that left its tree through `setsid` and a double fork runs below neither. The
   wrapper holds the grant in memory — message, recipient, the recipient's run,
   directories, broad ones, Git — for the task's time to live and five minutes more, at
   most 256 at once; past that it refuses a new one rather than forget an old one.
2. **At delivery.** The recipient's wrapper asks the address the letter's run leads
   to, and takes the answer only from the process that run names: the
   listener's pid from `SO_PEERCRED` must be that run's pid, alive with that start time
   before and after the answer, and in the same mount, user and PID namespaces as the
   asking wrapper. The grant confirmed must be the one the letter carries, for this
   recipient and this run. A letter main never registered, one naming a run it is not,
   one answered from another process fail, and main is told.
3. **When main cannot answer.** While main's wrapper is alive and does not answer, the
   task stays pending and is asked again until it expires. Once that wrapper has ended,
   nobody can confirm the grant, now or later: the task fails, naming the main that sent
   it, and has to be sent again from the current one.

What this holds against rests on the peer checks, not on the address being hidden. A
Codex sandbox with its network on shares the host's network namespace and sees every
`@rewake` name ([research-codex.md](research-codex.md#the-sandboxs-namespaces)); only
with the network off is it cut off from them. So the wrapper checks who is at the other
end both ways — the pid, the start time and the mount, user and PID namespaces — and the
namespace check at registration is what keeps a sandbox main started from granting.

What this holds against, and what it does not:

- **A worker inside the Codex sandbox.** Codex runs a command in a mount, user and PID
  namespace of its own, in `workspace-write` and `read-only` alike, and `setsid`, a
  double fork or `nohup` do not change them
  ([research-codex.md](research-codex.md#the-sandboxs-namespaces)). Its process is below
  no wrapper, so it registers nothing; a listener it starts in a run it names itself is
  in other namespaces, so nothing it confirms counts. This is what the scheme is for.
- **A sandbox main started itself.** A Codex that main runs from its own shell — for a
  live probe, as this repository's instructions do — runs below main's wrapper, and so
  does every command of its sandbox; with network allowed, one could reach main's
  address, register a grant for any recipient and put the letter in a mailbox itself.
  Its commands are in namespaces of their own, so the wrapper refuses the registration
  as it refuses a confirmation from them.
- **A worker's code main runs.** `go test` on a worker's branch, run by main, runs below
  main's wrapper in its namespaces and could register a grant. It is no new hole: code
  main runs outside a sandbox can do whatever main can, a grant included.
- **Codex's legacy Landlock backend** runs sandboxed commands in the app-server's own
  namespaces. It is off by default and never chosen as a fallback, but a session whose
  launch arguments or `config.toml` mention `use_legacy_landlock` takes no grant. The
  refusal is the recipient's: other sessions of the room take grants as before.
- **A worker with no sandbox** — Claude Code, whose approved shell commands run as the
  user — has no boundary at all. It can start a listener in the wrappers' own namespaces,
  write a registry record naming that listener's run as a main, and send a letter
  pointing at it; nothing in rewake tells that listener from a wrapper. It is not a new
  hole: such a worker writes wherever the user can anyway, and a grant on Claude Code
  removes prompts rather than draws a line.
- **A Codex main** cannot grant: its own sandbox refuses `connect()` on a unix socket
  ([research-permissions.md](research-permissions.md)), so its `rewake send` never
  reaches its wrapper. `--grant-dir` and `--grant-git` from a Codex main are refused with
  exit 1; before this change `--grant-git` from one was accepted
  ([git-grants.md](git-grants.md)).
- **A Claude Code main** registers through its shell, which shares the wrapper's network
  namespace today. A Claude Code sandbox with network isolation turned on would cut the
  shell off from the abstract socket as well, and the send would fail with exit 1 naming
  the wrapper it could not reach; not observed, since the sandbox is off here.

## Delivery

Before a grant task becomes readable, the wrapper serving the recipient checks every
directory again with the recipient's own view (its `HOME`, its `PATH`, the harness
directories, the sessions registered now) and requires the path to resolve to itself: a
link swapped in since the send fails it. Then it asks the wrapper of the main that sent
the task to confirm the grant ([who can grant](#who-can-grant)). A task whose grant does
not pass either check is not delivered without it: it fails, and its sender gets a note
with the reason. An expired grant task fails the same way, without asking the harness
anything.

A message carrying a grant — a directory or `--grant-git` — goes on a notice of its own.
The Codex adapter reads the thread before the task becomes readable, and while the thread
is active the task stays pending (`inbox.ErrNotYet`), retried every two seconds for up to
its time to live. Mail after it goes on without it and may reach the reader first. A
read that fails keeps a directory grant pending the same way; a thread with no single
local environment fails it. For `--grant-git` alone either delivers with a diagnostic,
as before.

A session started with Codex's legacy Landlock backend — `use_legacy_landlock` mentioned
in its launch arguments or its `config.toml` — takes no grant: every task carrying one
fails, naming the setting ([who can grant](#who-can-grant)).

On an idle thread the adapter sends `turn/start` with `runtimeWorkspaceRoots`: the
snapshot as read, less what rewake granted for tasks settled since, plus each granted
directory no root covers already. The sandbox and every other root stay as they are. With
`--grant-git` beside it, each granted directory that is a Git checkout gets its metadata
added too, checked against the hard tier, since metadata can lie outside the checkout.

The outcome is in the delivery status: `grantApplied` lists the directories the session
can now write, and `detail` says what was added, what was already writable and what was
taken back. The reader's `rewake inbox` prints `grant: write <dir>` above the task's
text, and `rewake inbox --owed` repeats it while the task is owed.

There is a narrow race left: the terminal can start a turn between the adapter's read and
its `turn/start`. The server then steers the notice into that turn, and the grant holds
for the turns after it — the same as `--grant-git` on an active thread before this change.

## Taking a grant back

What rewake adds it journals per session run: the path, the task's id, the time and the
outcome — `granted`, `revoked` when rewake took it out, `dropped` when rewake found it
gone already. The journal the adapter revokes by is held in the recipient wrapper's
memory; a copy under the room's `grants/` directory is what `rewake list --json` shows
as `grants` on each session, and a worker that rewrites or removes that copy changes
only what is shown. A directory some root covered before the grant is not journaled:
rewake did not give it, so it does not take it back. Metadata of a granted checkout is
journaled with `for` naming that checkout.

A live entry is never dropped: it is how rewake finds a grant to take back. At most 64
directories are granted to one run at once, and a task that would pass that fails with
the reason instead; ended entries beyond 64 go oldest first. A message granted once is
refused a second time. The copy of a run that has ended is removed at the next save of
any journal in the room.

`rewake inbox --owed` repeats a task's grant with a caveat — `grant: write <dir> — unless
a turn typed in this session's terminal has dropped it since` — because rewake learns of
a drop only when it comes to take the grant back; once the journal says a directory was
revoked or dropped, the line says so instead.

A task is settled once its reader has read it and reported on it, or its sender took it
back. At every delivery to the session after that, while the journal holds live entries of
settled tasks, the adapter reads the snapshot and sends it without their directories. A
directory another live task still holds stays until that task is settled too. Nothing
runs between deliveries: a grant whose task is settled lives until the next message to
that session.

## How long a grant lives

At least until the task is reported on, and in practice until the next delivery after the
report. It ends earlier in two ways the owner accepted, both observed live on Codex 0.155.1
and 0.157.1 ([research-codex.md](research-codex.md#runtime-workspace-roots)):

- a person's own turn in the terminal sends only the launch directory, which replaces the
  roots and drops the grant; rewake records it as dropped when it comes to take it back;
- a cold fork of the conversation starts without it.

A cold resume, on the other hand, restores the roots the conversation last saved, grant
included; rewake's journal belongs to the run, so the resumed run does not take it back.
Keying it by the thread instead is queued
([work-queue.md](work-queue.md#also-queued-not-scheduled)).

## What a grant does not stop

- **A link swapped in after delivery.** The recheck at delivery requires each directory
  to resolve to itself, but Codex may resolve a root again on each command, and a worker
  that can write the directory's parent can put a link in its place:
  `mv /tmp/x /tmp/x.old; ln -s ~/.ssh /tmp/x`. Rewake narrows it where it can see the
  parent is writable: a directory in `/tmp` or `$TMPDIR` is in the hard tier, and one
  inside a directory granted to the same run for another task is refused at delivery.
  A parent the worker can write for some other reason — its own workspace is covered,
  a directory the owner gave it is not known to rewake — is not caught. Whether Codex
  resolves a root once or on every command is an open question, to be probed.
- **What lies inside a granted directory.** A grant is for the whole tree: a `.claude/`,
  `.mcp.json` or `.rewake.toml` inside it is writable too, and `.git/hooks` when
  `--grant-git` rides along — Codex keeps a checkout's `.git` read-only under a writable
  root otherwise — and whatever reads them next runs what the worker wrote. Only a directory where a live session
  works is recognized ([the broad tier](#the-broad-tier)).
- **The shared metadata of a worktree.** `--grant-git` beside `--grant-dir` on a linked
  worktree would add the repository's common `.git`, hooks and configuration included,
  which every checkout of it runs — main's too, outside any sandbox. For a worktree of
  another repository than the worker's own it is refused. For one of the worker's own
  repository it is added as before: the worker could write that common `.git` from its
  own checkout already, and the hooks it writes run in main's checkout just the same
  ([git-grants.md](git-grants.md)).

## Claude Code

Nothing adds a working directory to a running Claude Code session from outside, but its
permission hooks can ([research-claude-actions.md](research-claude-actions.md#a-directory-given-to-a-running-session)).
Rewake's settings layer adds `rewake grant-hook` as a PreToolUse hook on the file tools
and as a PermissionRequest hook, each with a five-second timeout.

- **Where the grant lives.** Once the grant is confirmed ([who can grant](#who-can-grant)),
  the recipient's wrapper keeps it in memory. The hook holds nothing: it is a child of
  the harness, so it runs below that wrapper, and it asks the wrapper over an abstract
  socket named from the room, the session and its run (`@rewake/keep/<hash>`). The
  wrapper answers only a process below itself, checked as registration is; the hook
  believes only the process its run names. It sends the call's path fields, never the
  content a Write carries. A file in the state directory decides nothing, since a
  worker can write one; the copy under `grants/` is only what `rewake list` shows.
- **Delivery waits for an idle session.** A grant task stays pending while the session
  is working, retried every two seconds, as on Codex: a grant that arrived mid-turn
  would be taken back by a report the turn had not made.
- **Giving.** When the session first asks to write inside a grant, the PermissionRequest
  hook answers allow and adds the grant's root as a working directory for the session.
  From then on writes and shell commands inside it run without asking. A file tool is
  judged by its path; any other tool by what the harness suggests adding, and only when
  every suggestion lies in a grant, since an approved command runs whole.
- **What stays with the person.** `.git`, `.claude`, `.codex` and `.agents` at any depth
  inside a grant are never allowed, `--grant-git` or not (main's decision of September
  27, 2026): whatever reads them next runs what the worker wrote, and on Claude Code
  nothing but the prompt stands in the way. Once the grant is a working directory the
  harness would run a file tool anywhere in it unasked, so PreToolUse answers `ask` for a
  file tool writing there; a shell command inside the grant reaches them unasked all the
  same. A write outside every grant hears silence, and silence is also the answer to any
  error or timeout: the hook never allows on a guess.
- **Taking back.** At the first tool call after the task is settled, a read inside the
  working directory in `default` or `acceptEdits` is answered `ask` from PreToolUse; the
  PermissionRequest that follows is allowed with the grant's root removed. A file tool
  writing into a directory being taken back is denied with a line naming why; a shell
  command that mentions it hears silence and is left to the person. A directory another
  live task holds stays. A grant never used — never added — ends at the report.
- **What ends it early.** A cold resume starts without the directory, and the new run's
  wrapper does not restore it: a grant a file could restore is one a worker could forge.
  Restoring it by asking main's wrapper to confirm the grant again is left for later
  (main's decision of September 27, 2026).
  In `bypassPermissions` the hook is never asked, and the grant is moot.

This is a courtesy, not a boundary. Claude Code runs approved shell commands as the user,
so a worker writes wherever the user can if a prompt is approved, and can forge a grant
([who can grant](#who-can-grant)). What the hook adds is fewer prompts inside the
directory main named, and a prompt again once the task is done.

## At launch

Beside `--remote` the Codex terminal from 0.156 on refuses `--add-dir` and a
`sandbox_workspace_write.writable_roots` override and exits 1 before its first request
(live on 0.157.1; 0.155.1 took both). `rewake codex` refuses them first, with exit 2 and a
pointer to `--grant-dir` ([traps.md](traps.md#rewake-codex---add-dir-exits-1-on-codex-0156-and-later)).
