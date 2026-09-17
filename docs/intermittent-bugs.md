# Intermittent bugs

Record observed failures whose trigger is not yet understood. A successful
retry or restart is a workaround, not evidence that the defect is fixed.

## Two loaded root threads prevent delivery — open, September 17, 2026

During an owner-authorized live messaging check, a task sent from `main-codex`
to `write` failed with exit code 1:

```text
failed for write: delivery thread is unavailable: cannot identify the TUI among 2 loaded root threads
```

The recipient remained listed as a live session, but the task was not
delivered. A question and a normal task sent to `review` both succeeded,
including the final report returning to the sender.

The owner closed and relaunched `write`. A new task then returned
`RESTART-OK`. After the owner ran `/new` in that session, another task returned
`NEW-OK`. The owner subsequently restarted the main session; a further task
returned `MAIN-RESTART-OK` to it. Each result was read from the sender's inbox.
The original failure has not been reproduced by these checks.

The installed binary identified its source revision as `bca3c6b`, while the
working tree was at `5d4d370`; the intervening change affected documentation
only. The failing recipient's running binary revision was not verified:
its process was outside the inspecting shell's PID namespace. The prior
handoff described that recipient as running an older build.

At the first occurrence, the cause was unknown. The error originates in loaded-thread discovery when more
than one root thread qualifies as the terminal's target. The thread identities
and the sequence that left both loaded were not captured. The suggestion that
the main session had resumed the same conversation remains unverified.

On recurrence, capture the recipient's build and loaded-thread metadata before
restarting it, without reading conversation contents. Establish how both roots
were loaded and which one the terminal owns, then add a regression check for
that sequence. Do not resolve ambiguity by choosing an arbitrary thread.

### Recurrence and investigation, September 17, 2026

A later task's `LINK-WRITE-OK` completion was published successfully, but its
recipient archived it with `failed: app-server has no ready TUI thread: cannot
identify the TUI among 2 loaded root threads`. Report generation succeeded;
announcement failed and removed the readable report. This is a separate mailbox
defect, reproduced for finished, error and stopped reports.

Read-only loaded-thread metadata confirmed two persisted user roots, both created
before the recipient wrapper started. Neither was a fresh ephemeral startup
thread. Observer subscriptions surviving a thread switch are independently
reproduced; their participation in this particular live sequence is not proven.
The current API does not identify the terminal's selected thread.

The bounded fix preserves accepted reports after notification failure and releases
this observer's obsolete subscriptions after established root changes. Regressions
cover both mechanisms. Terminal ownership remains unresolved: two qualifying
roots still cause a refusal, and unsubscribe does not prove which root is selected.
See [ownership investigation](thread-ownership-investigation.md) for evidence,
contracts and remaining limits. Previously archived reports are not rewritten;
the original result was recovered by reading its archived evidence.
