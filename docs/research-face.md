---
description: The live checks of rewake's face, dated — each run's setup, what it drew beside the harness's own screen, and what it found and fixed.
---

# Research: rewake's face

What [face.md](face.md) states was checked live in these runs, each with its date, versions and
setup; a pi worker's own record is in [research-pi.md](research-pi.md#the-face-of-a-pi-worker).

## Live checks

Live, October 28, 2026 (Claude Code 2.1.310, the shell from its sources under a pty of 200 by 50,
a scratch state directory and project, a dev worker on the cheapest model): a prompt typed in the
face ran as the worker's turn, with its Bash call, the call's output and the reply drawn; the
switch both ways left the worker's pty at 97 by 48; a change to `render.ts` while the shell ran
was drawn by the next face with nothing restarted, main's process, the shell's and the pty
unchanged; `rewake screen` took the face. Two findings fixed then: a layer over a terminal whose
background the shell does not know let the native face show through every blank cell, so every
row is filled and painted with the default background; and a call the engine refused before any
hook (a tool not offered) stayed `running` until its turn's end now makes it a gap.

Live, October 30, 2026 (Claude Code 2.1.310, the shell from its sources under a pty of 200 by 50
that answered the colour queries, a private state directory and project, a dev worker on the
cheapest model): a letter, two Bash calls, a Read, a failing call, a reply with markdown and a
prompt typed in the face drew as [face.md](face.md#the-look) says, beside main's native pane; a change to `render.ts` while it
ran was drawn by the next face with nothing restarted. Found then and fixed: a styled text given to
a `<text>` as its content drew `[object Object]`, and a span's `attributes` in its style drew no
bold, so a row is spans with `bold` and `italic`; a colour by its index came out in `rewake screen`
as the renderer's own, not the person's, so the theme resolves it from the palette the terminal
said. Still seen: `rewake screen`'s picture draws a `✻` within a longer run two cells wide,
shifting the rest of that row by one and cutting its last character; the frame's cells are right.

Live, October 30, 2026 (brief 40; Claude Code 2.1.310, the shell from its sources under a pty of
200 by 50, a private state directory and project, a dev worker and a native one on the cheapest
model, the same task to both): the letter, the run of a Bash call, two Reads, a Grep the session did not
offer and a failing Bash folded into `Searched for 1 pattern, read 2 files, ran 2 shell commands` and
`Read 1 file` as natively; the MCP call, the edit's counts and its red and green lines, the
reply's heading, list, code block and table drew beside the native pane's; a change to `table.ts`
was drawn by the next face with nothing restarted. Still differs, for want of mirror fields: paths
are absolute or `~/…` where the native face makes them relative, an edit has no line numbers or
context rows; and a code block has no syntax colours, which need a highlighter the face does not
have.

Live, October 30, 2026 (brief 41; Claude Code 2.1.310, the shell from its sources under a pty of
200 by 50, a private state directory and project, a dev worker on the cheapest model beside main's
native pane, the same task to both: a 15-second Bash, then a long reply): every paint of the face
fell on the 50 ms grid, the glyph and the time moving in the same frames as the text, at native's
cadence; three notches of the wheel while a reply streamed held the rows shown until it ended, PgUp
and PgDn moved a page, and the wheel back to the bottom followed the stream again; the name stood on
the input's rule and left the strip; a change to `turn.ts` was drawn by the next face with the shell's
process unchanged. Still differs: the native pane reveals a streaming reply a whole row at a time,
where the face shows a row as it grows; a running Bash's own time and line count (`· 13s`,
`(13s · 14 lines)`) and the turn's tokens are not in the mirror.

