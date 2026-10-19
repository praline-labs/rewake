# Remote control: the tests

What proves `rewake compact` and `rewake interrupt`, part by part: the design is in
[remote-control.md](remote-control.md), and the workflow case in
[testing-cases.md](testing-cases.md#a-stop-and-steering-a-session). The tests of the
Claude Code module and its collector — the module under node against a strict host, the
interrupter mark, the compaction marks — left with the plugin in S9, October 11, 2026;
they are in the history at `98acc3b`, and how the retired `claude-steered` case was
built is in [testing-plugin.md](archive/1.x/testing-plugin.md#steering-a-session).

- `internal/core/control/control_test.go` — the protocol: a done answer and the cleanup, the
  modes, `not answering` with the request removed, a taken request without an outcome
  answered as an open failure and its late result cleared, a partial or foreign answer ignored, a second asker
  refused without writing, no directory, a request taken as the asker gives up, in
  each of the three orders above, a taken request left in place while it is served, and
  an asker cut short withdrawing its request as `cut short`.
- `internal/host/control_test.go` — the wrapper makes the directory, private, for a
  harness that serves it, removes it with the session, and makes none for one that
  does not.
- `internal/cli/steer_test.go` — the commands against a fake served side: the outputs,
  a compaction answered started and one answered requested, each ending the command
  and leaving main's record of it, made before the request is written, a request taken
  and unanswered keeping it and promising the letter, a command that could not leave the
  record promising no letter, the refusals and a final `failed`, which leave none, while
  an open `failed` keeps it, the record held while the command asks and free once it
  has ended, with their next action, `not answering`, and every wrong call refused
  with exit 2 and nothing written; a harness that is not steerable, and one without a
  focus; and a SIGTERM during the pickup, which ends the call as `cut short` with the
  request withdrawn instead of ending the process.
- `internal/host/session_notices_test.go` — no notice of a compaction this main asked
  for, and one each for a compaction another main asked for and one nobody asked for;
  `compaction_letters_test.go` — one letter, a note from the worker owing nothing, with
  the tokens and the count; a refusal and a failure as letters at once, another main's
  outcome not sent; a half waiting for the other only a moment, and going at once when
  the worker leaves; every letter owed by main's record of the request, which goes with
  it: a worker gone with no word of the compaction as a failed letter only after a
  moment in which its last snapshot is read again, and one that published its outcome
  late getting that outcome's letter, no word within the bound as the bound's letter, and a record an
  earlier run of main left closed by the next; `compaction_letters_hold_test.go` — a record
  its command holds left alone while its worker leaves, and a command killed with
  SIGKILL letting go of its record, which is then closed. `internal/infra/state/held_test.go`
  — the hold taken only once the writer lets go, and refused on a file removed or
  replaced between the open and the lock; `internal/core/control/pending_test.go` — the
  record read again under the hold, and a closed one not held.
- The workflow case `fixture-steered` — both commands end to end from a main against
  fixture workers whose program serves the control directory as the module did: an idle
  compaction with a focus answered started while it still runs, before the telemetry
  counts it, then its letter to main with the tokens and the count and no notice of it
  to main; a second compaction whose worker is killed before its end still ending in a
  letter to main; a compaction refused mid-turn with the turn going on; an interrupt
  giving main the stopped text naming it, also in its awaited list; an idle interrupt
  refused; a worker serving no control not answering; and a worker's call refused as a
  wrong call ([testing-cases.md](testing-cases.md#a-stop-and-steering-a-session)).
- `internal/core/control/serve_test.go` — the served side a run serves (`control.Serve`, the
  fixture's since the Codex adapter left): an answer
  written once with the request's id, a request carried out once across a second look
  and a fresh server, one withdrawn or replaced as it is marked left undone and handed
  to the served side once its answer is written, and a foreign id not taken.
- `internal/host/compaction_letters_running_test.go` — a compaction whose worker stopped
  waiting while it still ran, reported as `started`: no letter for that, and main's letter
  comes from its end, with the tokens.
