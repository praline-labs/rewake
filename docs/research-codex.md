# Research: what a running Codex session shows

Split out of [research.md](research.md) on September 23, 2026, when that file passed the
project's 400-line limit again. The division is by subject: the facts here are obtained
the same way as there — by watching a session behave — and are about Codex CLI, while
research.md keeps Claude Code. What the generated protocol schema and the reference tree
state is in [research-protocol.md](research-protocol.md); the Codex sandbox is in
[research-permissions.md](research-permissions.md).

Tags: **[verified live]** — live on the named versions; **[source]** — read in the
source; **[docs]** — official pages. Facts here age with harness versions: recheck
before touching an adapter.

**[verified live; Codex CLI 0.155.1; September 21, 2026]** The installed CLI moved to
0.155.1 and the adapter's pin moved with it. Observed on that version: launch with
registration, a task accepted through the app-server, a `finished` report back and
telemetry collection ([claude-parity-2026-09-21.md](claude-parity-2026-09-21.md));
and, in a second probe, steer — a message sent inside the second of four sleep-10
calls, with the worker observed `working` in a snapshot taken just before the send,
reached the recipient at the next boundary between tools, the series continued and the
original task returned its own result. Not observed on this version: conversation
selection after `/new`, refusal of a stale target, `stopped` from a keyboard
interruption, and a multi-message group arriving during an active turn. One probe
each, no repeats and no race coverage. A group arriving during an active turn and
delivery after `/new` were observed on September 26, 2026
([research-codex-live-checks.md](research-codex-live-checks.md#live-messaging-checks-of-september-26-2026)).
Every fact below carries the version it was
taken on; a fact tagged 0.154.0 has not been re-checked on 0.155.1.
September 20: [native mailbox contract](native-mailbox.md) and
[owner/installed acceptance with evidence limits](native-mailbox-acceptance.md).

## Codex CLI

### Delivery: `codex queue`

- `codex queue --thread <id|exact name> --message <text>`, run from any process,
  places the message in a durable queue (`$CODEX_HOME/queue_1.sqlite`, method
  `thread/queue/add`). Every Codex process, including a plain TUI, polls the
  queue once every 10 seconds and starts a turn on its own if the thread is idle.
  **[source: ext/queue/src/service.rs]**
- An idle TUI wakes up without a daemon and without `--remote`, with a delay of
  about 10 seconds. **[verified live]**
- Mid-turn, the message waits and triggers the next turn once the current one
  finishes; after an explicit interrupt there's no auto-start. **[source]** Not
  verified live.
- **A brand-new thread with no messages yet cannot accept one**: the rollout file
  is created lazily, and `queue` responds with
  `no rollout found for thread id <id> (code -32603)`, exit code 1. **[verified live]**
- Unknown name: `No active session found matching '<name>'`, exit code 1.
  **[verified live]**

### Thread identity and terminal events

The server adapter uses thread/started and thread/closed, not TUI file
descriptors. Locks remain a fact of the standalone TUI, but under --remote the
server owns them. A root thread source/originator and absent parent id distinguish
TUI work from nested agents. The message sidecar still carries the delivery id.

Legacy notify emits normal completions but skips some error branches. The
adapter now consumes turn/completed: completed, failed with error.message, or
interrupted. Intermediate error notifications with willRetry=true are not final.
No transcript is read to obtain the result or infer a missing failure.

### Sandbox (Linux)

See the preserved [permission research](research-permissions.md#sandbox-linux).

### Git metadata writes by role

See the preserved [permission research](research-permissions.md#git-metadata-writes-by-role).

### Managed worktrees and continuation permissions

See the preserved [permission research](research-permissions.md#managed-worktrees-and-continuation-permissions).

### `--worktree` with a remote terminal

**[source: release tags `rust-v0.155.1` and `rust-v0.157.1`; verified live in a
disposable container without network, no model call; September 26, 2026]** Asked
whether rewake's refusal of `--worktree` could be lifted by passing the flag through its
gateway ([launch.md](launch.md#codex)).

- **The terminal refuses the pair itself.** Both versions exit 1 on `--worktree` with an
  explicit `--remote`, a local unix socket included, verbatim:
  ``Error: `--worktree` is only supported for local sessions``. The refusal comes before
  a checkout is created and before the terminal connects to any app-server
  (`tui/src/startup_orchestration.rs:17` in 0.155.1, `:27` in 0.157.1, relative to
  `codex-rs`). rewake always starts the terminal with `--remote`, so passing the flag
  through cannot work; the barrier is upstream.
- **In a supported local launch the terminal creates the checkout.**
  `worktree_startup::prepare` calls `WorktreeManager::create`
  (`tui/src/worktree_startup.rs:232` in 0.155.1, `:238` in 0.157.1), rebuilds the
  configuration with the checkout as its cwd, and hands the server that cwd and the
  workspace roots as ordinary `thread/start` parameters
  (`tui/src/app_server_session.rs:2048` / `:2017`). After the start it binds the
  checkout to the thread id through `codex-thread.json` in the git metadata
  (`worktree/src/metadata.rs:49`, `tui/src/app/startup.rs:34`).
- **A managed worktree is more than `git worktree add`**: a unique directory, a
  detached `--no-checkout` add, `config.worktree`, and a rollback of an allocation that
  did not complete. The allocation layout is in
  [research-permissions.md](research-permissions.md#managed-worktrees-and-continuation-permissions).
- **Nothing beyond the commit comes along.** **[source: `codex-rs/worktree/src/lib.rs`
  at `rust-v0.155.1` and `rust-v0.157.1`, read September 27, 2026, not run]**
  `WorktreeManager::create` — `git worktree add --detach --no-checkout`, then
  `config.worktree` and `reset --hard` (`lib.rs:61-165` in 0.155.1, `:62-166` in
  0.157.1) — carries neither untracked nor ignored files into the checkout, by copy or by
  link, and runs no preparation script. The only setting the terminal has for it is the
  root, `git-worktree-root` under `[desktop]` (`worktree/src/settings.rs:28-35`). Setup
  run when a checkout is made — "local environments" — belongs to Codex's desktop
  application; the terminal does not use it. rewake's `.worktreeinclude`
  ([worktree.md](worktree.md#files-git-ignores-worktreeinclude)) has no counterpart here.
- **How the checkout is made and removed.** **[source: `codex-rs` at commit `67a709665`
  and at `rust-v0.157.1`, read September 27, 2026, not run]** Making the checkout, git
  runs with hooks, the filesystem monitor and clean, smudge and process filters turned
  off, and with the inherited `GIT_*` variables cleared (`worktree/src/git.rs:32`,
  `:142`). The directory is reserved atomically — the last level made without `-p` —
  and a checkout that was not completed is rolled back (`worktree/src/lib.rs:62`).
  Removal asks nothing about commits a detached HEAD alone holds (`lib.rs:284`). A
  launch that fails after the checkout was made leaves it and prints how to recover
  (`tui/src/worktree_startup.rs:62`). What rewake took of this and what it did not is in
  [worktree.md](worktree.md#the-launch).

- **Trust follows the main checkout.** **[source: `codex-rs/git-utils/src/trust.rs`,
  identical at `rust-v0.155.1` and `rust-v0.157.1`, fetched and read September 26,
  2026]** `resolve_root_git_project_for_trust` walks up from the cwd to the nearest
  `.git`; when that is a file, it follows it to `<common>/worktrees/<name>`, requires
  that directory's `gitdir` to name the same checkout's `.git` and its `commondir` to
  lead to `<common>`, and that `<common>` is the main checkout's `.git`, and then
  answers the main checkout's root, the key its trust setting is kept under. So a
  linked worktree made by plain `git worktree add` takes the trust of its repository.
  rewake's own checkouts ([worktree.md](worktree.md)) have this layout;
  `internal/worktree` has a test that holds them to it. **[live, Codex 0.155.1 and
  0.157.1, synthetic history, no model call; September 26–27, 2026]** Seen in a rewake
  checkout on its branch: the terminal asked for no new trust, and `AGENTS.md` was found
  as in the main checkout.
- **A conversation begun in a rewake checkout continues there.** **[live, Codex 0.155.1
  and 0.157.1, synthetic history, no model call; September 26–27, 2026]** A session
  launched with `--worktree` from `src/nested` of a repository, then `rewake codex
  resume` run in the checkout's `src/nested` without the flag: the resumed thread's
  cwd, the session's registration and the workspace roots all named that directory of
  the checkout. The terminal sends `thread/resume` with `cwd: null` on both versions;
  0.155.1 sends the checkout's directory as `runtimeWorkspaceRoots`, 0.157.1 sends
  `null` there, and the server answers the right roots either way. So the way on that a
  refused `--worktree` with `resume` names ([worktree.md](worktree.md#the-launch)) works
  as it says. Seen in the Codex-side acceptance run of the worktree lifecycle
  ([2026-09-27-worktree-land.md](roadmap/2026-09-27-worktree-land.md)).

What the server accepts for it is in
[research-protocol.md](research-protocol.md#where-a-new-conversation-runs).

### Remote continuation permissions

See the preserved [permission research](research-permissions.md#remote-continuation-permissions).

### The sandbox has its own pid namespace

The commands a Codex agent runs are started under `bwrap --as-pid-1`, so inside
them `/proc` holds only the sandboxed process itself. `/tmp` is the real one —
a file written there appears outside — but every pid from outside is missing.
**[verified live]** Anything judging "is that process alive" from inside the
sandbox therefore concludes "no" about every session but its own. A reader has to
compare `/proc/self/ns/pid` before believing a pid it did not create.

### The sandbox's namespaces

**[verified live; Codex CLI 0.155.1 and 0.157.1; September 26, 2026]** Native
`command/exec` against a real app-server, no model calls, in disposable containers whose
seccomp filter was lifted so bubblewrap could create namespaces; evidence in
`~/.cache/rewake/evidence/2026-09-26/rewake-ns-probe/` (`README.md`,
`verification.json`). What the directory grant's confirmation rests on
([grants.md](grants.md#who-can-grant)):

- A sandboxed command runs in a mount, user and PID namespace other than the
  app-server's, in `workspace-write` and `read-only` alike, with the sandbox's network on
  or off.
- `setsid`, a double fork and `nohup` keep the command's namespaces, and each such child
  was gone before `command/exec` answered. Whether a process started by a model's tool
  call outlives that turn was not tested: no model was called.
- Seen from outside, `SO_PEERCRED` on a listener inside the sandbox gives a non-zero
  outer pid whose `/proc/<pid>/ns/{mnt,user,pid}` are readable and are the sandbox's.
  From inside, a listener outside shows pid 0.
- A listener: in `workspace-write` with `networkAccess=true` a command binds a unix
  socket in `/tmp` and the outside reaches it; with the network off `socket()` succeeds
  and `bind()` gives `EPERM`; in `read-only` binding a path gives `EROFS`.
- The legacy Landlock backend, `[features] use_legacy_landlock=true`, runs `read-only`
  commands in the app-server's own namespaces on 0.155.1, and refuses `workspace-write`
  with exit 101; on 0.157.1 it refuses both restricted modes. The release source makes
  it deprecated and off by default, chosen only by an explicit setting, and a failing
  bubblewrap does not fall back to it.

**[verified live by review-codex; Codex CLI 0.155.1 and 0.157.1; September 27, 2026]**
The network namespace follows `networkAccess`. With `networkAccess=false` the sandbox has
a network namespace of its own, and no `@rewake` abstract name is visible from inside.
With `networkAccess=true` it shares the host's, and the `@rewake` names listed inside
are the ones listed outside. So an abstract address is hidden from a sandbox only while
its network is off; the grant rests on the peer checks, not on the name.

### Environment and instructions

- `shell_environment_policy` inherits the environment by default, excluding names
  matching the patterns `*KEY*`, `*SECRET*`, `*TOKEN*`. **[source]** The tool's
  variable names must not match these patterns.
- The `developer_instructions` config key can be set per run via
  `-c developer_instructions="..."`; it overrides the user's value if one is set.
  **[source]**
- The first run in a new folder shows a trust dialog, confirmed with Enter; the
  decision is written to `~/.codex/config.toml`. **[verified live]**

### Session-owned app-server

**[snapshot 44b9011; CLI 0.154.0; September 17, 2026]** Foreground
app-server --listen unix://PATH supports a TUI via --remote. The listener uses
WebSocket frames, not JSONL; proxy is only a byte relay. turn/start calls
start_or_steer_turn (request_processors/turn_processor.rs:646–677), including
after an interruption. Basic turn/steer is stable and requires expectedTurnId;
queue/start is experimental and is not needed for immediate delivery.

**[owner live probes, 0.154.0]** Same-turn steering and fresh `/new` delivery both
reached the TUI and the model. (The probe transcripts lived in a temporary directory
that is long gone; the observation is what remains.)
A closed old thread can absorb input without a visible answer, and resuming an
empty thread did not reliably reply. Track closure; never deliver to a cached
id after losing evidence that it remains current.

**[source and no-model probes]** The research report behind this — kept in a
temporary directory that is long gone — recorded RPC initialization, failed and
interrupted outcomes, owned locks and the remote configuration boundary. TUI notify is not forwarded; developer instructions are
conditional on a feature there, so launch configuration reaches the server too.
TUI --add-dir is carried as runtimeWorkspaceRoots; -C and positional input are
forwarded. REWAKE_* survives default shell environment filtering.

The owned server uses CODEX_INTERNAL_APP_SERVER_REMOTE_CONTROL_DISABLED=1
(transport/remote_control/mod.rs:87) to keep this local launch off the persisted
remote-control path. This internal marker and the remote configuration rules
are version-specific. The later [installed native-mailbox acceptance](native-mailbox-acceptance.md)
closes the supported launch/delivery path; raw protocol probes alone did not do so.

### Runtime workspace roots

**[verified live; Codex CLI 0.155.1 and 0.157.1; September 26, 2026]** No-provider probes
against a real app-server in a disposable container, private HOME and CODEX_HOME;
evidence in `~/.cache/rewake/evidence/2026-09-26/rewake-grant-probe/`. What rewake's
directory grant ([grants.md](grants.md)) rests on:

- `turn/start` with `runtimeWorkspaceRoots` replaces the thread's roots; the snapshot
  shows the added directory on both versions. A later `turn/start` without the field
  keeps them, and so does a failed `thread/compact/start`. A successful compaction was
  not run.
- A symlinked root is kept in the spelling it was given, not resolved; so is a root that
  is `$CODEX_HOME` itself, which the server accepts. Rewake resolves a granted directory
  and refuses the harness's own configuration before sending.
- A person's turn in the terminal sends only the launch directory as the roots, and
  `thread/read` afterwards shows the added root gone.
- A cold fork (`thread/fork` on a fresh server) starts without the added root. A cold
  `thread/resume` without the roots field restores it when the grant was given on an
  established history; a grant on the first turn, whose turn then failed, was lost on
  cold resume on both versions.
- **[verified live; Codex CLI 0.155.1 and 0.157.1; September 28, 2026; review-codex, in
  acceptance of the grant restored after a cold resume]** What the terminal itself sends
  on a cold resume differs: 0.155.1's `thread/resume` carries roots of its own,
  `runtimeWorkspaceRoots=[cwd, existing]` as reported, without the saved grant, so the
  grant is gone before rewake's first notice, which then adds it back; 0.157.1's carries `null`, and the saved
  root stays.
- The persisted thread settings carry the whole native permission profile with the
  added root; whether a write under `workspaceWrite` succeeds there was not shown, since
  the container policy kept restricted execution from running.
- Beside `--remote` the terminal takes `--add-dir` and `-c
  sandbox_workspace_write.writable_roots=[…]` on 0.155.1, and on 0.157.1 exits 1 before
  its first request: `Error: --add-dir is not supported with --remote. Configure
  additional workspace roots on the server.` and `Error:
  sandbox_workspace_write.writable_roots overrides are not supported with --remote.
  Configure additional workspace roots on the server.`

### The plan tool

**[live against a local Responses stand-in, no model calls; Codex CLI 0.155.1 and
0.156.1; September 25, 2026]** Probed for a checklist sent with a task, a feature the
owner later dropped ([work-queue.md](work-queue.md#dropped-a-todo-list-sent-with-a-task));
the evidence lay in `/tmp/rewake-plan-research`, a temporary directory. The terminal was
not watched.

- **The checklist is the model's `update_plan` tool**, which takes the whole list each
  call: `{"explanation"?, "plan": [{"step", "status"}]}`, status `pending`,
  `in_progress` or `completed`. Items carry no id, description or dependency; "at most
  one in progress" is only in the tool's description **[source]**.
- **It is off by default** on both versions: without a setting the tool was not among
  those sent to the model, and a forced call got `unsupported call: update_plan`.
  **`-c tools.update_plan.enabled=true` turns it on for a launch**; a call then
  answered `Plan updated` and raised the event. rewake does not pass it.
- **Plan mode refuses it even when enabled**, on both versions, verbatim: `update_plan
  is a TODO/checklist tool and is not allowed in Plan mode`. A checklist is kept in
  Default mode.
- **A client cannot set the plan.** There is no request for it, and one that runs a
  built-in tool does not exist ([research-protocol.md](research-protocol.md#the-plan-over-the-protocol)).
  A `function_call` of `update_plan` with its output put in through `thread/inject_items`
  is accepted, and runs nothing: no handler, no `turn/plan/updated`, no checklist in
  `thread/read` or `thread/resume`.
- **Across turns, resume and compaction**: the next turn's request carries the earlier
  call and its arguments, and after a real restart of 0.155.1 and a resume the next
  request still held the steps; the notification is not sent again and `thread.turns`
  shows no checklist, because the plan event is not written to the rollout while the
  call and its output are **[source]**. A stand-in compaction whose summary left the
  steps out removed them from the next request: nothing keeps the list apart from the
  history.
- **In the terminal** each update adds an "Updated Plan" entry to the history — done
  steps checked and struck through, the current one highlighted — a series of
  snapshots rather than a standing panel **[source]**.

### Live delivery and conversation-selection checks

A compaction held against a task, a pair of real sessions driven end to end, and the
terminal's selection between two installed versions — the dated live probes of delivery
and conversation selection — are in
[research-codex-live-checks.md](research-codex-live-checks.md).

Codex 0.159.0 against 0.157.1, read in the source on September 29, 2026 — what changed for
rewake and the cached empty conversation it does not follow — is in
[the record of that check](roadmap/2026-09-29-harness-versions-checked.md).
