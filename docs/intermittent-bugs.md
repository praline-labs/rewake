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
Loaded-thread metadata did not identify the terminal's selected thread.

The bounded fix preserved accepted reports after notification failure and released
the observer's obsolete subscriptions after established root changes. Regressions
covered both mechanisms. Ownership remained unresolved in that implementation:
two qualifying roots still caused refusal; unsubscribe did not prove selection.
See [ownership investigation](thread-ownership-investigation.md) for evidence,
contracts and remaining limits. Previously archived reports are not rewritten;
the original result was recovered by reading its archived evidence.


## Installed primary/side continuity — verified September 19, 2026

The gateway integration replaces loaded-root discovery with observed primary
intent; the historical two-root sequence above was not reconstructed. The owner
verified the installed build's concrete primary/side path separately: a new primary
task returned automatic `MAIN-WITH-SIDE-02` / `1.25` while side remained visible.
After Ctrl+C closed side, another task returned `MAIN-AFTER-SIDE-OK` without resume
or restart. No side answer settled either task. [Acceptance evidence](gateway-native-evidence.md#installed-primaryside-delivery-acceptance--september-19-2026)
records the installed-build scope.

The first slower attempt included a view switch before its correct report arrived.
Possible upstream API delay remains unresolved; it is not proof that side pauses
primary work. This successful continuity check does not establish the cause of all
older missing reports or warnings, or accept workflows that were not exercised.

## A clarification refused at admission — unexplained, September 19, 2026

The only record of this happening in real work. The package that held the analysis
was deleted on September 22, 2026; what it established is here.

**What happened.** A worker read its research task at 12:16:56 and sent a
clarification question to main at 12:17:49. It failed three seconds later:
`context deadline exceeded: selected conversation is not ready` — at local gateway
admission, before the delivery request reached the native server. The worker finished
at 12:22:30; the owner reported that the automatic notification did not appear, and
main read the finished report from the inbox by hand at 12:24:01. The report itself
was intact. Its earlier notification status had already been replaced by the manual
read, so the two failures are not established to share a cause.

**What the error does and does not mean.** Reservation gets three seconds, and the
same message stands for several different waits: no unique current connection, a
binding that is not ready, a ready binding whose resume reads have not settled, a
pending `thread/settings/update` on the selected thread, or an admission still queued
behind the connection's work queue. So the text is not evidence that native resume was
running or that the model was busy — ordinary active work is deliverable by steering.
Empty gateway close logs rule none of those out either: readiness invalidation and
reservation expiry close no connection and call no log callback, and the installed
wrapper wired `Closed` without `Record`.

**The hypothesis that was not proved.** Main had run one built-in subagent earlier. A
child's completion alone does not invalidate a selection; a numeric read of another
thread can invalidate it permanently until a recognized selection succeeds, and that
mechanism was reproduced locally. That it happened *here* was not. Attributing the
incident to the subagent would overstate the evidence.

**What to capture next time**, from the analysis, and still not implemented: on a
reservation timeout, a bounded metadata snapshot — connection and owner counts, the
current connection, binding generation and ready flag, the read phase and its age, the
pending request's method and target relation, whether admission waits on readiness,
read settlement, scheduler or gate — plus the failed completion-notice reason kept
separately from the later read status. No message text, titles or transcripts are
needed. Most of it is already available through the `Record` callback, which the
installed wrapper does not wire.

## A stopped harness leaves its wrapper stopped — cause unknown, September 24, 2026

Seen by review-claude on September 24, 2026, Claude Code 2.1.280, rewake built from the
working tree of stage 2 part A, in a private HOME with a stand-in API; evidence in
`/tmp/stage2A1-probe-OM8dqF` while it lasts.

**What happened.** A `SIGSTOP` sent to the Claude Code process, not to its job, stopped
the wrapper as designed: `followStop` (`internal/wrap/signals.go`) sees the harness in
`T` and stops the wrapper with it, to continue the harness once the wrapper is
continued. But the harness was running again within 0.15 s, while the wrapper stayed
stopped — the wrapper in `T`, the harness in `S` — and no mail was delivered to the
session until somebody sent the wrapper `SIGCONT`. Stopping the wrapper first behaved.

**What is not known.** Who continued the harness. An isolated Go reproduction of
`followStop` with a plain child did not reproduce it, so the harness itself, or
something around it, is the likelier source; that is not established.

**Why it matters.** A wrapper left stopped serves no mailbox, and nothing says so:
`rewake list` still shows the session alive. It needs a stop from outside to begin
with, which no rewake path sends.

**What to capture next time:** the order and senders of the signals the harness
receives — its own handlers for `SIGTSTP` and `SIGCONT` among them — the process states
of both at short intervals from the stop on, and whether a harness without its terminal
interface does the same.
