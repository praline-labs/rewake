# S17 evidence: the live upgrade gate and the cost of the hash

Records of what was run and seen on October 12, 2026, for S17 of stage 3: the live gate of
the clearing of 1.x ([stage3-upgrade.md](stage3-upgrade.md#the-tests)), run again on the fixed
candidate after part 3's first review, and the timing pass
of the build id ([stage3-state.md](stage3-state.md#the-cost-of-the-hash)). They are
records: not edited afterwards, superseded only by a newer record beside them.

## The live gate

Run by the orchestrating session on a fresh private base under `/tmp` (its own `HOME`,
`REWAKE_DIR` and `PATH`; `REWAKE_SESSION`, `REWAKE_EPOCH`, `REWAKE_DIR` and `REWAKE_ROOM`
unset), never the owner's state.

- **1.x**: the installed npm 1.0.3 binary, `rewake 1.0.3 · build b76b35b · built
  2026-09-29 18:41 UTC`, SHA-256
  `ad96c7bc86ff276fc5ecb2793b9d97c82e4e3ca38f0f269d26366032d90bdf30`.
- **2.0 candidate**: commit `8e4d46b89a5b27887be86af81ec99321ee232868`, from `git archive`,
  `CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags "-X …/internal/cli.Version=2.0.0-s17-gate
  -X …/internal/cli.built=<now>"`; `rewake 2.0.0-s17-gate · build unknown · built 2026-10-12
  15:38 UTC`, SHA-256 `889ae96dc0ac3a0b374e0e312a7f6266a2be4469c4f239ad2da136811d58479b`.
- **Harness**: a stub on the private `PATH` under the harness's command name, answering
  `--version` with `2.1.296` and otherwise sleeping.

The main run:

1. A 1.x session `a` started (`rewake --name a claude`), a task sent to it (`rewake send`,
   exit 3: the stub has no socket), the session ended by SIGTERM to its stub: its record
   removed, the letter failed by 1.x and archived to `inbox/a-claude/done/`.
2. A 1.x session `b` started and kept running, a task sent to it (exit 3).
3. The candidate, `rewake --main claude`, while `b` runs: **exit 1**, stderr
   `Rewake: a rewake 1.x session runs: b-claude in room default.` /
   `rewake 2.0 does not read the mail of rewake 1.x, so end those sessions and launch again.`
   The 1.x tree byte for byte as before, but for `b`'s observation file, which the running
   1.x wrapper republished itself, and its two sockets (a type-only difference); `b`'s
   pending letter kept. The launch left the 2.0 root beside it: `v2/.clear.lock` and
   `v2/rooms/default/{build,build.lock,inbox,sessions,sock}`, as the protocol orders it
   ([stage3-upgrade.md](stage3-upgrade.md#where-it-lives)).
4. `b` ended by SIGTERM to its stub; its letter failed and archived as `a`'s.
5. The candidate again: stderr `rewake: removed the state of rewake 1.x (1 room, 0 unread
   letters, 0 undelivered reports).`, then `rewake: room default, role main: selected
   explicitly with --main`; `rooms/` gone, only `v2/` in the base; the session ran on the
   stub, and when the stub was killed the wrapper exited 143 with its run directory, reply
   socket and session record gone. The unrelated `work/` and `home/` trees unchanged. The
   counts are 0 because 1.x fails and archives a session's waiting letters when it ends.

A variant: a 1.x session `c` with a task sent to it, its wrapper killed by SIGKILL with its
stub, so its record, its unread letter and its sockets stay. The candidate: stderr
`rewake: removed the state of rewake 1.x (1 room, 1 unread letter, 0 undelivered reports).`,
then a normal start; `rooms/` gone. A record whose wrapper is missing from `/proc` is
admitted as ended, and its letter is counted.

## The live gate on the fixed candidate

The first review of part 3 found the gate above silent on unrelated files with content and
on real worktree storage (G1), so it was run again on the fixed candidate, the same day, by
the orchestrating session on a fresh private base as before.

- **1.x**: the same binary, SHA-256 `ad96c7bc…bdf30`.
- **2.0 candidate**: commit `e654f0d`, from `git archive` and built as above; SHA-256
  `1c5c07e32ac4516dfa03f777d25746a301267640b3e301427f498db8794cb188` for the main run and
  `ca552f004fad6c6c6ea622ba5831bb3c9f88296e47f4e40bb76ea03957801da2` for the variant, built
  separately in its own base (the `built` time differs).

Added to the run: files with content inside the base beside `rooms/` (`notes/blob.bin`, 4096
random bytes, and `unrelated.txt`), in `work/` and in `home/`; and a real 1.x worktree — a
1.x session launched with `--worktree=gate` in a git repository under `work/`, a commit made
in the worktree while it ran (1.x removes an untouched worktree at the session's end), then
the session ended. 47 files hashed by SHA-256 before and after: the worktree's files and its
`.git` file, its record `gate.json`, the storage lock, the repository's
`.git/worktrees/gate` metadata and the unrelated files.

- While the 1.x session `b` runs: the candidate exits **1** with
  `Rewake: a rewake 1.x session runs: b-claude in room default.` /
  `rewake 2.0 does not read the mail of rewake 1.x, so end those sessions and launch again.`;
  all 47 hashes as before; the 1.x tree as before but for the running wrapper's observation
  file and its sockets.
- After `b` ended: `rewake: removed the state of rewake 1.x (1 room, 0 unread letters, 0
  undelivered reports).`, then `rewake: room default, role main: selected explicitly with
  --main`; the base holds `notes/`, `unrelated.txt` and `v2/` only; all 47 hashes as before;
  `git worktree list` exits 0 and still shows the worktree at `[gate]`; the session ended at
  the stub's kill with exit 143.
- The variant, a SIGKILLed 1.x wrapper with an unread letter: `rewake: removed the state of
  rewake 1.x (1 room, 1 unread letter, 0 undelivered reports).`, then a normal start.

## The cost of the hash

On an idle machine (load average 0.77 before, 0.88 after; no other test run), WSL2 kernel
6.18.40.1, 8 logical CPUs, Go 1.25.0, linux/amd64. Both binaries built from commit
`8e4d46b89a5b27887be86af81ec99321ee232868` as `scripts/pack.sh` builds the release:
`CGO_ENABLED=0 go build -trimpath -ldflags "-X …/internal/cli.Version=2.0.0-s17-timing -X
…/internal/cli.built=2026-10-12T15:51:29Z"`, not stripped.

- **with the hash**: 8,640,842 bytes, SHA-256
  `545ca9e85054adf932562ab2ff29b7c4e385fc4a02d34c9048affd455a429c87`;
- **with only the hash disabled**, the test variant: the same build with an `-overlay`
  replacing `internal/infra/build/id.go` so that `ID` returns a constant of the same length
  without opening the image; 8,640,178 bytes, SHA-256
  `9bd6dce7da5a8a57a1d3261b5843f6c635f4dc90866f7bb24d99d4f6f600d961`. Nothing in the
  release turns the hash off.

**The streamed SHA-256** of the first binary, read as `build.FileID` reads it, 100 times
each way. Warm: median 2.76 ms, p95 3.79 ms. With the file's pages advised away by
`posix_fadvise(POSIX_FADV_DONTNEED)` before each sample — the advice succeeded every time,
and `mincore` showed 0 of the 2,110 pages resident before 99 of the samples (the first,
taken right after the warm pass, still showed all), so for those the read was cold: median
4.02 ms, p95 4.85 ms over the 99.

**`rewake pending`**, end to end, over 100 separate processes per binary, alternating, each
in a room of its own base established with that binary's build: a live session that had
read a task from another, in a turn whose start was recorded, so every call succeeded
(exit 0, "Rewake: marked pending…"). Warm: with the hash median 20.62 ms, p95 25.02 ms;
without, median 18.66 ms, p95 22.52 ms — about 2 ms, the warm hash. With both binaries'
pages advised away before each sample: with the hash median 28.38 ms, p95 32.57 ms;
without, median 25.22 ms, p95 28.89 ms — about 3 ms.

**The flushes part 1 added (R3)** run only at a launch, which bootstraps its room and
flushes every level into its parent: a bootstrap of a room that exists, 100 times, median
26 µs, p95 47 µs, of which its seven directory flushes alone took a median of 13 µs, p95
18 µs, on this filesystem with nothing dirty. A writer such as `pending` does not
bootstrap, so the numbers above do not include them.

The raw samples, in order:

```
sha256 warm, µs:
  3009 3097 2696 3117 3200 4070 2759 2537 3219 3948 3170 2790 2446 3084 3167 2888 2568 2645
  2718 3792 3297 2746 2782 2453 2527 3441 2845 3569 3372 3488 3311 2730 2584 2555 2629 2625
  2650 2536 2455 2534 2623 2671 2854 3163 3096 2659 2743 2620 2504 3335 3404 2606 2627 2589
  2536 2514 3992 3061 2725 2660 2546 3350 3555 2721 2526 2495 3611 3624 2718 2535 2566 2860
  3288 2740 3231 2804 3745 3842 3265 2801 2736 3020 3350 3367 2911 2625 2619 2687 2662 2494
  3247 3691 3859 2830 2494 2530 2586 2904 2761 2651
sha256 advised away, µs (resident pages before: sample 0 all 2110, the rest 0):
  2902 5174 3794 3797 3785 4289 3584 3856 3980 4107 3886 3648 3486 3798 4461 3497 4278 3976
  4127 4119 4593 4259 4291 4796 4141 3753 3639 4468 4023 4361 3832 3599 3985 3984 4343 3920
  3362 4439 5029 4551 4032 4614 4859 3784 4207 4339 3889 3859 4075 3723 4263 3845 3594 4097
  3924 3749 3518 3960 3725 3666 3701 4573 4167 3782 3461 4837 3913 3560 3590 4260 4163 4074
  4414 4957 3716 4636 4532 3665 3801 4515 4220 4347 4293 4379 4144 3820 3884 4265 4848 3813
  4586 4000 3797 4094 4343 4012 4318 3983 4297 3910
pending, warm, hash, µs (all exit 0):
  26074 21385 17790 16968 18533 20102 17311 21840 20855 18351 17893 20041 24071 26957 28118
  23259 19842 21016 20464 23576 23747 20474 20236 21869 22060 22070 26827 21333 19076 23306
  20940 22970 20864 18579 19517 19832 18948 18907 22057 19661 18536 20699 19358 19677 20683
  19163 17370 19882 19890 19273 19229 20031 21168 21453 19868 21033 19747 23442 21228 21569
  19250 18397 20047 20183 23393 21697 19083 19448 19511 20061 21146 21223 23073 20278 20336
  25023 21410 22065 23076 20567 22000 21426 23181 20509 20459 20363 22037 20500 24774 24369
  26733 21833 19706 20510 23696 23256 20471 20240 21607 23661
pending, warm, nohash, µs (all exit 0):
  21450 20660 15697 15789 19586 17741 14641 15940 15627 17112 18071 16181 24671 20732 21011
  24584 18107 18331 17160 18234 17519 21696 20468 19506 19622 21904 22077 18278 16909 16694
  17227 17194 21920 18054 17493 16187 16867 16930 17148 20562 17958 18797 15921 16931 18464
  19910 18413 17531 17324 17356 17228 17935 16147 17348 17744 17240 17477 19464 22394 17891
  18296 22108 18740 19461 22323 20345 18687 17099 20536 26459 17156 20298 18809 17205 18681
  19434 18800 22158 18631 18999 19319 19143 22415 23176 19161 19644 17423 18916 22523 24168
  18213 20886 17697 21339 20998 18688 19198 20032 19478 17997
pending, both binaries advised away before each sample, hash, µs (all exit 0):
  28120 27082 24817 25764 24060 25654 28056 24582 23281 26634 23797 23735 26622 24309 23422
  24743 25895 28023 26397 29576 27422 29046 28683 26388 23248 26095 25199 30186 26834 24309
  30242 35030 28489 24743 26559 26877 26758 30818 29189 26380 32335 29524 24444 29675 26594
  26999 25047 29674 30222 30241 28303 26432 25794 32238 30556 28982 28290 27768 29765 29150
  30116 28667 26664 29816 29477 31967 29046 37072 31183 28738 25155 29320 27558 28484 29636
  28816 29360 28346 30252 29969 27750 30418 32571 31956 35535 28415 28197 31161 31801 28351
  29361 29662 27562 29849 31763 28104 22684 32585 31091 32951
pending, both binaries advised away before each sample, nohash, µs (all exit 0):
  17563 24211 22622 20875 20196 20149 23032 20599 21568 19877 21866 23653 22806 23443 20550
  20627 19228 24997 21705 31473 24742 24242 24926 23619 20361 22065 26030 22321 21967 20994
  26219 25821 22090 21261 22307 24938 25138 24503 23915 23871 27461 25308 23283 23385 25524
  25442 21868 25790 23189 20437 26028 25625 25522 23638 25950 26227 25994 25830 31225 25905
  23374 28659 25878 25525 27232 26172 27834 30355 27291 26638 27242 28685 26972 31713 23199
  24345 30078 28804 27723 25090 28379 26898 25576 26712 27979 28621 26030 26940 26868 26570
  25102 23410 23837 28885 27591 23530 26265 28073 27176 26126
bootstrap of an existing room, µs:
  63 26 28 24 32 31 23 27 26 32 28 29 23 30 24 23 26 26 24 27 47 28 23 25 27 27 23 27 23 30
  26 24 25 23 23 29 23 23 27 24 257 46 54 37 28 26 29 34 28 26 25 26 24 27 39 25 23 27 25 29
  23 24 27 23 40 33 23 25 22 24 26 25 24 27 22 23 25 50 29 23 22 27 23 26 24 22 26 22 62 33
  23 23 27 22 24 43 23 27 24 22
its seven directory flushes alone, µs:
  15 14 14 15 14 13 13 13 13 13 13 13 13 13 13 15 13 13 13 16 15 13 13 15 13 13 13 13 13 13
  13 14 15 13 13 14 13 13 13 13 20 19 20 15 15 15 14 15 14 16 13 14 13 13 13 13 13 14 13 13
  13 13 13 81 13 13 14 14 13 18 13 13 13 13 13 12 13 13 13 12 13 23 12 16 13 12 13 13 13 13
  13 13 13 12 13 13 12 13 13 14
```
