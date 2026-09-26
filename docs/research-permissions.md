# Native sandbox and permission research

[Back to delivery research](research.md). Historical source and live observations
retain their original versions and dates below.

### Sandbox (Linux)

Checked with `codex sandbox -P :workspace -C <dir> -- <cmd>`, without calling the model:

| action from inside the sandbox | result |
|---|---|
| write to `/tmp/...` | allowed |
| `connect()` to a unix socket | `EPERM` |
| write to `~/.codex` | read-only file system |
| variable | `CODEX_SANDBOX_NETWORK_DISABLED=1` |

**[verified live]** The network seccomp filter cuts off any socket domain,
including AF_UNIX, when the network is disabled. In legacy workspace-write,
`sandbox_workspace_write.network_access = true` allows the network while
`~/.codex` stays read-only. An explicit `-P :workspace` ignores that legacy
setting and uses its profile's network policy. With the default restricted
network the agent cannot connect to the delivery socket or call the queue,
but it can write files to `/tmp`.

### Git metadata writes by role

**[source: snapshot `44b9011`, September 13, 2026; live checks: CLI 0.154.0,
September 16, 2026]** The source workspace has version `0.0.0` in its Cargo
manifest; that placeholder does not identify the installed release's commit.

- `.git`, `.agents` and `.codex` are protected by default:
  `codex-rs/protocol/src/permissions.rs:799–854`. The Linux runtime binds writable
  roots, then reapplies protected subpaths with `--ro-bind`:
  `codex-rs/linux-sandbox/src/bwrap.rs:594–627,1039–1088`.
- The legacy override
  `-c 'sandbox_workspace_write.writable_roots=["<cwd>/.git"]'` permits commits
  but **replaces the array**. A sandbox check with an existing configured root
  confirmed that root was lost. The tmp and network fields are unaffected.
  Rewake does not use this override for Git access.
- **`--add-dir <gitdir>` adds to the configured roots.** A live `codex exec`
  run committed successfully with this flag and `-s workspace-write`; its
  sandbox header included both the added `.git` and all previously configured
  roots. This is the flag rewake passes for fresh main and write launches.
- The flag is shared with TUI and repeatable: `add_dir` is a `Vec<PathBuf>` in
  `codex-rs/utils/cli/src/shared_options.rs:74–76`. TUI forwards it as
  `additional_writable_roots` in `codex-rs/tui/src/startup_orchestration.rs:128–143`.
  `codex-rs/core/src/config/mod.rs:3454–3468` combines cwd, additional and
  configured roots, then deduplicates them. Both interactive and exec argument
  parsers accepted repeated identical flags with `--help`, without a model call.
- Explicit permission profiles ignore legacy roots, network and tmp settings:
  `codex-rs/core/src/config/mod.rs:3438–3468,3510–3547`. Rewake leaves the selected
  profile intact and adds runtime roots. A restrictive selected policy can still
  refuse writes; the adapter does not change it to workspace-write.
- `codex sandbox` does **not** forward root-level `--add-dir`: its dispatch and
  config overrides omit `additional_writable_roots`
  (`codex-rs/cli/src/main.rs:1698–1740`, `debug_sandbox.rs:629–633`). An EROFS from
  that command with `--add-dir` does not describe TUI or exec behavior.

The baseline and legacy comparison need no model call:

```bash
mkdir -p /tmp/git-permission-check/{repo,home,tmp}
git -C /tmp/git-permission-check/repo init -q
export CODEX_HOME=/tmp/git-permission-check/home
export TMPDIR=/tmp/git-permission-check/tmp
codex sandbox -P :workspace -C /tmp/git-permission-check/repo -- \
  git -c user.name=Test -c user.email=test@example.invalid \
  commit --allow-empty -m 'Check default metadata protection'
# exit 128: .git/index.lock: Read-only file system
cd /tmp/git-permission-check/repo
codex sandbox -c 'sandbox_mode="workspace-write"' \
  -c 'sandbox_workspace_write.writable_roots=["/tmp/git-permission-check/repo/.git"]' -- \
  git -c user.name=Test -c user.email=test@example.invalid \
  commit --allow-empty -m 'Check scoped metadata access'
# exit 0; this demonstrates the legacy override, not the rewake launch path
```

The positive legacy check uses shell cwd: sandbox's `-C` requires `-P`, which
selects a profile and ignores the legacy grant. Its legacy default is read-only,
so that check chooses workspace-write explicitly. A separate live exec turn
verified the actual additive launch path from the same temporary repository:

```bash
codex exec --add-dir /tmp/git-permission-check/repo/.git -s workspace-write \
  'Run git -c user.name=Test -c user.email=test@example.invalid commit --allow-empty -m "Check additive metadata access" and report its exit code.'
```