Live, October 30, 2026 (brief 44; Claude Code 2.1.310, the shell from its sources under a pty of
200 by 50, a private state directory and project, main and a dev worker in auto mode on the cheapest
model, the same prompt to main's native pane and to the worker's face: a Read, a Write outside the
project, an `ls`, a reply): both drew `Read 1 file`, the Write with `⎿  Allowed by auto mode
classifier` under its result, `Listed 1 directory` and the reply; the working line went
`(4s · ↓ 18 tokens)`, `(5s · ↑ 36 tokens)` after the calls ended, and `(5s · ↓ 363 tokens)` through a
reply; the top rule read `── ⏵⏵ auto mode ─…─ w-claude · 19k/272k ─` and the strips lost their
percent. Found then and fixed: a mode heard before the hello's answer waited for the first event
after it. Still differs: Claude Code says no mode with its SessionStart, so the rule shows the mode
from the first prompt's event on, and a change of mode with shift+tab at the next event; auto
mode's classifier and its fast path give the mod the same verdict, so an ask settled without a
dialog counts as the classifier's but for an edit inside the session's directory, which may name
the classifier where it did not answer (a Bash call shows no line either way, folded); the refusal
line was not seen live, the classifier allowing every call tried, and stands on the binary's text;
the count leaps where a call's input arrives whole at its step's end, natively streamed; the
thinking word has no effort suffix and does not shimmer; the write's preview has no line numbers;
and `rewake screen`'s picture draws `⏵` two cells wide, cutting the mode's last letters there.

Live, October 30, 2026 (brief 45; Claude Code 2.1.310, the shell from its sources under a pty of
200 by 50, a private state directory and project, main and a dev worker on the cheapest model, SGR
mouse events written to the pty and the OSC 52 it wrote decoded): a drag over a wrapped paragraph
copied it as one line of 437 characters, with the notice under the input; over an edit, its head,
counts and the old and new lines without signs or padding; over a table, its rows as tab-separated
cells, each wrapped cell whole; a drag held below the body scrolled to the end and copied all it
passed. A click copied a code block's code, a reply's markdown source, a folded run's command, an
edit's path, its old and its new lines and a write's file lines; the pointer over a code block lit
its rows alone; main's native pane beside it showed no selection. A copied code block
pasted into the face's input arrived as `make allmake test`: the input is one line and drops a
paste's newline.

Live, October 31, 2026 (brief 46): a pi worker drawn by the face, its turn and letter, beside pi's own
TUI, and what still differs from Claude Code's look, in [research-pi.md](research-pi.md#the-face-of-a-pi-worker).

## A pi worker beside a Claude Code dev worker (October 31, 2026, brief 49)

Claude Code 2.1.310 and pi 1.4.0, a private state directory and project, a Claude Code main, a pi
worker and a Claude Code dev worker on the cheapest models, the same prompts to both workers; the
shell from this change's sources under a pty of 200 by 50, and the main checkout's shell at
bb68ecb inside a private herdr 0.9.5 (its own `HOME`, every `HERDR_*` cleared, so it ran a server
of its own); SGR mouse events written to the pty and its screen replayed from the bytes it wrote.

- **The scroll.** The owner had seen no scroll in a pi worker's face that day, under herdr with the
  main checkout's shell. Not reproduced: the wheel and PgUp and PgDn scrolled the pi face as the
  Claude Code one, idle, while a 120-line reply streamed (the view held where it was put), through a
  turn with calls, with the keys on main or on the worker, with herdr and without; pi's native pane
  scrolled its own scrollback. What did put a scrolled view back at the bottom: a change of a file
  under `shell/src/face/`, which reloads every face of a shell run from those sources.
- **The light.** Before, the part under the pointer was lit, a pi reply whole. After: a move lit
  nothing, a click on the reply lit its rows with the notice `sent the reply (353 chars) via OSC 52
  …`, and 1.5 s later nothing was lit.
- **The same on both**: a letter with its dot and the answer after it, a folded run, a reply's
  heading, list and table, the working line with its arrow, tokens and thinking word, the turn's
  end, a drag over a table copied as tab-separated cells, a prompt typed in the face.
