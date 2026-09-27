# A grant after a cold resume

A cold resume — the session ended, then started again with `--resume` — starts a new run
of the recipient, with a new epoch and a new wrapper. Everything rewake held for the
grant lived in the old wrapper's memory ([grants.md](grants.md#taking-a-grant-back)), so
without more the new run holds nothing: on Claude Code the directory is gone and the task
continues without it; on Codex the thread may restore the root, and then rewake never
takes it back. This page is how the new run gets the grant again, and what it takes to do
it without trusting a file a worker could write.

## What the owner decided

On September 27, 2026 main decided that main's wrapper confirms a grant again for a
resumed run, by the same scheme that confirms it at delivery
([who can grant](grants.md#who-can-grant)). What a new run finds on disk only says what
to ask for.

## Which conversation a run continues

Each delivery pins the conversation it went into before the task becomes readable
(`inbox/<name>/threads/<id>`, [delivery.md](delivery.md#a-resumed-conversation)). A new
run learns its own conversation from its harness: on Claude Code from the session's
telemetry (`session_id`, which `--resume` keeps), on Codex from the thread the terminal
started or resumed. A conversation is a link between runs only through those two; the
session's name alone is not one, since a new conversation under the same name owes
nothing.

A run that continues a conversation takes over what the runs before it owe there: their
waits for tasks delivered into it move to the new run, which reports on them at its next
turn end ([delivery.md](delivery.md#a-resumed-conversation)). Without that, the task
would be closed by the resume and its grant could never end by a report.

## The copy that follows the conversation

Each journal entry carries the conversation it was granted in and the main run that
sent it (`thread`, `from`, `fromEpoch`). The copy of an ended run is kept while it names
a live grant in a conversation whose main still runs, and removed at the next save once
that main has ended too; a copy naming no conversation goes as before. The new run
reads every copy for its conversation and collects the live grants by message: those are
hints. A copy is a file a worker could write, so a hint is only a question to ask. A
stale copy — a run's that later revoked the grant — is not removed either: its hint is
asked like any other, and main refuses it.

## Confirmed again

For each hint the new run's wrapper asks the wrapper of the main the hint names, over the
same abstract address as at delivery, and takes the answer only from that run's process,
alive as it started and in the same namespaces. Main's wrapper hands the grant over only
when every condition holds:

- it holds a grant with that message, for this recipient;
- that grant was confirmed at a delivery once — it went into some conversation;
- the run it belongs to has ended;
- its task is open: on its way or unread, or read and not reported on by any run of the
  recipient; neither withdrawn nor failed;
- the process asking is the wrapper of the run it names: that pid from `SO_PEERCRED`,
  alive with that start time, in main's mount, user and PID namespaces.

The grant then belongs to the new run; the run before it can no longer have it
confirmed, and a later resume asks the same way. The directories are checked again with
the new run's view, as at delivery. Main's wrapper keeps a registered grant while its
task is open, not for a fixed time, at most 256 at once
([who can grant](grants.md#who-can-grant)).

**Main gone.** When the main that sent the grant has ended, nobody can confirm it, and
the grant is not restored: a task sent again from the current main is the way on. A main
that runs and does not answer yet is asked again: on Claude Code every two seconds while
the session runs, on Codex at the next notice.

## Claude Code

When the launch names the conversation — `--resume <id>` or `-r <id>`, not with
`--fork-session` — the wrapper asks before the harness starts and passes each confirmed
directory as `--add-dir`, which a cold resume keeps, while a directory added by the hook
is not kept (seen live on 2.1.280, [research-claude-actions.md](research-claude-actions.md)).
The grant goes into the keeper as one the session has, so the hook takes it out after
the report as it takes out one it added: the harness removes a directory given with
`--add-dir` through the same answer. What was restored and what was not is printed
among the launch notes.

A `--continue`, a picker, or a `/resume` inside the session names the conversation only
through telemetry. The wrapper then asks once the conversation is known and keeps what is
confirmed; the hook adds the directory at the first write in it, as for any grant.

`--grant-git` alone gives Claude Code nothing to restore.

## Codex

A cold resume restores the roots the thread last saved; a grant given on the thread's
first turn was not saved and is lost (seen live on 0.155.1 and 0.157.1,
[research-codex.md](research-codex.md#runtime-workspace-roots)). At the first notice into
a conversation the adapter looks for the hints of that conversation not in its own
journal and asks for each:

- **confirmed** — its roots are taken out and added back through the ordinary grant
  path, so they are journaled for this run and taken back after the report; a root the
  thread lost is added back with that notice;
- **refused** — its task closed, its main ended, the copy forged — the roots the hint
  names are taken out of the thread, so a copy nobody confirms holds no right; the launch
  directory is never taken out;
- **main not answering** — the roots stay as they are and the next notice asks again.

A root no copy names is left alone: rewake cannot tell it from one the person gave.

## What it does not cover

- A copy removed or never written — a worker can delete it — leaves a Codex root the
  thread restored without a journal entry, and rewake does not take it back; a Claude
  Code session simply does not get the directory again.
- A main that has ended cannot confirm, so nothing it granted is restored.
- The real harnesses were not run through a resume with a grant end to end; the
  workflow cases `claude-grant-resume` and `codex-grant-resume` run the fixtures, which
  keep a conversation's id and its saved roots the way the harnesses were seen to.
