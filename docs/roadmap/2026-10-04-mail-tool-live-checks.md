# Stage M3 of the mail tool: the live checks, run

October 4, 2026. The live checks of [mail-bridge-live.md](../mail-bridge-live.md) ran
the same day on build 916f888 in the owner's logins: codex-cli 0.159.0 and Claude Code
2.1.284, a cheap model at low effort, about 20 Codex turns and about $1.6 of Claude Code.
The case-by-case results are in
[the run of October 4, 2026](../mail-bridge-live.md#the-run-of-october-4-2026), the facts
about the harnesses in [research-mail-tool.md](../research-mail-tool.md). The four
findings that needed code are built under revised rules and accepted after four rounds
of fixes; the stage is not closed, since the live checks of those rules and several gates
stay open ([below](#open)).

## What was found

- **Claude Code does not start a killed server again** (2.1.284). The channel's row for
  a lost last connection says "not connected, no notice" on the strength of probe 1; live,
  the next call finds the tool gone until the person reconnects it in `/mcp`, and the
  session is told nothing. A dead server needs the failing notice on Claude Code too.
- **A folder's trust dialog holds Claude Code's servers.** In a folder never trusted no
  server starts until the dialog is answered, while the hello timer counts from the
  harness's start; with the dialog open past 15 s it went failing and sent the session
  to the shell.
- **A Codex sub-agent's call fails the parent's channel.** Sub-agents are on by default
  in 0.159.0; the sub-agent's thread starts a second instance of our server, the gateway
  never sees its call, and the refusal by timeout counts as "calls not observed", which
  told the parent "rewake tool failed".
- **A managed MCP file stops a Claude Code launch.** With `managed-mcp.json` present the
  harness refuses any `--mcp-config` and exits 1, so `rewake claude` with the tool does
  not start; rewake has to see the file and launch without the tool.
- **The `-c` trust form in the plan was wrong.** Codex splits a `-c` key on every dot and
  keeps the quotes; the working form puts the path in the value. The plan is corrected.
- Smaller: the project-scope refusal says the check "may have started that server once"
  for a pending entry, which is not started; the Stop hook's pending reminder reads
  "main-claude still wait".

## What was done

- `closedGates` holds G1 for codex 0.159.0 and G5 and G7 for Claude Code 2.1.284, so a
  Claude Code 2.1.284 launch carries the tool with no assumption.
- The gate statuses, the corrected `-c` form and the run's table are in the launch and
  live documents; the claims the run refuted carry its answer where they stand.

## The rules revised

The same day, on main's directions, the rules were revised for the four findings and the
version read, before any code (each marked *revised* where it stands), in four review
passes, and built the same day ([below](#the-revised-rules-built)):

- [The harness version](../mail-bridge-version.md): taken only where a
  closed gate could change the choice, never by an extra run of the person's wrapper —
  Codex reuses the `--version` its transport already reads, moved before the claim;
  Claude Code reads it from the native installer's path of the `claude` on `PATH`, under
  a `--command` wrapper too ([the owner's decision](#the-owners-decision-on-the-claude-code-version));
  an unknown version keeps every gate open. The L5 bounds sit in a per-version table beside `closedGates`.
- A dead server is "server gone" on Claude Code as on Codex, told once to worker and
  main, main's notice naming `/mcp` ([the channel](../mail-bridge-channel.md#the-tool-observation)).
- Claude Code's hello timer opens at the first SessionStart, which comes after every
  startup dialog, together with the servers' start (live, scratch configuration).
- A Codex request whose `_meta.threadId` is not the primary thread is an agent's call,
  refused at once; a connection serves the thread its calls name, so a sub-agent's
  server keeps no tool alive for the parent.
- A managed `managed-mcp.json` present in any form means no tool and no name check;
  G6 closes once the code takes it.
- The cases still unrun and the live checks the revised rules need are in
  [mail-bridge-live.md](../mail-bridge-live.md#what-stays-unrun-and-what-it-leaves-open).

## The owner's decision on the Claude Code version

On October 4, 2026 the owner decided that a Claude Code launch takes the version from the
native installer's path of the `claude` on `PATH`, under a `--command` wrapper as well:
the owner runs one Claude Code version for all sessions, so the wrapper's own `claude`
is that one, and no extra run of the wrapper is spent to learn it
([mail-bridge-version.md](../mail-bridge-version.md)).

## The revised rules, built

Built on main's task the same day, with no live run; each document marks its rules built
and says how in its own *as built*:

- The version ([mail-bridge-version.md](../mail-bridge-version.md#as-built)): Codex's one
  `--version` read before the claim under rule 6, compared at start with `initialize`'s
  `userAgent` by the table of [the version, confirmed at
  start](../mail-bridge-launch-codex.md#the-version-confirmed-at-start), the injection
  withdrawn when it is not confirmed and no assumption keeps it; Claude Code's from the
  path of the `claude` on `PATH`, after rule 4; the L5 bounds per version on both
  harnesses. `wrapped-launch` is green: three wrapper runs on Codex, one on Claude Code.
- The managed MCP file by `lstat` (`claude/managed.go`).
- The channel: "server gone" on Claude Code with main's `/mcp` line, the hello timer from
  the first SessionStart, conversation connections bound by `_meta.threadId`, other
  threads' calls refused before any wait, and the selection transition
  ([as built](../mail-bridge-channel-codex.md#as-built)). Every row of
  [the failure table](../mail-bridge-channel-failures.md) is held by a test named after
  it, and the selection is a generated space against an oracle written from the rules.

## Four rounds of fixes

The build was the first round. Each round went to review-codex, every review the same
day, and each fix came with its counterexample pinned, tests and killed mutants.

The first review accepted the rest and found two defects, fixed in the second round:

- A call's outcome lost its conversation. The endpoint told a wait that timed out with
  no connection or thread, and the record took it, and any ticket, for whatever
  conversation was current — so a call of A waiting past B's selection turned B
  failing, and a ticket of A validated after it would have made B working. The
  endpoint now tells both with the request's generation and thread; the record judges
  them as that connection's events, and on Codex keeps every ticket and folds it by
  its own time ([as built](../mail-bridge-channel-codex.md#as-built)). The generated
  space gained these letters over either connection, and the failure table three rows.
- The withdrawal waited for the `--command` wrapper only, while a child of its group that
  ignores SIGTERM still lived. It now SIGKILLs what is left of the group and waits,
  bounded, until no member is left, refusing the launch past the bound
  ([the version, confirmed at start](../mail-bridge-launch-codex.md#the-version-confirmed-at-start)).

The second review closed both and accepted that the withdrawal proves the end of the
launch's process group only: a member that leaves it through `setsid` is a stated
boundary. It found one more defect, which the generated space had confirmed rather than
caught, fixed in the third round:

- A resume of the current conversation folded its held events over the state its own
  events had already left. A hello of an unbound connection, held before A's own close,
  came after the close at the answer, so the close stayed "server gone" though another
  connection was live then. The answer now refolds the pending period from the first
  admission — admissions, own events and held ones by event time
  ([as built](../mail-bridge-channel-codex.md#as-built)); the counterexample is a row of
  the failure table. The oracle had held own events and released held ones at the answer
  just as the record did, so it agreed with the mistake; it now reads ahead how each
  selection ended and folds the whole path once in event time. The space also runs six
  seconds apart, inside the hello timer, where the rewritten oracle found one more
  disagreement: an answer selecting the conversation again with no admission before it
  cancelled the conversation's running timer, which now goes on.

The third review closed it, accepted the rewritten oracle and that an answer with no
admission keeps the running timer, and did not accept the reading of the timer at a
reselection, fixed in the fourth round: own events had been refolded so that none ended
the timer, a
hello included. So when A's server was gone and A was resumed, a second server of A seen
and closed before the answer showed "no hello observed" at the timer's end rather than
"server gone" at its close — and only when its binding to A was known; unbound, it
passed. Once the answer selects A, an own hello or ticket now ends the wait at its own
time, while an own failure still ends none, and nothing ends the timer before the answer
([as built](../mail-bridge-channel-codex.md#as-built)). The case is a row of the failure
table; the oracle takes the same rule from the contract, and the space six seconds apart
then catches the old reading by itself, on `bound-2-A admit hello-2 selected-A`.

The fourth review accepted the code of the revised rules with R1–R4 closed. It accepted
that a failure of A's own connections, refolded when the answer selects A again, still
ends no timer: a close of an old connection does not prove the start the selection
waited for, while a hello or a ticket does. The reviewer's probes of the four rounds are
in the tests where they add to the generated space and the failure table; the rest
duplicated them.

## Open

The live checks still to run
([mail-bridge-live.md](../mail-bridge-live.md#what-stays-unrun-and-what-it-leaves-open)):

- G6 in its container: a launch with the managed file present starts without the tool,
  and the tool returns once the file is gone.
- L5 on the confirmed versions: 861 keeps Codex's reads on and 860 turns them off;
  Claude Code's bound of 2048.
- The version unconfirmed: a `--version` and a `userAgent` that disagree, or none,
  withdraw the injection — built as unit tests only.
- L8's three outcomes on Codex: the parent's server killed after a sub-agent's call, killed
  before its first call, and a sub-agent's server that never calls.
- G8b, the rest of L10, L11 on Codex, the thread-request part of L3, and G4.

The open gates: G2 (no call lists registrations without starting them), G3 (the gateway
checks the server's cwd, where a resume runs in the thread's recorded one), G4 (not
run), G6 (built, not seen live), G8 (the user, project and managed layers and a change
during a session), G9 (a request's `config` reaches the layers unfiltered) and L4 (no
native denial signal on either harness).
