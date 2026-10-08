# Testing the tool set

The workflow cases that make rewake's calls through the fixture program's own tool
transport, and the timed turn start that program sends, with their controls. The tiers,
the commands and how to read a result are in [testing.md](testing.md); the transport
itself is in [mail-bridge-transport.md](mail-bridge-transport.md), and the rules these
cases hold, with the unit tests beside them, are in [rules/tools.md](rules/tools.md).

## One case per tool

`tool-inbox`, `tool-send`, `tool-pending`, `tool-whoami`, `tool-retry` and `tool-list` run
in the fixture column only, one per tool of the set, each with a main and a worker: the
transport is the fixture's own program, which registers the tools the probe offers and,
asked by `RW_SHIM_TOOL_CALLS`, makes its first turn's calls as a harness would — the
native call reported to the adapter, the request to the wrapper's endpoint, the result it
would hand the model reported back — and logs each answer. The main sends a task; the
worker's turn makes the call and ends.

`tool-inbox` reads the task through the tool: the answer must show it, and the turn's end
must report and settle it, so the read counted. `tool-send` must reach the main as one
heads-up; `tool-pending` must be accepted and reach the main as an interim message with
its line while the task stays owed; `tool-whoami` must name the worker and say the call
came through the tool; `tool-retry` retries a heads-up by the receipt its answer named and
is judged by the retry's answer, parsed: the heads-up's own id, recipient and receipt,
with the heads-up still one — a refusal that only mentions the receipt does not pass;
`tool-list` must name both sessions.

## The read's schedule

The schedule of `tool-inbox` is a choice the scenario makes and checks: its inbox call
asks the program (`awaitRead`) to go on only once the letter left the unread overview,
within ten seconds, and the scenario requires the program's record that it did. The
fixture's ends otherwise follow a result within milliseconds, and an end that overtakes
the acknowledgment leaves the letter unread (T7, T8) — that order is the rule's, held in
`test/toolrig/order_end_test.go`, not a harness's behaviour this wait stands for
(October 8, 2026).

## The timed turn start

`fixture-turn-start` runs two tasks through one fixture worker and requires the program's
own time of each turn's start in the mailbox, later for the later turn: the program times
its start before the turn does anything and sends it with the start, as a harness that
knows when its turn began does, and the endpoint records it where a pending mark of an
earlier turn whose end was lost finds its turn over.

## Controls

The controls are product mutants, listed by scenario in
[testing-cases.md](testing-cases.md#every-control): five refuse one tool's calls at the
endpoint, the read has one that keeps the letters' text from the answer while the read
commits and one that never acknowledges it, and the timed start one that drops the time.
The crosswise check (`TestToolControlsCrosswise`) runs every scenario of the family under
every other's control, but `tool-retry` under `tool-send-refused`, which leaves it nothing
to retry: a cell that must come out unjudgeable is not a control (October 8, 2026).
