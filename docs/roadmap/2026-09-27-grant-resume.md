# A grant restored after a cold resume — September 27, 2026

Both stages of the directory grant left a cold resume without its grant: the journal
lived in the wrapper of the run that received it. On Claude Code the resumed session
started without the directory; on Codex the thread could restore the root, and then
rewake never took it back, and a grant given on the conversation's first turn was lost.
Main decided the same day that main's wrapper confirms the grant again; the journal that
follows the conversation, queued separately until then, became part of it. The mechanism
is in [grants-resume.md](../grants-resume.md).

## What was done

- **Main holds a grant while its task is open** (`grantauth.Authority.Open`, answered by
  `inbox.TaskOpen`): until it is reported on by any run of the recipient, withdrawn or
  failed. The TTL and five minutes stay only for a grant whose letter is not found. The
  limit of 256 and the refusal past it are unchanged.
- **Reconfirm.** A new operation on main's address hands a grant over to a new run of the
  recipient when that grant was delivered once, its run has ended, its task is open, and
  the process asking is the wrapper of the run it names, alive and in main's namespaces.
- **Linking runs.** Each delivery was already pinned to a conversation. A new run, once
  its harness names the conversation, takes over the earlier runs' waits for tasks pinned
  to it (`inbox.AdoptWaits`) and only then sweeps their records; its report settles those
  tasks, and a report from any run of the recipient now settles a message in `--awaited`.
- **The copy follows the conversation.** Journal entries carry `thread`, `from` and
  `fromEpoch`; an ended run's copy stays while it names a live grant whose main runs;
  `grant.Hints` collects them for a conversation.
- **Claude Code.** `--resume <id>` or `-r <id>`, without `--fork-session`, is asked
  before the start, and what main confirms is passed as `--add-dir` and kept as a grant
  the session has, so the hook takes it out after the report. A `--continue`, a picker or
  a warm `/resume` is followed through telemetry every two seconds.
- **Codex.** At the first notice into a conversation the adapter asks for the hints not in
  its journal: a confirmed grant is journaled again and a root the thread lost added back;
  a hint nobody confirms has its roots taken out, never the launch directory; a main that
  does not answer yet is asked at the next notice.
- **Fixtures.** The Claude Code fixture takes `--resume` and `--add-dir`; the Codex one
  keeps a thread's roots between runs in a file (`RW_SHIM_THREAD_STORE`) the way the
  server was seen to. The workflow cases `claude-grant-resume` and `codex-grant-resume`,
  with `claude-resume-not-given`, `claude-resume-not-held`, `claude-resume-closed-held`,
  `codex-resume-not-restored`, `resume-waits-dropped` and `codex-resume-closed-held`
  ([testing-cases.md](../testing-cases.md#a-directory-granted-with-a-task)).

## Decided along the way

- **A fork of the design.** A resume that only restored the grant would still leave its
  task closed: the ended run's wait was swept by the new one, the sender read that no
  report was coming, and main would drop the grant as closed. So the new run takes over
  the waits of its conversation. The conversation, not the name, is the link: a new
  conversation under the same name owes nothing.
- **A stale copy is not removed** when a later run of the conversation revoked the grant:
  its hint is asked like any other, and main refuses it.
- **On Codex the case after the report asks that nothing be journaled**, not only that
  the roots lack the directory: the report the run finds would take a wrongly confirmed
  grant out again in the same delivery, and the roots alone would not show it.

## What stays open

- A grant whose main has ended is not restored: nobody can confirm it. A task sent again
  from the current main is the way on.
- A copy a worker deletes leaves a Codex root the thread restored without a journal entry;
  rewake does not take it back.
- A forged copy can make rewake take a root the person gave out of a Codex thread — a
  denial only, never the launch directory.
- `--grant-git` alone gives Claude Code nothing to restore.
- Neither real harness was run through a resume with a grant end to end; the fixtures
  rest on the live probes of September 26 and 27, 2026.
- Review, and acceptance on the Codex side: the Codex path is touched.
