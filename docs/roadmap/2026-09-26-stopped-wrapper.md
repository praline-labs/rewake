# A wrapper that stops before it continues its harness — September 26, 2026

The intermittent bug of September 24, a harness stopped from outside running on while
its wrapper stayed stopped, had a suspected cause and no confirmation
([intermittent-bugs.md](../intermittent-bugs.md#a-stopped-harness-leaves-its-wrapper-stopped--fixed-september-26-2026)).
A read-only health review of the code on September 26 confirmed it with a probe, and
the fix followed.

## What was done

- **The stop is aimed at the calling thread.** `stopSelf` (`internal/wrap/signals.go`)
  sent `SIGSTOP` to the process, which a thread of the kernel's choosing takes while the
  caller returns from `kill` and runs on; `followStop` then continued the harness before
  the wrapper had stopped. It now uses `tgkill` on its own thread, with the goroutine
  locked to that thread for the call, so the stop is taken on the way out of the syscall
  and the next line — the harness's `SIGCONT` — runs only once the wrapper is continued.
- **A test that runs the follower where the wrapper does.**
  `TestAStoppedHarnessStaysStoppedWithItsWrapper` (`internal/wrap/stop_follow_test.go`)
  starts the test binary as a wrapper with its main goroutine pinned to the main thread,
  so `waitForHarness` runs on another one, as in a real wrapper. Ten rounds: stop the
  harness, wait for the wrapper to stop, check the harness is still stopped, continue
  the wrapper, check the harness is continued.
- **`docs/launch.md` split by subject.** It had passed 400 lines; the launch defaults
  and aliases moved to [launch-defaults.md](../launch-defaults.md), and the launch
  sequence now says how the wrapper stops itself.

## Evidence

- The old `stopSelf` under the new test: failed in the first round in 20 runs of 20,
  with `-race` and without.
- The fix: 20 runs of 20 passed under `-race -shuffle=on`, 20 of 20 without `-race`;
  a run takes about 0.35 s.
- The probe before the fix, a plain Go program with a `sleep` child: following on the
  main thread left both stopped in 10 runs of 10; on another thread the harness ran on
  in 7 of 10 and 6 of 10; on another thread with the stop aimed at it, both stayed
  stopped in 10 of 10.

## What stays open

Review and acceptance on the Codex side, since this is process behaviour.
