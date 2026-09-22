# Milestone 7. Notices instead of pasted text — done, September 16, 2026

The owner's call after living with milestone 6 for an hour: a message pasted
into a session looked like something the user typed, drowned the screen in a
block of text plus two lines of rewake hints plus Claude Code's own paragraph,
and gave the agent no sense that a tool was involved.

- A harness is told that mail is waiting — `rewake: api notify, 1 new message` —
  and the agent reads it with the new `rewake inbox`. Claude Code draws the
  notice as a single `● …` line because it is wrapped in `<task-notification>`;
  Codex gets the same line as a plain message.
- Messages have a kind: `notify`, `question` (`send --question`), `finished`.
- The end of a turn is reported to whoever wrote during it, with the last reply:
  a Stop hook passed in `--settings` for Claude Code, `-c notify` for Codex. A
  direct answer replaces that report, and reading an answer or a report asks
  for nothing back.
- The intro shrank to what rewake is and "run `rewake guide`"; the instructions
  moved into the guide.

**Live criterion, met:** two Claude Code sessions — one asked the other
a question on a shell's instruction, both read the guide on their own, the
question and the answer each arrived as one line, and the end of the asking
session's turn came back as `● rewake: web finished`. Then Claude Code and
Codex: a task sent to Codex arrived as a plain `rewake: api notify` line, Codex
read it with `rewake inbox`, answered in its final message only, and the notify
program turned that into `● rewake: cx finished` on the Claude side with `42`
in the inbox.

The run found two things no test had:

- an agent tried to answer a message from `shell` with `rewake send shell`; the
  old message text used to say that cannot work, and nothing said it any more.
  The guide says it now;
- an answer read by the asking session put the answering one on its waiting
  list, so the answering session woke up once more only to read that its answer
  had been read. Messages to a waiting session are now marked as replies.

The review round five findings are recorded in [reviews.md](../reviews.md).
