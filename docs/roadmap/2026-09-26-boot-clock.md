# Durations between processes on the boot clock — September 26, 2026

`TestNotesSecondsApartAreAnnouncedOnce` failed once in an ordinary run: one notice with
two of the three letters. The cause was not load. The machine's wall clock was stepped
forward by about 1.8 s every half a minute by a time sync, and the coalescing cap
counted from the writer's `createdAt`, a wall-clock time: a step between the first and
the second letter closed the window early. Main set the fix the same day, over every
place that measures a duration between two processes.

## What was found

- **The coalescing cap** (`internal/inbox/window.go`) counted from `createdAt`. A step
  forward cut the four-second cap to about two and split bursts; a step back was caught
  by the guard against a time ahead of now. Shown by the failing run — the letters'
  `createdAt` 2.82 s apart for a 1.1 s sleep, the step logged between them — and by the
  same test with the first letter's `createdAt` set 1.8 s back, which failed every time
  with the same signature.
- **The answer mark of a waiting `send --question`** (`internal/inbox/answer.go`) was
  fresh for three seconds by its modification time, the wall clock. A step between the
  heartbeat's touch and the wrapper's look could make a live send look gone, and its
  answer was then announced to the agent as well as handed to the send.
- **A telemetry snapshot** (`internal/sessionstate/store.go`) was fresh for two seconds
  by `publishedAt`; a step made it read stale until the next publication.

Every other duration was looked at. Those measured inside one process are on the
monotonic clock already. The long ones — a letter's thirty-minute lifetime, the day
finished mail is kept, the five minutes a compaction letter waits — lose or gain a step's
seconds out of minutes. A reply socket's grace is short, five seconds, but safe for
another reason: what it covers, a socket between its bind and its listen, lasts
microseconds. The order that ids and `createdAt` give changes only on a step back.

## What was done

- **A letter carries `createdBoot`**, the boot clock's reading, stamped in `inbox.Put`
  when it has none, the one place every new letter goes through. The window places the
  write by it on the server's monotonic clock, and falls back to `createdAt` for a
  letter without one or with a reading ahead of the clock, as after a reboot.
- **The answer mark holds the reading.** The heartbeat writes it every second and never
  creates a mark that is gone — the answer's taker removes it while the send prints. A
  mark with no whole reading, from an earlier build or read mid-write, is judged by its
  time as before.
- **A snapshot carries `publishedBoot`**, and freshness is read from it, from
  `publishedAt` when it is missing.
- **Neither reading reaches an agent**: `inbox --json`, the telemetry of `list`, the
  answer to a question and the snapshot a departure notice carries leave them out.
- **A mark is judged by a clock read after it**: the heartbeat touches a mark without the
  mailbox lock, so a reading taken before the file could be older than the touch it
  found, and a live send looked gone (found in review).
- **The long terms stay on the wall clock**, written down in `internal/boottime`: a
  virtual machine paused while its host sleeps does not advance the boot clock, and ids
  must sort after a reboot. `wrap/session_notices.go`, which orders a compaction against
  main's start by the wall clock, was left as it is.

## Evidence

Each change but one has a test that fails without it, checked by mutation:
`TestAWallClockStepInsideTheWindowKeepsTheCap` (the old failure, made certain),
`TestWrittenAtFallsBackToTheWallClock`, `TestPutStampsTheBootClock`,
`TestAMarkIsJudgedByItsBootReading`, `TestTheHeartbeatDoesNotBringAMarkBack`,
`TestFreshnessGoesByTheBootClock` and `TestBootReadingsStayOutOfTheJSON`.
`TestALetterSeenLateKeepsItsCap` sets the boot reading back with `createdAt`, so it still
shows a letter written before it was seen. The one exception is reading the clock after
the mark rather than before: the window it closes is a touch landing between the two, too
narrow to hit without replacing the clock, so it rests on a probe in review that wrote
the mark after the reading and saw a live send judged gone.

## What stays open

Nothing on the Codex side: the window and the answer mark are the shared mailbox server
and `send`, which the workflow suite runs on both columns.
