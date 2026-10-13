# agent-deck: ideas and patterns worth taking

[Back to prior art](../prior-art.md). The design these ideas serve is
[stage4-space.md](stage4-space.md) and its [build steps](stage4-space-steps.md).

agent-deck (repository github.com/asheshgoplani/agent-deck, commit 8d0f1fe1, v1.16.28) is
a Go terminal UI, CLI and web UI that manages many agent sessions, each hosted in tmux.
It was read on October 13, 2026 as code, with no run of the tool; the only runs were
probes of the `charmbracelet/x/vt` library itself, made by the first report against both
the version agent-deck pins and v0.1.0. **F:** marks a fact read in agent-deck's code or
documents (path and line in agent-deck at 8d0f1fe1), or measured by a probe; **I:** marks
an inference for rewake. Everything here ages with agent-deck's versions (a single
maintainer, several tags a week, all code under `internal/`) and with the library's.

The verdict: agent-deck is a neighbor and a source of tested techniques, not a
replacement, and rewake does not build on it. It owns no protocol-level mail, no
one-report-per-task rule and no turn boundary from the harness; it drives a composer by
keystrokes and reads the screen, with tmux owning the terminal. What it has paid for in
bugs is exactly what stage 4 is about to meet: a terminal emulator in process, a terminal
handed between programs, a checkout that outlives its owner, a message put on Claude
Code's socket, and screen tests. This document gathers what that history teaches.

Worktrees, delivery to Claude Code's socket and testing are in
[agent-deck-ideas-delivery.md](agent-deck-ideas-delivery.md), since this document would pass
400 lines with them. Sections 3 and 4 below cover all five areas.

## 1. Terminal emulation with `charmbracelet/x/vt` (S4-4, S4-8)

Context. agent-deck already runs `vt.SafeEmulator` on a PTY: it executes `tmux attach` in a
child PTY, feeds the bytes to the emulator and renders the result into its UI
(internal/ui/embedded_terminal.go, session_output.go). **F:** it never reads cells; it
takes `Render()` as one ANSI string. Rewake reads cells to build styled spans, so the cell
facts below are from the probes, not from their code. No benchmark exists on their side.

### Ideas worth taking

**Pin v0.1.0, not their pseudo-version.** **F:** agent-deck pins
`v0.0.0-20260828171018-3c30eef5e73e` (go.mod:15); in that version `"e"` + U+0301 + `"x"`
written in one call loses the mark (cell 0 stays `e`). v0.1.0 keeps it within one `Write`
(probe). Neither version joins a cluster split across two `Write` calls (below). They bumped
the pin once to "adopt upstream VT hyperlink correction" (commit 12dd4800, 2026-09-05).

**Feed on one reader, under one exclusive lock, then publish.** **F:** a single goroutine
reads 32 KiB from the PTY master and calls `Write` with the whole read under an exclusive
frame mutex; only after the call returns it sets a dirty bit (a channel of capacity one,
non-blocking send); readers of the screen take the shared side of the same mutex
(embedded_terminal.go:306-366). Why: the emulator's callbacks (cursor hidden, shown,
repositioned) run inside `Write`, so a snapshot taken between them sees a half-painted
frame (their cursor test). Rule for `host/terminal`: bump the screen sequence and wake the
long-poll only after the chunk is committed.

**Drain the emulator's replies on their own goroutine, and bound the queue.** **F:** x/vt
answers terminal queries by writing into an unbuffered `io.Pipe` that `Write` blocks on
until somebody reads it (probe plus source). agent-deck copies it back with
`io.Copy(ptmx, emulator)` on a goroutine (embedded_terminal.go:329-334) and has a
regression test for the shutdown case, where `Write` stays blocked on a reply
(embedded_terminal_shutdown_test.go). **I:** the frame lock is held across `Write`, so a
four-party wait is possible: the reader holds the lock inside `Write`, the parser waits for
the reply reader, the reply writer waits on the child's input, the child waits for us to
read its output. Their tmux child reads promptly so it has not appeared; a harness
repainting at length may not. Derived from code, not reproduced. Put a bounded queue
between the drainer and the PTY write (drop-oldest or refuse with a counter), and share one
write mutex with keys so a reply never lands inside a key.

