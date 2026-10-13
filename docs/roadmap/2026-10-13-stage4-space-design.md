# Stage 4 of 2.0: the first working space, designed and reviewed

The design of stage 4's first end to end, October 13, 2026, on branch `s4-space-design` from
`v2` at `32f3ab3`: [stage4-space.md](../v2/stage4-space.md) and its second half
[stage4-space-steps.md](../v2/stage4-space-steps.md). Documents only: no code changed. The
write session wrote it on the records of the probe's runs on Claude Code 2.1.296
([the first](2026-10-13-stage4-probe-1.md), [P15b](2026-10-13-stage4-probe-2.md)); it went
through review on the Codex side, which closed B1–B4 over three rounds; the fourth round's
correction of B5, at `b0e0480`, was accepted by main without another round.

## What the design decides

- **The pieces.** The mod in TypeScript, embedded in the binary and written per run for
  `--plugin-dir`; its endpoint in Go on a mod socket, every request checked by its peer; a
  harness-neutral space host in main's wrapper that owns the pseudo-terminal; the terminal by
  the standard library with an emulator library behind one interface; the space record in the
  core under the room's lease; `rewake space open|list|close`.
- **The protocol.** HTTP over a Unix socket, no wait past 20 s, one JSON schema with a small
  generator of rewake's own and a drift test.
- **The keeper.** A subagent type the mod registers and spawns without a turn of main's model;
  its one held call seats the worker; a second call is answered at once; the host moves the
  checkout's branch to main's commit after proving the checkout is the keeper's and empty.
- **The worker.** Started by main's wrapper in the keeper's checkout, detached from main's
  process tree, with a session id rewake picks; its turn boundary reports without holding.
- **The screen.** Styled text in the keeper's key surface, the cell grid as the fallback; the
  prefix Alt+a for the keys the surface does not get.
- **Closing.** Owned by the wrappers, never by the 1.5 s `session.end` hook; a record is closed
  only from what was observed; kept checkouts are reminded and never removed by rewake.
- **The rules SP1–SP9** to write before code, and the build steps S4-1 to S4-10, each with its
  acceptance.

## The review: five blockers and how each closed

- **B1. A hosted worker would be an authorized descendant of main** (first round). The
  authority of decisions and grants accepts any descendant of the addressed wrapper, and a
  worker started as main's wrapper's child is one. Closed in the second round by the topology,
  not by a new authority rule: a short-lived launcher starts the worker's wrapper in a new
  session and exits, so the worker is reparented away from main; main's wrapper refuses an open
  when it is itself a subreaper, the host requires the ancestry check to refuse before any code
  the worker controls runs, and an unreadable ancestry refuses the open (SP9).
- **B2. The worker would resolve another room tree** (first round): scrubbing every `REWAKE_`
  variable dropped main's state base. Closed in the second round: identity and transport
  variables are dropped, and the host sets the base of main's checked room with the room (SP3).
- **B3. The closing sequence relied on a budget `session.end` does not have** (first round):
  its whole chain gets 1.5 s of wall clock, its waits included. Closed in the second round: the
  hook only signals, the wrappers stop the workers, a keeper is released only after its
  worker's end is observed, and `clear` and `resume` close nothing.
- **B4. A dead main wrapper left no bounded worker shutdown** (first round). The second round
  gave the worker's own wrapper a liveness pipe whose write end only main's wrapper holds; the
  third round closed what remained — the escalation would have killed the wrapper itself, which
  shared its harness's process group — by giving the hosted harness its own group, made the
  terminal's foreground group, with the wrapper outside every signal it sends.
- **B5. Catching SIGTTOU does not let a background group hand over the terminal** (third
  round's review): Linux allows it only while SIGTTOU is blocked or ignored. Corrected in the
  fourth round: the wrapper blocks SIGTTOU around its own `tcsetpgrp` calls, and the harness
  starts with the default signal mask and dispositions. Main accepted the correction without
  another round, as a correctness detail of job control that the build step's test holds.

## The owner's decisions of the day

- The terminal library is `charmbracelet/x/vt`, an exception to the dependency rule: it is the
  only maintained emulator that tracks glyph widths, which drawing wide glyphs needs; the build
  step measures what it brings into the binary.
- How fully the screen fills the window is not settled: the probe of Claude Code's
  external-editor prompt edit (P15d) is a probe, not a choice, and the design stays on styled
  text.

## What stays open

- The live checks the build steps name: P4, whether `$.http.fetch` connects from the harness
  process, which every request of the mod depends on; P15c, whether styled text repaints fast
  enough; the keeper's second call and a hold that reports progress; the keeper's checkout,
  its base and Claude Code's judgment of it after the move; what the release achieves within
  1.5 s at exit; a keeper across `/clear`, which guards the keeping of spaces through it.
- The `events` loop across reloads and `/clear`, the keeper's model, paste into a space, the
  default room by main's name, and pi's interface as an alternative renderer.
- Restore by session id, the step after the first end to end.
