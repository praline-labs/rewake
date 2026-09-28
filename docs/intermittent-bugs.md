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

## A stopped harness leaves its wrapper stopped — fixed, September 26, 2026

Seen by review-claude on September 24, 2026, Claude Code 2.1.280, rewake built from the
working tree of stage 2 part A, in a private HOME with a stand-in API; evidence in
`/tmp/stage2A1-probe-OM8dqF` while it lasts.

**What happened.** A `SIGSTOP` sent to the Claude Code process, not to its job, stopped
the wrapper as designed: `followStop` (`internal/wrap/signals.go`) sees the harness in
`T` and stops the wrapper with it, to continue the harness once the wrapper is
continued. But the harness was running again within 0.15 s, while the wrapper stayed
stopped — the wrapper in `T`, the harness in `S`. That no mail would reach the session
until somebody sent the wrapper `SIGCONT` is inferred from the wrapper's state, not
observed: no mail was sent while it was stopped. Stopping the wrapper first behaved.

**Cause, found September 26, 2026.** The wrapper's own `SIGCONT` continued the
harness. `stopSelf` sent `SIGSTOP` to the process; the kernel hands a process-wide
signal to a thread of its choosing — the main one, as a rule — and the thread that
called `kill` returned from it and ran on, so `followStop` continued the harness before
the group stop had reached the wrapper. The isolated reproduction of the snapshot review of
`43a1f62` (September 24, 2026), which did not reproduce it, most likely followed on the
main thread, which takes the signal on its own way out of `kill` and so
never loses the race; in the wrapper, `waitForHarness` runs wherever its goroutine lands
after the blocking `Wait4`, which is usually another thread. A probe with a plain child
showed it: the follower on the main thread left both stopped in 10 runs of 10, on
another thread left the harness running in 7 of 10 and 6 of 10, and on another thread
with the stop aimed at that thread left both stopped in 10 of 10.

**Fix.** `stopSelf` (`internal/wrap/signals.go`) sends the stop to its own thread with
`tgkill`, with the goroutine locked to the thread for the call, so the stop is taken
before the next line runs and the harness is continued only once the wrapper is.
`TestAStoppedHarnessStaysStoppedWithItsWrapper` (`internal/wrap/stop_follow_test.go`)
runs the follower on a non-main thread in a process of its own, ten rounds a run: on
the old code it failed in the first round in 20 runs of 20, with and without `-race`;
on the fix it passed 20 runs of 20 under `-race -shuffle=on`, and 20 without.

**Why it mattered.** A wrapper left stopped serves no mailbox, and nothing says so:
`rewake list` still shows the session alive. It needed a stop from outside to begin
with, which no rewake path sends.

## A requested compaction counted as nobody's, with no outcome — open, September 28, 2026

main-claude ran `rewake compact write-claude` at about 12:05:45 while write-claude was
idle. The command answered `requested (its start was not seen within 3 s)`.
write-claude's telemetry counted compaction 20 at 12:06:58 with no request on it, so main
got the plain "context compacted (compaction 20)" notice, and at 12:20:45 the bound letter
"no outcome ... within 15m0s". The worker's snapshot holds no outcome for the request:
neither the module's `compact.asked` nor its `compact.ended` reached the collector. The
session was idle, so compaction 20 was most likely the module's own call.

The wrappers ran a build of September 27; the installed binary, which the module and the
hooks run by path, had been replaced by a newer build at 11:59:38. That binary's
`rewake observe` delivers both events and the wire format did not change; hooks kept
working through the same path, the plugin module was not edited, and no settings file
changed. The last compaction attributed correctly was at 10:39. Host debug logging was
off.

The trigger is unknown. The module's `told` swallows any failure of `$.process.run`, and
nothing else carries the mark or the outcome; whether the host's `$.process.run` fails for
a module after its executable was replaced is unverified. A stale tracked turn in the
module (no mark sent) would explain the unmarked count but not the missing outcome. The
fix proposed: `told` checks the exit and retries once, and the module writes the mark and
the outcome into the control directory beside the result, for the wrapper to take when
the collector has none ([work-queue.md](work-queue.md#now-after-100)).

On recurrence: before anything else, ask the same worker for another compaction; a
refusal "the compaction rewake asked for last is still running" means the host call never
settled. Note whether the rewake binary was replaced since the worker started, and run the
worker with host debug logging if it can be arranged.
