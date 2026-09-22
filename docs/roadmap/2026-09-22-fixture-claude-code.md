# A fixture for the Claude Code column — done, September 22, 2026

The suite had three scenarios and one column. It has the same three and two: the
scenarios run twice, against a fixture that plays a Claude Code session the way the
older one plays a Codex app-server. Nothing new is asserted — the point was to make
what is already asserted hold on the younger adapter as well.

The two fixtures are different animals, and the difference is the whole content of
this change. The Codex one answers a protocol: a request comes in, a reply goes out,
events follow. The Claude Code one receives and nothing else — the wrapper dials a
unix socket, writes one line, closes it, and there is no channel back. So the fixture
listens where the launch told it to, checks the one line it gets field by field,
reads its mail with its own rewake, and ends its turn by running the Stop hook the
launch configured, with the payload on stdin. The report then travels the product's
own path: `rewake turn-ended` reads that payload exactly as it does in a real
session.

**What the column cannot show, and says so.** The notification carries an id for the
announcement, a status, a count and a preview of the latest member — never the member
ids. There is no conversation to select, no acknowledgement, no turn. So the
observation about reaching an accepted conversation is recorded `unsupported` with the
capability it needs, in both scenarios that declare it, and `mid-turn` is
`unsupported` whole: a one-way socket has no turn in progress to deliver into. An
unsupported observation is not a pass and is never dropped by name —
[check-runner.md](../check-runner.md) is explicit that a capability applies to a single
observation as well as to a case.

**What it shows differently.** Where the other column reads the member ids out of the
notice, this one reconstructs them from the session's own overviews: the mail
available at the nth delivery, less what was already available before it, is what the
nth delivery announced — checked against the count the notice carried, so a
membership that does not have the announced size is a contradiction rather than a
detail. And a replay is found by arithmetic rather than by a repeated id: a group
rebuilt around a replayed letter carries a *different* announcement id, because that
id is derived from its members, so comparing ids would have missed it. A delivery
announcing more messages than newly became available has announced something twice.
That was found by running the replay control here and watching it pass for the wrong
reason.

**What the fixture assumes about the real harness.** Two things, and neither is
verified against a running Claude Code; both are marked as assumed in
[research.md](../research.md). The hook payload: `rewake turn-ended` reads more of it
than the message (`internal/cli/turn_result.go` — `agent_id`, which decides whether the
end counts at all, then the event, turn and thread ids, the message under three
spellings, and the failure fields), and of what the fixture sends only
`last_assistant_message` on Stop is verified live; the event name, `session_id`, `cwd`,
`transcript_path` and the StopFailure fields follow the hook reference. For a failure
that means the error text in `last_assistant_message`, with `error_details` and `error`
beside it — the first version sent an ordinary answer there beside an error string,
which is a payload the harness does not produce. And the envelope: the fixture refuses
a line carrying a field nobody agreed to, a priority other than `next`, or a second
line on the same connection, which is stricter than the harness is known to be.

**Where it is stricter, and where it was looser.** Stricter is allowed and intended; a
fixture that accepted more than the harness would let a scenario pass on a launch or
an envelope the harness would refuse. The first version was looser in one place: it
accepted any launch flag, where the real one answers an unknown option with a
refusal ([research-launch.md](../research-launch.md)). It now accepts the flags rewake
passes — the socket, the briefing, the settings, the allowed tools, and the model and
effort a launch default can add — and refuses anything else, positional words
included. Making it strict found its own bug at once: it read the test binary's path,
which the stand-in script puts before the harness arguments, as an unknown option and
refused every launch.

**What it does not prove.** That the real Claude Code binds that socket, parses that
envelope, or runs that hook with that payload — this is a fixture, and the column's
job is to catch what the shared service code does wrong, not to certify the harness.
The two columns still mean different things: Codex is the regression gate and this
one is the search column, because the adapter behind it is younger and less
exercised. The summary names the column of every red result and promotes neither.

**The gate cannot lose an observation.** An absent capability is acceptable on this
column and nowhere else. The first version let it through on both, and review showed
what that cost: with one line removed from the gate column's table, the scenario
recorded that observation as unsupported and the run stayed green, the gate no longer
checking what it existed to check. Now three things hold it: a test in the ordinary
checks requires the gate column to declare every capability any scenario asks about,
the list of which is filled by declaring one rather than written beside the
declarations; a case on the gate that comes out unsupported is classified as a
failure; and the summarizer treats a gate case marked unsupported as red even if a
record says otherwise. On a machine without the Codex CLI the schema case on that
column is unsupported, and therefore now red — a missing prerequisite on the gate is
a gate that is not checking, which [check-runner.md](../check-runner.md) already says
must not produce a pass.

**task-report can fail here.** Its hook-to-report path had no control on this column,
so nothing showed it able to break. It has seven now, run in each column they apply
to: the four fixture controls the other column already had, and three that mutate the
product — the Stop hook left out of the launch settings, `rewake turn-ended` not taking
a Stop as the end of a turn, and a report that settles nothing. The first two exist on
this column only, because the other column's reports travel through neither a hook nor
turn-ended, and a control that cannot break anything on a column passes there for the
wrong reason. They are crossed like the others, over the observations they name, with
one rule for the cells not asked: an observation about a report is not asked in a world
that sends none, nor one about a finished answer in a world that sends an error. Which
world sends what is each control's own declaration, and every run of that control checks
it against what came back — a wrong one turns the control red instead of quietly
removing a cell.

**A mutant that fails to build is a named red case.** The unnamed red run of the cross
had the shape of a mutant built before its case started: no case record, a summary that
could only say no case explained the failure, and the build directory deleted on the
way out. Mutants are built inside a started case now, into a directory the case keeps
when it is red, with the reason written beside the build; and when the engine fails
with no case to explain it, the summary names the tests that failed.

Findings that came out of the fixture itself, each invisible while one column ran. A
control that named a letter by the order it was sent was reading a group that names its
members by id order, so which letter is read first and which is deferred is not the
scenario's to predict; the controls count reads now. On the gate column, where both
reads happen, the overview control still requires both refused; the weaker form, one
refused read and none allowed, is used only on this column, where the deferred letter's
read never happens because the overview that would have listed it consumed it. A
recipient that never started listening used to be recorded against the grouping
observation; it has an observation of its own. And the scenario list counted a scenario
once per column; it counts scenarios, and the summary counts the cases.
