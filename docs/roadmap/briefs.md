---
description: The shell's view of briefs, not built yet — the briefs button on the bar, the modal over the panes with the list and one brief rendered as markdown, read by rewake's commands once a second.
---

# Briefs in the shell: the design

Briefs themselves are built ([briefs.md](../briefs.md)): the records, the commands, the worker
tools. What is left of the step is the shell's view of them, a modal the person reads and edits
nothing in. It reads the JSON of [briefs.md](../briefs.md#the-json-the-shell-reads), whose shapes
stay fixed; once built, what this page settles moves into `shell.md` and the page leaves.

## The shell

- **The button.** While a worker is up the bar is drawn, and it ends with a `briefs` button after
  the workers', two columns apart, in the unfocused grey with the terminal's foreground — it names
  no program, so no program's colour. A click or the prefix's `b` opens the modal; `b` again or Esc
  closes it. Main has no button still; the hints gain the `b` row while a worker exists.
- **The modal** is the layer `shell.md` leaves room for: a `<box position="absolute">` over the row
  of panes, the whole width on a narrow screen; the panes keep drawing under it, and the focus comes
  back to the pane after it closes. The keys go to the modal while it is open.
- **The list**: one row per brief, newest first, open ones then closed ones in the faint colour —
  `3 · Auth refactor · 2/5 · write-claude, review-claude`, an ended run faint. ↑ ↓ and Enter, or a
  click, opens one; an empty list says main makes briefs with `rewake brief new`.
- **One brief**: a `<scrollbox>` holding the text, rendered by OpenTUI's `<markdown>` (0.6.6, on
  `marked`, already in the shell's tree), then a rule and the comments, each
  `<name> · <time>` over its text. The wheel, ↑ ↓, PgUp PgDn, Home End scroll; Esc or Backspace goes
  back to the list. Given no `treeSitterClient`, `<markdown>` starts OpenTUI's default one; a
  probe of a compiled shell (review, October 26, 2026) drew headings, a checkbox and fenced code.
  Rejected: plain text — a table or nested list main writes would read badly, and rendering adds
  no dependency.
- **How it reads and learns of a change**: by rewake's commands, as main's run, like the strip —
  `rewake brief list --closed --json` once a second while the list shows,
  `rewake brief show <id> --json` once a second while a brief shows, nothing while the modal is closed. A new answer that
  differs from the last is drawn, the scroll position kept. Rejected: watching `briefs/` from the
  shell — it would build state paths itself and read records past the CLI's checks.

## Known limits

- The shell shows a change within a second and the command's time.
