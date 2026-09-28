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

On September 27, 2026 main decided that a grant comes back after a cold resume only when
main's wrapper confirms it again ([after a cold resume](#after-a-cold-resume)).

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
`rewake edit <id>`, which replaces the task and carries its grant over. What it carries
is what main's wrapper holds for the task, never what the letter on disk says: a worker
can rewrite its own unread mail, and an edit would otherwise register a wider grant in
main's name. A letter naming a grant the wrapper does not hold is refused with exit 1,
pointing to `rewake withdraw` and a new send.

Each directory is resolved the way the harness will see it: a relative path from the
sender's directory, cleaned, links resolved. It must exist and be a directory, or the
send fails with exit 1. The resolved path is what the message carries, and the send
names it (`Rewake: grants <worker> write access to: <dir>`), so main sees where a link
led. A path that goes through a shared temporary directory, `/tmp` or `$TMPDIR`, to
somewhere outside it is refused with exit 2, the refusal naming where it leads: a
sandboxed worker can write there and put a link in place before main names it. Every
step of the resolution counts, not only the name given: a link of the owner's that
leads to `/tmp/out`, which leads on to the owner's notes, is refused as well. A directory that
lies in the recipient's workspace already is not carried and is named in the output as
already writable (`alreadyWritable` in `--json`); a directory inside another granted one
goes with it, so the message carries only the outermost.

Paths are compared by elements through `filepath.Rel`, never as string prefixes, and a
protected directory is resolved as far as it exists, so a link does not hide it. Under
`/mnt/<letter>` the comparison ignores case, as a Windows drive does, and a Windows
short name such as `PROGRA~1` is read back to its long name: the drive answers both for
one directory, and lists only the long one. The long name is found by the inode in the
parent's listing; a short name that matches none is refused with exit 2, since every
check compares long names. Checked live on WSL: `/mnt/c/PROGRA~1` and
`/mnt/c/Program Files` report one inode.

### The hard tier

A directory that is, lies inside or contains one of these is refused with exit 2,
whatever flag names it:

- rewake's state directory, `~/.config/rewake` and the directory of the rewake binary;
- each harness's own configuration, named by the harness through `ProtectedDirs()`:
  `$CODEX_HOME` or `~/.codex` for Codex; `$CLAUDE_CONFIG_DIR`, `~/.claude*` and
  `~/.local/share/claude` for Claude Code;
- `~/.ssh`, `~/.gnupg`, `~/.aws`, `~/.kube`, `~/.docker`, `~/.password-store`,
  `~/.config/gh`;
- every directory on the sender's `PATH`, and for one named `bin` the toolchain it
  belongs to — a Go, Node or Rust installation, a `GOPATH` — whose libraries its
  programs load; not `/`, the home directory or `~/.local`, which hold far more;
- what the owner's tools load and run unchecked: `~/go/pkg/mod`, `~/.cache/go-build`,
  and `$GOMODCACHE` and `$GOCACHE` when set;
- the shared temporary directories, `/tmp` and `$TMPDIR`: the Codex sandbox lets its
  commands write both by default, so the worker could swap a directory granted there for
  a link ([below](#what-a-grant-does-not-stop));
- `~/.config/git`, `~/.config/systemd`, `~/.config/autostart`, `~/.config/environment.d`,
  `~/.config/fish`, `~/.local/share/systemd`, `~/.local/share/applications`;
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
- a directory with `.git`, `.claude`, `.codex` or `.agents` anywhere in its path, in
  any case. A checkout keeps its metadata there and a harness its configuration, and
  what reads them next runs what was written. Inside a grant Claude Code keeps them with
  the person ([grants-claude.md](grants-claude.md)); granted as the root, or below one,
  nothing inside would, so the grant names them. The metadata `--grant-git` adds beside
  a granted checkout is what that flag is for, and passes the hard tier alone;
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

Only a verified main, and nothing on disk proves it. Main's wrapper registers each grant
from a process running below itself, holds it in memory while its task is open, and
confirms it at delivery only to the recipient's wrapper, told apart by pid, start time and
namespaces; the recipient takes the answer only from the process main's run names. A
worker in the Codex sandbox cannot grant itself one, a worker with no sandbox has no
boundary to hold, and a Codex main cannot grant at all. The scheme, and what it holds
against and what not, is in [grants-authority.md](grants-authority.md).

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
refused a second time. Each entry names the conversation and the main run it came from.
The copy of a run that has ended is removed at the next save of any journal in the room,
unless it names a live grant whose main still runs: a resumed run reads it
([after a cold resume](#after-a-cold-resume)).

`rewake inbox --owed` repeats a task's grant with a caveat — `grant: write <dir> — unless
a turn typed in this session's terminal has dropped it since` — because rewake learns of
a drop only when it comes to take the grant back; once the journal says a directory was
revoked or dropped, the line says so instead.

A task is settled once its reader has read it and reported on it, its sender took it
back, or it failed to arrive. A status saying taken back or failed settles it whatever a
wait record says: such a task was never read, and a wait naming it is one the worker
wrote to keep its grant. At every delivery to the session after that, while the journal holds live entries of
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

A cold resume starts a new run; what it keeps of a grant is in
[after a cold resume](#after-a-cold-resume).

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
- **What the hard tier does not name.** The tier is the places rewake knows to run as
  the owner; it is not every one. A shell's startup directory other than those named, an
  editor's plugin directory, a toolchain whose programs are not on `PATH` are granted as
  any other directory. The send names each directory as resolved, and reading it is
  main's part.
- **The shared metadata of a worktree.** `--grant-git` beside `--grant-dir` on a linked
  worktree would add the repository's common `.git`, hooks and configuration included,
  which every checkout of it runs — main's too, outside any sandbox. For a worktree of
  another repository than the worker's own it is refused. For one of the worker's own
  repository it is added as before: the worker could write that common `.git` from its
  own checkout already, and the hooks it writes run in main's checkout just the same
  ([git-grants.md](git-grants.md)).

## Claude Code

Claude Code has no sandbox, so a grant goes through its permission hooks. Rewake's
`grant-hook` asks the worker's wrapper, which keeps the confirmed grants in memory; the
first file-tool write inside a grant adds its root as a working directory, `.git` and the
harness directories inside it stay with the person, and after the report a forced
question takes the root out. It spares the worker prompts and draws no line. How each
step works, and in which permission modes, is in [grants-claude.md](grants-claude.md).

## After a cold resume

A resumed run asks main's wrapper to confirm again each grant the journal copies name
for its conversation; main hands one over while its task is open. Claude Code is started
with the directory; Codex has the roots journaled again, and one nobody confirms taken
out. A main that has ended restores nothing ([grants-resume.md](grants-resume.md)).

## At launch

Beside `--remote` the Codex terminal from 0.156 on refuses `--add-dir` and a
`sandbox_workspace_write.writable_roots` override and exits 1 before its first request
(live on 0.157.1; 0.155.1 took both). `rewake codex` refuses them first, with exit 2 and a
pointer to `--grant-dir` ([traps.md](traps.md#rewake-codex---add-dir-exits-1-on-codex-0156-and-later)).