**Know what x/vt answers.** **F:** probe, both versions, 20x4:

| Query | Answer |
|---|---|
| DA1, DA2 | `ESC[?62;1;6;22c`, `ESC[>1;10;0c` |
| `CSI 5 n`, CPR `CSI 6 n` | `ESC[?0n`, `ESC[row;colR` (1-based) |
| DECRQM `CSI ? Ps $ p` | from the mode table; 2004 reset, **2026 and 2027 report 0** |
| OSC 10/11/12 `?` | the emulator's defaults, white on black, so a theme probe sees a dark terminal |
| no answer at all | kitty keyboard `CSI ? u`, XTVERSION, OSC 4, `CSI 14t/18t`, DECRQSS, XTGETTCAP, `CSI ? 996 n` |

**I:** the common pattern of an Ink-style program is a batch of capability queries closed by
DA1 as a "replies finished" marker. DA1 is answered, so the program does not wait, and
takes the legacy path (no kitty keyboard, no synchronized output, no grapheme mode), which
matches rewake's text-plus-control-bytes key surface. Choose the OSC 10/11 answer
deliberately with `SetBackgroundColor`/`SetForegroundColor`. Unverified against a live
Claude Code or pi (open question 1).

**Glyph clusters split across chunks change the grid.** **F, probe:** both versions flush
the pending grapheme at the end of every `Write`: a ZWJ pair split in two calls gives two
wide cells; `❤` then VS16+`x` gives a one-wide heart with the selector lost. agent-deck has
no defense (its test passes because the mark ends the chunk). **I:** delay the `Write` of a
trailing partial cluster (last rune non-ASCII, ZWJ, variation selector, regional indicator)
by one more read, bounded at about 5 ms, never past one cluster. Pin the result per library
version with a split-at-every-offset test.

**Build spans by walking cells, not by parsing `Render()`.** **F, measured on this machine
under no known heavy run, 50 iterations, not a benchmark harness:** a full 200x50 repaint of
about 35 KB through `Write` took 1.5 ms; `Render()` of 200x50 took 0.9 ms; a locked `CellAt`
walk of all 10 000 cells took 0.16-0.19 ms (about 18 ns per cell, one read lock each). The
walk is cheaper than `Render()` plus parsing back, and it is the only route to structured
spans. Cell model (ultraviolet): content string, style (fg, bg, underline color, underline
kind, attribute bits bold, faint, italic, blink, rapid blink, reverse, conceal,
strikethrough), link, width. Rules:

```
for each row y:  x = 0
  c = cell(x, y); if c.Width == 0 { x++; continue }      // placeholder after a wide cell
  start a new span when (style, link) differs from the open span
  append c.Content; x += max(c.Width, 1)
trim trailing default blanks (cell == empty), keep styled blanks (background color)
reverse stays an attribute; the mod resolves it, or the host swaps fg/bg when it cannot
```

Rewake's span attributes map one to one onto bold, faint, italic, underline, reverse and
strikethrough; blink, conceal and underline style or color have no field: decide to drop
them, or conceal as fg = bg. Take the lock once for a whole walk.

**Diff rows on your own.** **F, probe:** `Touched()` is never cleared outside alt-screen
entry and `Resize`, and v0.1.0's `Draw` ignores it. Compare each row's span list (or a hash)
with the last answer. A scroll changes every row below the region, and the answer carries
them all. A resize publishes every row.

**Resize under one lock, grid first.** **F:** agent-deck calls `pty.Setsize` first (the
kernel signals the child) and then locks and resizes the emulator (embedded_terminal.go
:380-396); it clamps to uint16 and refuses below one. **I:** between the two the child's
bytes for the new size can reach a grid of the old size, and x/vt does not reflow (probe:
height shrink drops the bottom rows without scrollback; a wide cell stays at the new right
edge). Order inside one critical section: lock, `emulator.Resize`, `pty.Setsize`, unlock;
then publish every row with columns and rows in the answer.

