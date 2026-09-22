# The task-report scenario, as built — September 21, 2026

What building the first delivery scenario taught. The contract it was built to is
section 1 of [check-runner-scenarios.md](../check-runner-scenarios.md).

The shape check covers the delivery path too. It did
not at first: four messages of the new path — the `turn/start` reply, `turn/started`,
`item/completed`, `turn/completed` — went around a check built precisely against shape
drift, and were each missing a field the real server requires (`items` and `status` on
the turn, `completedAtMs` on the completed item). The lesson is the mechanism's, not
the fields': a check that is not extended along with the fixture has stopped working
while still reporting green. The messages now come from one set of constructors that
both the running shim and the shape check read, so the two cannot drift apart again.

The shim also refuses a turn whose `input` list is absent or null, not only one that
carries text. Go reads a missing field and a JSON null as the same empty slice, so
asking for a length accepted two requests the real server refuses — the same class of
defect that the parameter checks were rewritten for. The class did not end there, and
it took two rounds to reach the bottom of it. A decoder ignores fields it was not asked
for, so `effort: 5` and `model: true` — protocol fields carrying values the server
refuses — were accepted by a shim that decoded only the four it cared about. A table of
top-level fields fixed that and stopped one level too early: `toolOutput` only had to
*be* an object, so `toolOutput.namespace: true` went straight through, and with it
every field of every nested object in the request.

What the fixture serves is now described recursively — an object's fields are a closed
set at every depth, an array says what its elements must be — and the description
covers the notice inside the delivery as well, which travels as a string of JSON and
used to be an opaque blob. The same description closes the conversation requests, whose
nested object is `config`: closed and empty, because the fixture serves no setting at
all. The schema case compares the fixture's verdict against the schema's on the same
requests, in one direction: the fixture may refuse more than the server, never less.

Two things a JSON kind cannot state came next, both found by comparing the fixture
against the live server rather than against the schema. A workspace root must be an
absolute path — the schema renders the type as a plain string and puts the requirement
in prose — so the description carries named conditions on strings beside their kind.
And a request that repeats a field name is refused before anything reads it: JSON
leaves the winner to the reader, and two readers here chose differently, a struct
keeping the earlier object's fields and a map the later one's, which let a request
missing a required field pass as complete. The description is now the only reader that
decides: the struct is filled from a request it has already accepted, and what the
description cannot state — that the input list is *empty* — is the only thing checked
after it.

The class turned up three more times, each inside the check itself, and the third
made the pattern plain enough to write down as a rule. The scan for repeated
names decoded numbers as floats, failed on one too large to represent, and answered
"nothing repeated" — so a request carrying a huge number *and* a repeated key passed,
because the scan never reached the repetition. Not being able to look is not the same
as having looked: the scan now keeps numbers as written, and reports a failure to read
as its own refusal. Then the same shape again, one level in: the scan reported its find
by returning the name, so a repeated key that *was* the empty string looked like no find
at all, and a pair of empty names hid a real repetition behind them. And the end of
input was read as the end of the document, so a value truncated inside an object counted
as fully scanned.

The rule that came out of it, and that the fixture now follows: a result and the absence
of a result are never the same value. A failure is its own return, a find whose value may
be empty gets its own flag, and the one single-valued answer left — a problem
description — is always a non-empty sentence, so "" can only mean checked and fine. An
empty notice is refused rather than skipped for the same reason.

That comparison is worth what it is and no more. Its samples check themselves, not
every nesting the protocol allows: a field nobody thought to write down is checked by
neither side. And shape is only shape — it says nothing about when a message may be
sent, whose state it reflects, or what a refusal means. Those stay the work of the
controls and of review.

A delivery check that is also a delivery is a hazard, and it caught us. Accepting a
turn starts the work in a goroutine, that work runs rewake, and a contract test that
accepted one left the goroutine running past the point where `t.Setenv` had restored
the ambient environment — the state directory of a live session. Two things close it.
The check is now a function of its own that decides and returns, so a contract test
never starts work at all; and every rewake call the shim makes is guarded by a mark
the case sets and the ambient environment never has, so a call from outside a case
cannot happen rather than being unlikely to. The guard's own test puts a rewake on
PATH that leaves a mark and requires the mark to stay absent — without that, the test
passed for the wrong reason, because with no rewake on PATH the call fails anyway.

