# Explicit Git metadata grants

Owner decision, September 19, 2026: the orchestrator decides whether a particular
task needs additional Git access. Rewake does not infer permission from message
text, task/question kind or recipient role. This supersedes automatic launch and
per-task grants. Existing permissions configured independently by the owner remain
unchanged; nothing is revoked or narrowed at launch.

A verified current main can send:

```text
rewake send writer-codex --grant-git "Commit the reviewed change"
rewake send writer-codex --question --grant-git "Check and update the branch"
```

Without the flag, no roots are added. The explicit decision is carried as
`grantGit: true` in the durable individual message and its full JSON view, and in
the send result. It survives queueing, grouping, selected reads and retries. Normal
causal read receipts still identify the individual task/question; replies never
inherit a grant. Preview does not consume that task or create an obligation.

Only main/write recipients on an adapter supporting per-message grants are
eligible. The current owned-server adapter supports them; unsupported harnesses,
ordinary-role recipients, stale/shell/non-main senders and notify/report requests
refuse explicitly. Role eligibility is not a permission decision. Fresh launch,
resume and fork preserve owner arguments and do not automatically append Git roots.

For an eligible admitted explicit task/question, the existing native reservation
pins the current destination. The adapter reads current local workspace roots with
`includeTurns=false` and appends only missing validated repository metadata paths:
ordinary gitdir, worktree private/common metadata or submodule metadata. It preserves
all existing roots and the selected native sandbox policy. It neither grants
credentials nor opens arbitrary folders or a general-purpose tool permission.
Malformed/unresolved/symlinked metadata and unreadable roots leave the grant unapplied
with a delivery diagnostic; they never broaden access as a fallback.

A late granted task stays in the [unannounced queue](inbox-groups.md), alongside
ordinary work until delivery is ready. It cannot attach to an already dispatched notice.
Its roots are considered when its own ready batch is dispatched promptly, without
waiting for observation or normal completion. Already-present metadata needs no
added root field; expired, consumed or reserved members cannot grant rights to
neighboring messages.

The native field is a replacement root snapshot extended additively. On an active
turn, changes may apply only to subsequent work; this is not a promise of retroactive
permission for a running tool call. Later native owner input can replace roots again.
Rewake adds no persistent grant/revocation policy in this feature. Explicit persistent
session/repository decisions are a separate roadmap item.
