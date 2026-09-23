# Honest delivery status for Claude Code — September 23, 2026

The owner decided, on September 23, 2026, that a message Claude Code's inbound gate holds
must not count as delivered anywhere, and that the first notice to a new session waits
until the session can take it. The trap it closes is in
[traps.md](../traps.md#a-message-reported-delivered-was-held-by-claude-code--the-status-is-now-honest);
the gate itself in [research-launch.md](../research-launch.md#claude-codes-cross-session-inbound-gate).

**Who listens.** The receiver sends its receipts only to a reply socket in its own
socket's directory, and only after checking that the listener is the process that wrote
the line. So the wrapper both writes and listens: its inbox server writes every notice
of its session, while `rewake send` only drops a file and has exited long before a hold
ends. The listener is `sock/<name>.<epoch>.reply.sock`, made before the harness starts
(`internal/harness/claude/lane.go`); the Codex path is untouched.

**What the sender sees.** A new state, `held`, with the harness's reason. `send` waits
through it and, if it is still held when the wait ends, prints `held for <name>: …` and
exits 3. A release makes it `delivered`; an expiry, a refusal, a denial or a drop makes it
`failed`, and a held task or question that fails sends its sender a note, with an
`undelivered` field, that it never reached the agent, is owed no report and is not sent
again. A question stops waiting for its answer once its own status is `failed`. That
change is in `send --question` for every recipient, so it touches both columns: a Codex
question that fails after its first wait — its session ended, say — now exits 1 at once
instead of waiting out its deadline, which is right, since a failed question cannot be
answered. Also in both columns: when a failed message has no waiting copy left, its
readable copy now moves to `done/` instead of being deleted (`settle` in
`internal/inbox/outcome.go`); a Codex message can reach that path too, and keeping the
copy harms nothing. A held message is readable, and an agent that reads it anyway has read it;
nothing hides it. A write with no receipt within 300 ms counts as delivered — for now:
a late word within a minute takes it back
([delivery-adapters.md](../delivery-adapters.md#claude-code-adapter)).

**The first notice.** It waits for the session's first status line, or three seconds.
The owner asked for SessionStart; a live probe showed SessionStart runs about 80 ms after
the socket appears, while the gate still holds, and a line written then was held and
released 300 ms later; a line written at the first status line, 240–280 ms after the
socket, was accepted every time ([research.md](../research.md#the-inbound-gate-on-rewakes-line)).

**Heavy review, same day.** Not yet accepted on the first pass, for eight findings, all
fixed before a second review:

1. A wrapper killed while its session held a task — SIGKILL, OOM — never failed it. The
   next session with the name read `held` from disk, took it for settled, and left it
   held with its copy waiting; the sender waited for a report forever, and `send` on the
   dead session answered `held` with exit 3, since its held branch skipped the liveness
   check. Now a `held` on disk that this run does not hold is unsettled, the next tenant
   fails it and tells the sender, and `send` checks liveness for `held` as for `pending`.
2. The 300 ms window could count a hold as delivered under load, against the owner's rule.
   Now lines counted from silence are kept for a minute or the last 128, a late `held`
   takes the delivery back unless the message was read, a late failure with no hold
   before it fails the message, and either failure sends the undelivered note, because
   the sender may have exited 0 already. The session's end fails what was taken back as
   well, though no waiting copy is left for it. The fixture gained a mode that reports
   its hold at 900 ms, so it is no longer quicker than the binary has to be.
3. The guide said exit 3 "will land on its own", which a hold waiting for a person does
   not; it now names both.
4. The `--question` change above, recorded for both columns.
5. A failed accept other than a closed listener retried at once, on a whole core; it now
   backs off up to a second. A killed wrapper's reply socket stayed in `sock/` forever;
   the next lane there removes each one a dial is refused on, once it is five seconds
   old — a socket is bound before it listens, and a neighbour starting at the same
   moment must not lose its own.
6. The refusing fixture mode was never run in the suite; now it is.
7. The Open line on the reply socket, replaced by the live observations below.
8. A link titled launch.md pointing at claude-telemetry.md; `delivery.md` at 399 lines,
   split: the adapters are now [delivery-adapters.md](../delivery-adapters.md).

**Tests.** Unit tests for the lane — correlation, the window, close receipts in either
order, a late word, the opening — for receipt parsing, for the held states in the inbox
server, and for `send` and `--question` on a held or failed message. The workflow case
`claude-inbound` on the Claude Code column, with a fixture that holds, releases, expires
and refuses as the binary does, and reports a hold late, and six product mutants
([testing.md](../testing.md#claude-code-telemetry-budgets)). Three paths are covered by
unit tests alone, on purpose: the session's end failing what it holds, a `--question`
stopping on `failed`, and a held message the agent reads anyway. Each is decided in one
function with no harness in it — the inbox server's shutdown, the question's status
watch, the read-is-final rule of the status writer — and the unit tests drive that
function with the states the suite would produce; a suite case would add the fixture's
timing and nothing the fixture decides.

**Live, through the wrapper's own reply socket.** Observed by the reviewer on
September 23, 2026, Claude Code 2.1.280, in a private HOME with a fake key and an
unreachable API, rewake built from this tree:

- With `--settings '{"crossSessionInbound":"hold"}'`: `send` printed `held for
  hold-claude: …` and exited 3; the screen showed `Held peer message — from
  uds:…/hold-claude.<pid>.<epoch>.reply.sock [verified pid <wrapper>]`; after two Ctrl-C
  the status became failed, `…expired unreleased: Your held message expired without
  approval…`; the letter went to `done/` and the reply socket was removed.
- Without hold: a send right after the socket appeared was `delivered to acc-claude via
  socket`, with no Released line.
- Under the trust dialog: held, then after approval delivered, `released after being
  held`, with the screen line `Released 1 held cross-session message… (permissions are
  prompting again)`.

**Open.** A held message is still held: the person at the receiving session releases it,
not rewake. A word more than a minute late is ignored. Not verified live: a recipient in
bypass, and the five-minute deadline.