What that guard is, stated plainly: protection against a mistake in a test, not a
security boundary. It stops the suite from touching a live session by accident. It is
not a reason to run the suite anywhere its isolation does not hold.

Two shim sessions, not one and a command: the report
is addressed to whoever sent the message, and a message sent by the harness of the
suite would have a service name as its sender, leaving nothing to observe. Both
sessions perform their own mailbox read.

The recipient is launched as `--general` and the sender as `--main`. The main role is
silent by design — it records no obligation to answer when it reads, and a finished
turn of a silent role is not turned into a message — so a recipient launched as main
produces no report at all, and the scenario would be reporting on the role rather than
on delivery.

A report is tied to a read by the obligations that read recorded — sender, epoch,
message id, and a monotonic read number compared against the counter captured when the
turn ends — not by whether the read fell inside the turn's interval. What reaches the
sender carries that link as `inReplyTo`: the list of messages the finished turn
discharges. The correlation observation reads exactly that field, from the sender's own
`rewake inbox --json`, and requires it to equal exactly the one message the recipient
consumed — the id taken from the recipient's own machine-form read, and separately
required to be among the ids the delivery named.

Two earlier versions of this check were worth less than they looked. The first compared
texts: the text of a report is whatever the recipient chose to say, the shim put the
read into it, and the control changed that same string, so the check and the breakage
were the same object. The second compared sets for overlap, which accepts a report that
settles the right message *and* something else; the acceptance round showed it by
adding a foreign id beside the real one and watching the scenario stay green.

Equality is what the observation promises, so equality is what it asks. Two mutations
of the product hold it to that: emptying `InReplyTo` in
`internal/cli/turn_reports.go` fails it on the empty list, and appending a foreign id
to the real one fails it naming both. Neither mutation touches the fixture.

The third control was first written against a second terminal event and had to be
rewritten: the wrapper ignores a terminal for a turn it has already finished, so a late
failure produced no second outcome and the control was green for the wrong reason. It
now fails the turn itself — after the text, before the report is published — and the
report's kind is what the observation reads. Its name says that window, because a
control named wider than it acts is the same false green in another costume.

What remains of the original question, stated narrowly: a failure arriving *after* the
report was published cannot produce a second outcome, because the wrapper suppresses a
terminal for a finished turn. Both neighboring questions were asked on September 21,
2026, and they came out differently.

**The session outliving the verdict** is a working observation with a working control.
Every other observation in the delivery scenario is read from a file the session wrote,
and a file outlives its writer — so the scenario ends by asking whether both sessions
are still running, and the `early-exit` control makes the recipient leave quietly, with
a successful exit, as soon as it has worked a turn. Nothing else in the case notices
that: the exit code is zero, no process is left behind, and the files are all still
there. The control fails the case, goes green only with its own breakage, and reddens
under each of the other three.

**One report for a turn that ends twice** is a case without a control, and that is a
result rather than an omission. `second-terminal` makes the recipient end the same turn
a second time and requires the sender to be told once — and its negation could not be
reached. Two barriers were removed, one at a time and together, and a single report
still came back: the `w.done` guard in the observer
(`internal/harness/codex/gateway/observe.go`) and the write that skips a report whose
id is already in the mailbox (`inbox.PutOnce`, paired with the deterministic
`inbox.ReportID`).

The third is what appears to hold it: the obligations cleared once a report is
published (`internal/cli/turn_reports.go`). After the first report the waiter has no
messages left, so a second outcome has nothing to report about. That is a reading of
the code and not a result — it is the one barrier no single-line mutation could remove,
which is also why it is the one that stayed untested. Naming it as a guess rather than
listing three equal suspects is the difference between helping the next person and
sending them to look where there is nothing to find.

The case therefore says the system answers once; it does not say which barrier is
load-bearing, and it stays green with two of the three removed. That is the boundary,
and it is why the case names it in its own comment.

Each control is verified by mutation, and by the cross: with its own breakage removed
it goes red, and with another control's breakage in place it goes red too. The first
pass of this scenario had two controls that were green either way, and the cross is
what found them.

The judgement is anchored rather than timed: a control waits for the recipient's own
record of the turn, then leaves one short window for a report to appear before asking
whether it did. Sleeping a flat five seconds instead cost the suite fifteen of its
thirty-seven.