**TERM and COLORTERM on the child.** **F:** drop inherited `TERM=`/`COLORTERM=`, set
`TERM=xterm-256color`, `COLORTERM=truecolor`; the comment says an inherited
`xterm-ghostty` invites graphics protocols a cell grid cannot hold (embedded_terminal.go
:292-304). **I:** add a UTF-8 locale when the environment has none; never `xterm-kitty`,
`xterm-ghostty` or `tmux-256color`. Test the environment from inside the child.

**Carry the input modes in the screen answer.** **F:** x/vt fires `EnableMode`/`DisableMode`
for DECCKM (1), focus (1004), mouse (1000-1006) and bracketed paste (2004); defaults are
set before callbacks exist, so none arrives for them; v0.1.0 adds `CursorHidden()` and
`CursorStyle()` accessors, which make agent-deck's cursor callbacks and mutex unnecessary.
In agent-deck the outer terminal encoded keys per mode; in rewake the mod does, so the
answer must carry application-cursor mode and bracketed paste, plus alt-screen
(`IsAltScreen`), or Right arrives as `ESC [ C` where the program wants `ESC O C`. **I.**

**OSC 52 is dropped by default; handle it.** **F:** x/vt has no default handler. Theirs
ignores a payload that is not `52;<sel>;<base64>`, empty, `?`, or longer than 1 MiB of
base64 (the parser bounds a string at 4 MiB, but decoding costs several allocations), and
always returns true; the queue holds one, newest wins (embedded_terminal.go:70-122, four
clipboard tests). **I:** a headless host has no clipboard: count and drop, or hand to the
mod; never answer `?`.

**One `write(2)` per key.** **F:** commit c9a27a4c, issue #2551: forwarding `ESC O` and `C`
in two writes made tmux read Alt+O and type a stray `C` while an arrow was held; the fix
holds an incomplete tail (`ESC`, `ESC O`, a CSI of only parameter bytes, at most 32 bytes)
until it completes or 50 ms pass (session_input_router.go:632, 733-766). **I:** the mod
posts whole key events, so on the host side write each in one call under the reply mutex
and never merge two posts. A client that relays a raw stream (`rewake attach`, host mode)
needs the holding logic itself.

### Traps their history found

- Cursor snapshot between callbacks showed a hidden cursor: one lock around `Write` and the
  snapshot (their cursor test).
- A `Write` blocked on a reply held shutdown: close the input pipe first, then the PTY, join
  reader and drainer, close the emulator, reap the child (teardown, embedded_terminal.go
  :434-453; shutdown test).
- The `CursorStyle` callback's second argument is "steady", not "blink" (comment :251).
- OSC 8 and OSC 0/2 with `;` inside are dropped (probe; `len(parts)` check in osc.go).
- Scrollback: 10 000 lines per screen, only the main one exposed; the alt screen does not
  leak into it (probe). Unused by them.
- Mode 2026 is not implemented (DECRQM says 0); their 2026 use is on the outer terminal only
  (session_output.go:143-165). **I:** to avoid sending a half-drawn frame, hold publication
  while an `EnableMode(2026)` is outstanding, release on `DisableMode` or after 150-200 ms.

### Checklist

- S4-4: v0.1.0 pinned; reader, lock, publish; drainer with bounded queue and shared write
  mutex; close order; env; OSC 52 policy; size clamp; cursor in the same snapshot; the
  cluster mitigation and its per-version test.
- S4-8: cells to spans; own row diff; modes and `alt` in the answer; resize order and full
  republish; one write per key.
- Tests to write (names are suggestions): `TestTerminalRepliesAnswered` and its negative for
  the unanswered queries, byte for byte from the table; `TestTerminalReplyBackpressure`
  (a child that floods and queries without reading its input); `TestScreenWholeChunkAtomic`;
  `TestClusterAcrossChunks` (whole against split at every offset for `e`+U+0301, `❤`+VS16,
  a ZWJ pair, a keycap, a Devanagari conjunct; fails today by design); `TestCellsToSpans`
  (each attribute alone and combined, three color kinds, wide glyph, styled and default
  trailing blanks, link change, width sum equals columns); `TestChangedRowsOnly`;
  `TestResizeOrder`; `TestAltScreenEnterLeave`; `TestOSC52Cap`; `TestKeyWriteIsOneSyscall`
  (pipe-backed fake PTY recording write boundaries); `TestChildEnv`; `TestModeTracking`.

