# Codex's `--worktree`, researched — September 26, 2026

The work queue held Codex's `--worktree` as a question for research: rewake refuses the
flag (`internal/harness/codex/codex.go`, the check before the launch plan), and two
routes were open — pass the flag through the gateway if the terminal hands the checkout
to the server in `thread/start`, or have rewake create the checkout before starting the
server. A Codex-side review session did the research on September 26, 2026: the source
at the release tags `rust-v0.155.1` and `rust-v0.157.1`, the schemas both versions
generate, and the two binaries run in a disposable container without network and
without a model call.

## What was found

- **Passing the flag through cannot work.** Both terminals refuse `--worktree` together
  with an explicit `--remote`, a local unix socket included, before a checkout is
  created and before they connect to any server: ``Error: `--worktree` is only supported
  for local sessions``, exit 1. rewake always starts the terminal with `--remote`, so the
  first route is closed upstream
  ([research-codex.md](../research-codex.md#--worktree-with-a-remote-terminal)).
- **In a local launch the terminal owns the checkout.** It allocates a managed worktree,
  rebuilds its configuration with the checkout as cwd, passes that cwd and the workspace
  roots as ordinary `thread/start` parameters, and afterwards binds the checkout to the
  thread id in the git metadata.
- **The protocol would take a checkout made elsewhere.** `ThreadStartParams` has an
  optional `cwd` and `runtimeWorkspaceRoots` and no worktree field, and the server
  applies `cwd` per conversation
  ([research-protocol.md](../research-protocol.md#where-a-new-conversation-runs)). So
  the second route is possible — but it is a feature of rewake's own: matching a managed
  worktree means repeating the allocation, its settings, the trust checks and the binding
  to the thread, and following them from version to version.
- **The old explanation was incomplete.** research-permissions.md gave the private
  gitdir rewake cannot know in advance as the reason for the refusal. The grant a main's
  task adds reads the cwd of the selected conversation (`taskGitRoots`,
  `internal/harness/codex/server_gitwrite.go`), and the direct barrier is the terminal's
  refusal.

## What was decided

rewake keeps its refusal. A plain `git worktree add`, launched from its checkout, is the
supported way and already works. rewake does not allocate a managed checkout itself
unless the owner asks for it. The entry left the work queue.

## What changed in the documents

- `docs/research-codex.md`: the terminal's refusal and how a local launch creates the
  checkout, with the source lines of both versions.
- `docs/research-protocol.md`: where a new conversation runs — the `thread/start`
  fields in both schemas.
- `docs/research-permissions.md`: the reason for rewake's refusal corrected.
- `docs/launch.md`: the upstream reason and the supported way beside the refusal.
- `docs/work-queue.md`: the entry removed.

## What stays open

Nothing to build. If a later Codex version accepts `--worktree` with `--remote`, the
refusal can be revisited; the probe is the same run of the terminal in a container.
Later the same day the owner asked for a checkout of rewake's own:
[A worktree for a Codex launch](2026-09-26-codex-worktree-launch.md).
