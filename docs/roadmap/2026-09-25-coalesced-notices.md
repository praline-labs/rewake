# One notice for a burst of letters — September 25, 2026

The owner's request of September 25, 2026: restarting three workers gave main seven
separate notices — a departure and an availability for each session, a few seconds
apart — and each woke main on its own. Mail waiting at the same moment was already
grouped (the 150 ms first collection, [inbox-groups.md](../inbox-groups.md)); letters
seconds apart were not.

## What was done

- A window in the mailbox server both harnesses share (`internal/inbox/window.go`,
  wired in `serve.go`): mail that asks for nothing waits for company — three seconds
  after the latest arrival, four at most from when the earliest was written — and goes out in one
  notice. It sits before the adapters, so the Claude Code socket line and the Codex
  gateway call see the same groups. The contract is in
  [delivery.md](../delivery.md#the-notice).
- What may wait is decided by kind, in `canWait`, from `AsksForWork`: notify and the
  four reports wait; a task, a question and an unknown kind go at once and take whatever
  waits with them. A report reserved as a waiting `send --question`'s answer does not
  wait. The scheduler in `Serve` now only ever brings a pass forward, so a task arriving
  behind held notes is not held by their timer.
- The owner asked for a cap of about five seconds; it is four, because `send` waits five
  for a status and a letter held to the cap must still be announced inside that wait for
  the send to answer delivered. A held letter's status says why it waits, so a send that
  gives up first prints pending with that reason, exit 3 — true, not a new state.
- The window is a field of `inbox.Server`, zero meaning none; the wrapper sets
  `inbox.Coalescing`. The mailbox servers the existing unit tests build by hand keep
  delivering at once, and one wrapper test proves the wrapper sets it.
- `rewake send --help` and the TALK summary say that a heads-up waits and send waits
  with it, with the cap taken from the constant.

## Evidence

Unit tests in `internal/inbox/window_test.go`: notes seconds apart in one notice, a
report and notes together, a stream announced at the cap, a task / question / unknown
kind announced at once with the waiting note, a reserved answer not held, a session
ending in the window keeping its report readable, a retry not restarting the window, a letter seen late keeping the cap from its write;
and `internal/wrap/window_test.go` for the wrapper. Twelve mutants, each killed by its
test: work allowed to wait, a reserved answer allowed to wait, no cap, quiet counted
from the first arrival, a retry taken for a new arrival, a scheduler that never brings
a pass forward, no pending detail, a wrapper without the window, a build's window
ignored, a non-positive build value accepted, a cap raised to send's own five seconds, a cap counted
from the sight alone.

The batch-arrival case now sends three heads-ups 600 ms apart after its third letter
and observes that none of them is announced alone; its fifth control, `unwindowed`,
serves the mailbox without the window and breaks exactly that observation, in both
columns. The observation asks "never alone" rather than "all three together" because
under the replay control a redelivered task takes waiting heads-ups with it, which is
correct behaviour and must not read as a failure in the crosswise check. The heads-ups'
own sends are logged under `-v`; in the first run all three answered delivered, exit 0,
on both columns.

Four scenarios leaned on a notice going out at once and were adjusted, each for a
reason of its own:

- `claude-inbound` gave the undelivered note three seconds after the fixture's last
  word; the note asks for nothing, so the sender's wrapper now holds it up to the
  window's cap, and the wait is three seconds plus `suiteCap`.
- `stopped-routing` looked for main's report to itself in the mailbox's pending files
  only. That report is written straight to main's unread mail and is not announced; it
  used to be read because another notice woke main within the second the case waits,
  and that notice now waits for company. The look covers unread mail as well, so the
  control that sends the stop to main is caught by the file rather than by a timing.
- `claude-steered` launched `busy` and `bare` without a request directory, so the
  fixture's 25-second ceiling applied to them. The two heads-ups to `busy` now take
  a few seconds each, and `bare` was gone before the last step asked for it, so
  "no such session" answered the exit 2 the finding asked for: the ordinary run
  passed for the wrong reason, and the `any-role-steers` control, whose mutant lets a
  worker steer, could no longer tell. Both sessions now get a request directory, as the Codex column's `busy`
  already did, and both columns' finding requires the role refusal's text, not just
  the exit code.
- `claude-interrupted` lost `unheard` the same way under its plugin-not-passed control:
  the wake to it went out after the wake to `heard`, which now waits for company, and
  past the ceiling. Its four workers get `staysUp` too, the helper these launches now
  share.

The first full run with the real window took 23m43s against 13m44s the run before it
(taken while another session's cases ran, so not a clean measure); summed over the 79
cases both runs share, the window added 509 s, and 65 of them grew by more than two
seconds — most by one wait on a heads-up or a report each, the steered cases by 12 to
17 s. So the suite's build now serves a shorter window, 1.5 s of quiet and a 2 s cap,
through `-ldflags -X` on `builtQuiet` and `builtCap` ([testing.md](../testing.md#running-it));
batch-arrival spaces its heads-ups 600 ms apart to stay inside it, and its unwindowed
control still breaks. The real values stay held by the unit tests, one of which keeps
the cap a second inside `send`'s five-second wait. With it the full run was green, 99
passed and 3 unsupported of 102, in 19m28s — the cases both runs share 291 s over the
run before the window — and the crosswise check passed 75 of 75.

## Review and acceptance

review-claude found no logic errors and ran it live on Claude Code: four heads-ups
1.2 s apart gave one notice, a restart burst of two workers gave one, and a task took
the waiting heads-ups along. review-codex accepted the Codex path. Three follow-ups,
main's decisions:

- The cap counted from when the server first saw a letter. Live, the first letter of a
  burst reached delivered 4.38 to 4.48 s after its send, half a second inside send's
  five, so under load it could answer exit 3 where the documents promised delivered.
  The cap now counts from the earlier of the write and the sight, with a test for a
  letter seen late. The retry test moved to a far cap, since a cap counted from the
  write had run out by the retry and hid a retry taken for a new arrival.
- The wrapper's own tests ran on the real window; they now shorten it for the test.
- The suite's documented length is about twenty minutes.

## What stays open

- Seen live on Claude Code only, in review; the Codex column's grouping is shown at
  the fixture tier.
- An agent's `rewake send --notify` now returns after about three seconds instead of at
  once. Accepted as the cost of deciding by kind; a heads-up from an agent is rare and
  not blocking work.