## 2. Terminal handoff for `rewake attach` (S4-8) and the later host mode

`rewake attach` is the full screen of the first spaces; the host mode, `rewake claude --host`,
is a later stage of its own ([design.md](design.md#the-owners-answers), answer 11). Both
relay a program's bytes to the person's terminal, so the techniques below serve both.

Topology differs. **F:** agent-deck relays bytes between the real terminal and a `tmux
attach` client in a PTY it owns (internal/tmux/pty.go `AttachWithOptions`, about 476-780);
tmux, as the outer client, sets and resets the real terminal's modes itself, and Bubble Tea's
`tea.Exec` handles alt screen, mouse and bracketed paste on release and restore. Their own
code resets almost nothing: OSC 8 and SGR (pty.go:647), a scrollback clear (CSI 3 J), and a
kitty pop (keyboard_compat.go:49). **I:** rewake relays the program itself, so the mode
bookkeeping that tmux does for them falls on rewake. They give the relay mechanics; the mode
tracking has no precedent there.

### Ideas worth taking

**Raw mode with an idempotent restore that signals cannot skip.** **F:** `signal.Ignore
(SIGINT)` while attached, `StartWithSize` pre-sized from the real tty, raw mode on both the
child PTY and stdin (the child's too, or Ctrl+Z becomes SIGTSTP of the relay), `defer
term.Restore` (pty.go:545-574). **F:** nothing handles SIGTERM or SIGHUP during attach; the
process-wide handler exits without running the defers, so the terminal can stay raw
(cmd/agent-deck/main.go:908-913). Commits 178314a3 and c2ecfb12 show exits racing. **I:**
restore from one single-flight function called from the defer, from a handler for SIGTERM,
SIGHUP and SIGQUIT, and from `recover` in each goroutine; keep the termios saved at start;
offer a `--reset` or print the reset at launch to repair after SIGKILL. With raw mode,
`OPOST` is off: status lines printed meanwhile need `\r\n`.

**One long-lived stdin reader that swaps its sink.** **F:** a blocking `Read` on a tty
inherited from the shell cannot be canceled (Go does not register fd 0 with the netpoller),
so the stale reader won the next keystroke after the child ended and wrote it to a closed
PTY: "eating exactly one keypress" (commit 350973a0, repeated in 03270a19, cacc8023,
7cee9ffb; pty.go:30-36). Their fix is a `poll(2)` reader with a 100 ms timeout that retries
on EINTR, checks cancellation before and after the poll, and is joined (1 s backstop)
before the flush (QuiesceAttachInput, pty.go:350-372). **I:** host mode avoids the whole
class with one reader for the process that forwards to the current program's PTY; nothing is
abandoned at a switch. `rewake attach` ends with the process, so the OS is the next reader;
the poll-based reader is needed only if a second owner of the tty appears. Order, if
abandoning: stop, join, drain output, `TCIFLUSH`, then restore.

**Replies belong to the program that asked.** **F:** replies to the program's terminal
queries (DA1/DA2, CPR, OSC 10/11, XTVERSION) arrive on stdin after the asker is gone and the
next reader types them (commit 119c55d3; #731, #2356). Their filter drops OSC, DCS, APC, PM
and SOS always; lets CSI keyboard finals through; lets DA/DSR/CPR finals through, because
dropping them broke the asker (#738: Shift+Enter collapsed to CR), but only one per kind
while armed in a 500 ms window (termreply/filter.go; guard.go). **I:** in a transparent pipe
replies must reach the asker. Filtering applies only at a switch, for replies to queries the
program just left made. Count queries instead of timing a window: queries seen in a
program's output against replies seen on input, per kind (DA1, DA2, DSR, OSC 10/11,
XTVERSION, kitty `CSI ? u`); a reply beyond the count is orphaned and dropped. Stricter and
testable; their time window plus one-per-kind budget is the heuristic it replaces.

**Mode resets at a switch.** **F:** nothing sent by agent-deck for `?2004`, `?1049`, mouse
or `?1004`; only OSC 8, SGR 0/24/39/49, `CSI 3 J` plus OSC 1337 ClearScrollback at both
edges, `CSI < u` (kitty pop, one entry), `CSI > 4;0 m` (modifyOtherKeys off), cursor show
and shape (pty.go:647; keyboard_compat.go:49-103; session_output.go:103-121). Reason for
the kitty pop: tmux's extended keys leave CSI-u mode on the outer terminal. **I:** from the
specs, not agent-deck, for the program being left: leave the alt screen only if it entered
it, `?2004l`, `?1000l ?1002l ?1003l ?1006l ?1015l`, `?1004l`, `?25h`, `?2026l`, `?7h`,
DECCKM and keypad reset, `CSI < N u` for the pushes seen, `CSI > 4;0 m`, SGR 0, OSC 8
close, `CSI 0 q`, scroll region and charset reset. Then re-assert for the program entered
the modes it had set. An emulator per program gives the mode state and the screen in one
move, which is why section 1's mode tracking matters here.

**The prefix key automaton.** **F:** their detach key matcher takes three forms in order,
raw byte, modifyOtherKeys `ESC[27;5;<code>~` and CSI-u `ESC[<code>;5u`, matched per read
chunk of 32 bytes (pty.go:74-97); a split sequence is forwarded as data, CSI-u with extra
fields (`:` event type, lock bits, alternate keys) does not match, a pasted 0x11 detaches.
Commit a9eca1ae, #812: holding a lone `ESC` glued it to the next key (the agent saw Alt+Up).
**I:** rewake's prefix is Alt+a then `d` (or the keys of S4-8). Alt+a forms: `ESC a`,
`ESC[27;3;97~`, `ESC[97;3u` with event and lock extras; `d` plain or `ESC[100u`/`ESC[100;1:1u`.
The automaton works over a byte stream, not per chunk: state survives a split at any offset;
a lone `ESC` flushes after a short timeout; a pending prefix flushes to the program after
1-2 s; "prefix then anything else" forwards both; Alt+a twice sends one; nothing matches
inside bracketed paste (track `ESC[200~` ... `ESC[201~` on stdin; their session_input_router
does for the same reason).

**Redraw by snapshot, then a size nudge.** **F:** agent-deck has no explicit redraw: tmux
repaints a new client, the PTY is pre-sized and gets one synthetic SIGWINCH (pty.go:585-619),
and its window policy is reapplied per attach (sharedview.go). Its own UI re-serves the last
frame until the exec callback returns (#1753, c77108e3). **I:** clear, replay the entered
program's modes from its emulator, paint its snapshot with the cursor, then change the
window size twice a few ms apart (rows-1, then rows): Linux signals SIGWINCH only on a real
change, and the second change makes Ink-style programs repaint fully. Ctrl+L is a key to the
program; never send it.

**Relay SIGWINCH and re-read the size.** **F:** read the size with `GetsizeFull` into
`Setsize` on SIGWINCH, one synthetic signal at start (pty.go:585-619); through nested ssh
SIGWINCH is late or lost, so they re-read after returning (#936, 1958b276); the PTY is sized
at spawn from the real tty (#1167). **I:** also set the size at a switch and after the
snapshot.

**Do not wait for output while holding back the replies it needs.** **F:** commit bee821ff
"unblock terminal replies before waiting for output" fixed a deadlock of that shape; the
normal exit waits up to 250 ms for output to drain before resetting styles (01c14c9a).

### Traps their history found

- Style, hyperlink and scrollback leaking across the edge: OSC 8 close, SGR resets, drain
  wait, and the clear at both edges (01c14c9a; #419, #618).
- Raw input split in the middle of a sequence at a handoff must be kept, not dropped
  (0d4c1c96, #2551).
- Pty proxies in the way (atuin) break raw mode and alt screen: detect and warn (#1558).
- First frame after detach black because a flag was cleared in a callback that raced the
  first draw: clear it before resuming (#1753).

### Checklist

- `rewake attach` (S4-8): refuse a non-tty; save termios first; size, SIGWINCH relay and one
  synthetic resize; raw on stdin only, never on the worker's PTY; replay modes, paint
  snapshot, nudge; one stdin pump with the automaton; on detach join, drain (250 ms),
  `TCIFLUSH`, resets, restore; the same path on SIGTERM/HUP and on a worker's death, printed
  notice after the resets.
- Host mode (its own later stage): only the shown program's output reaches the terminal, the
  others feed their emulators and are never left unread (their PTYs would fill); switch order:
  freeze and hold keys, resets for A, orphan A's outstanding queries, clear, B's modes and
  snapshot, nudge B, switch the sink; exit repeats the attach exit.
- Tests: pump cancellation over `os.Pipe`; quiesce ordering with injected flush; the prefix
  automaton split at every offset of every form, with paste, timeout and lone `ESC`; reply
  counter tables fed byte by byte; mode-tracker goldens (push N, pop M); a real-PTY switch
  test with a probe program that sets every mode and queries DA1/OSC 11/`CSI ? u`, checked
  by an outer emulator after A, B, A; crash tests for SIGTERM/SIGHUP/panic (termios equals
  saved, reset once); a SIGWINCH storm during a switch.

## 3. What not to take, and why

- **tmux as the substrate.** tmux owns the PTY, so there are no per-cell attributes, no
  single size owner and no transparent relay without a second emulator; their code is mostly
  compensation for tmux (version checks, window-size policy, view-mode banners, orphan
  reaping). Rewake's host owns the PTY and speaks the protocol from the same process.
- **`send-keys` and paste as the delivery path.** Its own history is the argument: swallowed
  Enter, a suggestion typed back as real text (#2651, 8d0f1fe1), truncated paste (#2079),
  merging with a draft. The one place rewake would type is keys into a hosted worker (SP6).
- **Screen patterns as the primary state source.** Every harness release can break them;
  hooks and the mod's turn boundary win. Kept only as a documented fallback, in the
  delivery document, section 3.
- **The conductor, bridge and watcher stack, remote hosts, product telemetry.** A model-driven
  coordinator over a Python bridge, webhook adapters and ssh remotes are outside rewake's
  goal (one machine, mail between sessions).
- **agent-deck as host or mail layer.** As host it reverses the topology (their TUI is the
  outer shell; rewake's person stays in Claude Code); as mail it conflicts with the contract
  (one report, mail for a stopped session, tool-level delivery), and their ledger is opt-in,
  phased and still carries enumerated known gaps.
- **Coupling.** Single maintainer, several releases a week, internal packages, a socket
  transport pinned to one protocol number. Read it; do not depend on it. License (MIT) is
  not an obstacle to reading or borrowing a technique.

## 4. Open questions for live checks

Collected from all reports; each names the step that would settle it.

1. Does Claude Code or pi block, or change behavior, on the unanswered queries (kitty
   keyboard, XTVERSION, OSC 11 answered black, DECRQM 2026/2027 answered 0) when the
   emulator is their terminal? The DA1 sentinel pattern is an inference (S4-4, S4-8).
2. Does the cross-chunk cluster mitigation hold against real Claude Code output, and is the
   5 ms bound enough (S4-4)?
3. Does the keeper's checkout honour `.worktreeinclude`; if not, copy with the same rules
   (intersection with `check-ignore`, no overwrite, stop at a nested `.git`) or warn at open
   (S4-7).
4. Does the worker's harness meet a folder-trust dialog in `.claude/worktrees/agent-<id>`,
   and does Claude Code judge the checkout "changed" after the reset (stage 4's Q6) (S4-7).
5. Is the id in the checkout path and lock reason the spawn's `agentId` (Q5) (S4-7).
6. Does the receiver accept a frame with no `session_id`, and what does a stale one do
   after `/clear`, `/new` or resume (see the delivery document, section 2) (S4-5, S4-6).
7. Does the mod reading `CLAUDE_CODE_MESSAGING_SOCKET` from the environment inherit a
   grandparent's socket in a child session of a child session (S4-5).
8. Does an arrow held across a host-mode relay arrive whole, with application-cursor mode
   carried from the screen answer (S4-8)?
9. What redraw does a size nudge cause in Claude Code and pi on a switch, and does a
   snapshot first remove the flicker (S4-8)?
10. Memory of a 10 000-line alt-screen scrollback at 200 columns under heavy scrolling was
    not measured (S4-4).
11. The cause of the older library's reversed hyperlinks was not read; irrelevant if v0.1.0
    is pinned, but unverified there.