- **Differences, none of them a defect of the face**: each pi step's thinking block, empty at
  minimal thinking, draws a `∴ Thinking` row where the Claude Code worker at low effort drew none
  (the face draws a thinking block whichever harness sent it), so pi's extension now sends no empty
  thinking block ([research-pi.md](research-pi.md#the-face-of-a-pi-worker)); pi's rule carries no
  mode, pi having none; pi's strip shows no limit window.

## Unfolding, failures and retries in a pi worker (October 31, 2026, brief 56)

pi 1.4.0, this change's build in a private state directory and project under `/tmp`, the shell from
its sources under a pty of 200 by 50, a pi main and pi workers on the cheapest models the workers'
agent folder takes at minimal thinking; SGR mouse events written to the pty, pictures by `rewake
screen --text`.

- **A run with a failure, a long output.** Three bash calls, the second failing, then a 30-line
  write: the face drew `Read 1 file, listed 1 directory, ran 1 shell command, 1 failed` and the
  write's ten lines with `… +20 lines`. ctrl+o drew each call of the run, the failure's
  `Error: ls: cannot access …` under its call, both outputs whole with nothing counted, and
  `Showing detailed transcript · ctrl+o to toggle` under the input; ctrl+o again folded it all.
- **A click.** On the run's line it opened the run; on its call's `… +27 lines` it opened that output,
  the row staying on screen row 16 as `… −27 lines`; a second click folded it, the row still there.
  The first click, on a conversation shorter than the pane, moved the run's line from row 25 to 10:
  the face is bottom-aligned, and with ten rows above it nothing could hold it, so the view shows
  from the first row. A click on the thinking summary sent `**Planning sequential command
  execution**` by OSC 52 and said `sent the thinking (41 chars)`.
- **Errors.** A model the account does not take: `⎿  Error: Codex error: The '…' model is not
  supported …` in red, as pi drew it. The network cut for the whole launch (a network namespace of
  its own): `Retrying (attempt 1)…` to `(attempt 3)…` in amber while pi waited, a red
  `⎿  Error: fetch failed` for each of four attempts, then `⎿  Error: Retry failed after 3 attempts:
  fetch failed` and the turn's end after 14 s; pi's own view (the prefix's `v`) drew the same five
  error lines.

## The input as pi's editor, a queued prompt, the conversation from the top (October 31, 2026, brief 57)

pi 1.4.0, this change's build in a private state directory and project under `/tmp`, the shell from
its sources under a pty of 200 by 50, a pi main and a pi worker on the cheapest model the workers'
agent folder takes at minimal thinking; keys and SGR mouse events written to the pty, pictures by
`rewake screen --text`.

- **Growth.** A prompt longer than the pane wrapped the input to 3 rows, the conversation giving
  them; sent, the input was one row again. Past the bound (14 rows on that pane) the top rule read
  `↑ 4 more` with the text scrolled to its end; Up to the first line showed lines 1–14 and
  `↓ 4 more` on the bottom rule.
- **Keys.** shift+Enter (sent as `CSI 13;2u`), ctrl+j, a blank line and `\`+Enter made one prompt of
  six lines, sent by Enter as six lines.
- **Paste.** A paste with blank lines and a tab came with its lines and the tab as four spaces; a
  40-line paste showed `[paste #1 +40 lines]` and went whole: the model counted 40 lines.
- **Queued.** A prompt sent while the worker ran a command showed `Steering: Also say the word
  second after that.` above `⣆ Running Bash…`; once pi took it, it was drawn after
  `Ran 1 shell command (ctrl+o to expand)`, where pi's own view (the prefix's `v`) drew it.
- **From the top.** A short conversation started on the pane's first row, `❯ …`, `● ok`,
  `Done in 1s`, and followed the bottom once it filled the pane.
- **Hints.** `seq 1 12` drew `Ran 1 shell command (ctrl+o to expand)`; a click on it gave
  `Ran 1 shell command (click to fold)` and the call's three rows with `… +9 lines (ctrl+o to
  expand)`; a click on that drew the twelve lines and `… −9 lines (click to fold)`, and another
  folded it back to `… +9 lines (ctrl+o to expand)`.