Unlike the sandbox probes, this invokes a model; the September 16 verification
was performed by the orchestrating session. Rewake's automated checks use
launch plans and temporary Git fixtures instead. The metadata resolver reads
`.git` and `commondir` without invoking Git; tests compare it with
`git rev-parse --path-format=absolute --git-dir --git-common-dir` for ordinary
repositories, worktrees and submodules. Both worktree metadata directories are
needed: Git writes per-worktree state and shared repository state.

### Managed worktrees and continuation permissions

**[source: snapshot `44b9011`; sandbox verification with CLI 0.154.0,
September 17, 2026; no model call]** These worktree checks concern local
launches. Rewake now uses a remote TUI, whose continuation permission boundary
is documented below; the local behavior does not establish remote behavior.

Managed worktree allocation uses `$CODEX_HOME/worktrees/<four-character id>/<repo>`
by default, or `desktop.git-worktree-root` when configured
(`worktree/src/settings.rs:43–56`, `worktree/src/paths.rs:13–30`, relative to
`codex-rs`). `worktree/src/lib.rs:85–99` runs a detached `git worktree add`;
`:147–159` preserves the source cwd's relative subdirectory within the checkout.
The TUI retains additional writable roots, sets overrides.cwd to the new
checkout cwd, and rebuilds config (`tui/src/worktree_startup.rs:217–221,277–286`).
That cwd becomes a workspace root (`core/src/config/mod.rs:3454–3468`).

The checkout is writable as cwd, but the parent metadata grant alone is not
reliable. A sibling checkout succeeded with only the source `.git`; a checkout
in the deeper managed layout (`<home>/worktrees/abcd/<repo>`) failed to create
`<source>/.git/worktrees/<name>/index.lock`. Adding that exact private gitdir
alongside the common `.git` made the commit succeed. Granting only the private
gitdir failed to write objects. Both tmp write exclusions were enabled, so the
probe did not borrow broad access to `/tmp`.

The source explains the observed layout sensitivity: worktree pointer targets
are protected subpaths (`protocol/src/permissions.rs:2212–2231`), while Linux
sorts writable roots by depth and reapplies read-only subpaths after each bind
(`linux-sandbox/src/bwrap.rs:559–560,583–627`). A parent root alone does not
reliably override the later private-metadata carveout.

The sandbox subcommand ignores root-level `--add-dir`, as noted above. The
model-free probes therefore compared effective writable-root lists from an
already-created worktree cwd. Parent-only failed in the managed layout; this
form, with both roots, succeeded:

```bash
codex sandbox -c 'sandbox_mode="workspace-write"' \
  -c 'sandbox_workspace_write.exclude_slash_tmp=true' \
  -c 'sandbox_workspace_write.exclude_tmpdir_env_var=true' \
  -c 'sandbox_workspace_write.writable_roots=["<source-repo>/.git","<source-repo>/.git/worktrees/<name>"]' -- \
  sh -c 'printf "fixture\n" > probe.txt && git add probe.txt && git -c user.name=Test -c user.email=test@example.invalid commit -m "Check worktree access"'
```

Rewake refuses managed `--worktree` launches under its owned server. The direct reason,
found on September 26, 2026, is upstream: the terminal itself refuses `--worktree`
together with `--remote`, before it creates a checkout, and rewake always starts it with
`--remote` ([research-codex.md](research-codex.md#--worktree-with-a-remote-terminal)).
The unknown private gitdir, once given as the reason, is not the whole obstacle any
more: the grant a main's task adds (`taskGitRoots`,
`internal/harness/codex/server_gitwrite.go`) reads the cwd of the selected conversation,
not the launch directory. The supported alternative is to create the worktree first,
then launch from its checkout; metadata discovery can then add both directories.
Automatic support would need the terminal to accept the flag with `--remote`, or rewake
to allocate the checkout itself, which is not built unless the owner asks
([the decision](roadmap/2026-09-26-codex-worktree.md)). Rewake's session record retains the
wrapper's original cwd, not the dynamically chosen checkout path, and it does
not read saved transcripts to predict where a continuation will run.

### Remote continuation permissions

**[snapshot `44b901161`; CLI 0.154.0; September 17, 2026]** Remote resume/fork
reject permission overrides, including `--add-dir`. Rewake omits generated grants
and warns; caller flags stay intact. Delivered work can add missing Git metadata.
[Source evidence and API limits](continuation-permissions.md) cover that grant,
steering, manual TUI replacement and the lack of a grant at idle attachment.

The automatic launch grants described in the September 16 observations above are
historical. Since September 19, [explicit main-authorized grants](git-grants.md)
replace that policy; roles and ordinary launch arguments add no permission by default.
