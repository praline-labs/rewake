# Re-reading the owed task — September 23, 2026

The owner approved, on September 23, 2026, a way for a session to re-read the task it
is working on, and the rule to use it after a context compaction. Worker sessions run
with a smaller context and compact often; a compaction summary retells the brief and
can drop an item, and a session that had read its task had no command to read it
again: `rewake inbox` shows only unread mail, `--message` takes only an unread id, and
the read task lay in `done/`.

**What was built.** `rewake inbox --owed` prints again, in full and in the form of an
ordinary read plus the id, every message the current run's `awaiting/` records still
name — the tasks and questions it has read and not reported on — oldest read first,
with `--json`. It changes nothing on disk: no lock, no read status, no wait record, no
announcement. Nothing owed is one line and exit 0, which says that a task from a plain
shell — it owes no report and records no wait — is not listed and cannot be shown again
this way; outside a session it is the usual refusal. The age sweep of `done/` and of
`unread/`, where a read whose last step failed leaves the text, now keeps what the
current run still owes, so a task
worked on for more than a day can still be shown; a text lost anyway is named by id
and sender with `kept: false`. Mechanism: [delivery.md](../delivery.md#reading-again-what-is-owed-rewake-inbox---owed).

**Decisions.**

- `--owed` is exclusive with `--peek` and `--message`: those look at unread mail, and
  `--owed` at what was read. A combination is refused.
- A main session is refused, naming why: its reads record no obligation, so it owes
  nothing to show. Proposed, not built, for the owner to decide: a separate view for
  main of the tasks it waits on from others — found through the other sessions'
  `awaiting/` records naming main's run, plus what they have not read yet — which is
  what main needs after its own compaction. It answers a different question from
  `--owed` and would read other sessions' mailboxes, so it is not folded into this
  flag.
- One line in the write and general playbooks, which feed both the launch briefing and
  the guide: after a context compaction, re-read the task with `rewake inbox --owed`
  instead of working from the summary, and say so in the report. The briefing goldens
  were regenerated for it.

**Tests.** Unit tests select owed messages across the states that exist: a read task
and a read question shown in the order read; nothing once a report settled them; still
owed with a `rewake pending` mark in place and after the interim turn end it makes;
held and unread mail left out; a text no longer kept named; the refusals — a
combination, a value on the switch, main, outside a session — and a test that the state
directory is identical, file for file and byte for byte, after `--owed` in both forms.
The sweep keeps an owed task's text past its age, in `done/` and in `unread/`, and lets
a settled one go. A stopped turn end keeps the task listed, and a new run of the name
sees nothing of the old run's. The
workflow case `owed-reread` runs in both columns: a worker reads a multi-line task,
runs `--owed` in the same turn, which must print that task in full with its id in both
forms, and the turn end must then report it once and settle it. Its mutant
`owed-empty`, an `--owed` that finds nothing, must break the first observation and
only that one ([testing.md](../testing.md)).

**Open.** The view for main, above, awaits the owner.

**Later the same day.** The text form was trimmed with the rest of the feed
([entry](2026-09-23-quiet-feed.md)): it opens with `Rewake: owed a report for <n>
message(s):`, each message carries the header of an ordinary read without its id, and
nothing owed is `Rewake: nothing owed a report.` The plain-shell explanation moved to
`--help`, the id stayed in `--json`, and the workflow case now checks the id there and
the header in the text.
