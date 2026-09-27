# Directory grants

`rewake send <worker> --grant-dir <dir> "task"` lets a worker write a directory outside
its workspace for one task. It is `--grant-git`'s sibling ([git-grants.md](git-grants.md)):
main decides per task, rewake carries the decision with the message, and the harness
adapter applies it when the task is delivered. Stage 1 covers the shared part and Codex;
Claude Code takes the grant in stage 2, and until then a grant to a Claude Code session
is refused.

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

## Sending

```text
rewake send writer-codex --grant-dir ../shared-lib "Bump the client in shared-lib as well"
rewake send writer-codex --grant-dir-broad /mnt/d "Sort the exports on the D: drive"
rewake send writer-codex --grant-dir ../shared-lib --grant-git "Commit the bump there too"
```

Both flags repeat, at most 8 directories together. A grant goes only on a task or a
question, sent by a verified current main; a heads-up, a report, an addendum (`--to`)
and a sender that is not main are refused with exit 2. A recipient whose harness cannot
take a directory into a running session is refused with exit 1 and told what to do
instead.

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
- `~/.config/git`, `~/.config/systemd`, `~/.config/autostart`, `~/.config/environment.d`;
- the system directories: `/etc`, `/usr`, `/bin`, `/sbin`, `/lib*`, `/boot`, `/dev`,
  `/proc`, `/sys`, `/run`, `/var`, `/opt`, `/root`, `/srv`, `/snap`; and `/` itself, which
  every path lies inside, only as itself and as what it contains.

"Contains" is what keeps `$HOME` and `/home` out: each holds `~/.ssh`. A worker that
truly needs one of these gets it from the owner — for Claude Code, `--add-dir` at its
launch or `/add-dir` in its terminal; for Codex, a launch in that directory — or main
does that part of the work itself.

### The broad tier

Matched exactly, and granted only with `--grant-dir-broad <the same path>`:

- `/home`, `/mnt`, `/media`;
- each mount point under `/mnt` and `/media`, read from `/proc/self/mountinfo`;
- each directory directly in `$HOME`;
- each `~/.config/<app>` with a `credentials` file directly inside. Writing there does
  not raise an agent's own powers, so the owner chose to confirm it rather than refuse.

A directory of the broad tier under `--grant-dir` is refused with exit 2, the refusal
naming the `--grant-dir-broad` call; `--grant-dir-broad` on a directory that is not broad
is refused the same way, so a confirmation always names what it confirms. The hard tier
wins where both match.

## Delivery

Before a grant task becomes readable, the wrapper serving the recipient checks every
directory again with the recipient's own view (its `HOME`, its `PATH`, the harness
directories) and requires the path to resolve to itself: a link swapped in since the send
fails it. A task whose grant no longer passes is not delivered without it: it fails, and
its sender gets a note with the reason. An expired grant task fails the same way, without
asking the harness anything.

A message carrying a grant — a directory or `--grant-git` — goes on a notice of its own.
The Codex adapter reads the thread before the task becomes readable, and while the thread
is active the task stays pending (`inbox.ErrNotYet`), retried every two seconds for up to
its time to live. Mail after it goes on without it and may reach the reader first. A
read that fails, or a thread with no single local environment, fails a directory grant;
for `--grant-git` alone it delivers with a diagnostic, as before.

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

What rewake adds it journals, per session run, under the room's `grants/` directory: the
path, the task's id, the time and the outcome — `granted`, `revoked` when rewake took it
out, `dropped` when rewake found it gone already. A directory some root covered before the
grant is not journaled: rewake did not give it, so it does not take it back. Metadata of a
granted checkout is journaled with `for` naming that checkout. `rewake list --json` shows
the journal as `grants` on each session.

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

## At launch

Beside `--remote` the Codex terminal from 0.156 on refuses `--add-dir` and a
`sandbox_workspace_write.writable_roots` override and exits 1 before its first request
(live on 0.157.1; 0.155.1 took both). `rewake codex` refuses them first, with exit 2 and a
pointer to `--grant-dir` ([traps.md](traps.md#rewake-codex---add-dir-exits-1-on-codex-0156-and-later)).
