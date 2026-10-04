# Failure points of the mail channel

The points the rules of [mail-bridge-channel.md](mail-bridge-channel.md) meet, one row
each: what the wrapper has proven at that point, what stays unknown, and what rewake
does. The terms — conversation connections, the failure interval, the policy block, the
shell observation, the categories shown — are that document's. Rows marked *revised*
follow the rules revised after the live checks of October 4, 2026, built the same day.
Every row is held by a test named after its Point: `internal/channel/table_test.go` and
`table_codex_test.go`, with the few that need the wrapper or the endpoint mapped there
to theirs; `TestEveryFailurePointOfTheTableIsHeld` reads this table and fails on a row
no test names.

| Point | Proven | Unknown | Rewake does |
|---|---|---|---|
| a hello, no call yet | a server reached the endpoint | whether calls are permitted | connected; nothing told |
| a policy denial (signal of L4) | the harness refuses this tool | — | denied; main told once; the worker told nothing |
| the block, then a disconnect, a transport error, a hello or a timer | — | — | denied stays; no fallback advice |
| a long call started, a denial recorded, the old call answers | the old call ran | whether the policy permits now | denied stays: its ticket is older than the block |
| a denial of one call by the person's hook | that call refused | whether others will be | unchanged, the call's own answer |
| working, then calls refused for no hook observation | the observer is gone | why | failing, "calls not observed"; told |
| a call times out with its child alive | the harness gave up waiting | the effect | unchanged; the receipt holds the outcome |
| the tool committed, the answer lost, the server gone | the effect is recorded | — | failing; the worker told "no repeats", the briefing names `retry` |
| the same failure several times between two shell observations | the tool fails | — | one interval, its start kept |
| two servers, one closes | the other is live | — | unchanged |
| an old server's EOF after a newer hello | it ended | — | ignored: the newer one lives |
| the last of two servers closes, the older one | no server lives | — | failing, "server gone" |
| a connection's hello delivered after another connection's close | it was live then | — | that close is no failure, whatever was told meanwhile |
| a startup failure delivered after a later hello | it failed first | — | failing from its time; the hello noted as a reconnection |
| a stray process's refused hello while a server works | it holds no capability | who it is | logged, unchanged |
| Claude Code's server gone an hour after its last call | it closed | — | failing, "server gone"; told: 2.1.284 does not start it again |
| a Codex sub-agent calls the tool | its thread is not the primary one | — | that call refused as an agent's; the channel unchanged |
| Codex: the parent's server closes, a sub-agent's bound earlier still lives | no conversation connection | — | failing, "server gone" at the close |
| Codex: the parent's server closes, then the other connection's first call names a sub-agent | none of the conversation lived from the close | — | failing, "server gone" placed at the close, told at the binding |
| Codex: the parent's server closes, the other connection never calls | — | whose it is | unchanged: the compromise of an unbound connection, open |
| Codex: a binding folded after events that followed it | the connection's class from its hello | — | folded again from that hello by event time |
| Codex: a foreign connection's hello while the timer waits for the primary's | — | — | the timer runs on; the hello is not the conversation's |
| Codex: A working, B admitted at T1, selected at S, no hello of B | B expected a server from T1 | whether B's server starts | at S: A's connections foreign, its state gone untold, B starting; the timer from T1 passes at T1 + 15 s — B failing, "no hello observed", at that time (told at S if it passed before S) |
| Codex: the same, a failed startup status naming B at T2 before S | B's server failed | — | held; at S: B failing, "command cannot start", from T2, told at S; A's state gone untold |
| Codex: the same, a failed status naming another thread, or naming none, before S | — | whose it is | naming another: dropped at S; naming none: B's, as above |
| Codex: B's hello at T2 before S | a new server reached the endpoint | whether it is B's | held; at S: B connected from T2, the timer stopped at T2; a later binding to another thread folds it again |
| Codex: A's own server closes between T1 and S | A has no server | — | A failing, "server gone", at the close; at S the interval closes untold and B starts |
| Codex: a call of A admitted before T1 is not observed after S | A's observer did not report it | whether B's does | B unchanged: the failure is A's, by the call's connection, and A's connections are foreign from S (*revised after the build review*) |
| Codex: a call of A admitted before T1 is not observed between T1 and S | A's observer did not report it | — | A failing, "calls not observed", at its time, as its own close counts; at S the interval closes untold and B starts (*revised after the build review*) |
| Codex: a ticket of A validated after S | A's call ran | — | B unchanged, connected and not working on A's proof; the run's last ticket moves, a fact of the run (*revised after the build review*) |
| Codex: the selection refused, unknown or read-only, or the primary left empty | no conversation was selected | which one the terminal will select | held events dropped, the timer cancelled, nothing told; the display stays A's |
| Codex: a resume of the current conversation | its connection lives | — | no timer opens; held events fold for it; its own state goes on |
| Codex: a resume of A, an unbound connection's hello held before A's own close, then the answer | the held hello lived at the close | whose the connection is | the held events and A's own of the pending period fold together by event time: A goes on as it was, on the second connection, the close no failure, no interval; the failure told at the close stands, and main is told the tool works again (*revised after the build review*) |
| Codex: A's server gone, a resume of A, a second server of A seen and closed, then the answer | A's second server lived, then closed while the harness lived | — | the timer the admission opened waits for A's start, and once the answer selects A its hello is that start, at its own time: the close is the failure shown, "server gone" at its time, never "no hello observed" when the timer's end passes; the interval from the first close stays open, no ticket came, and nothing new is told (*revised after the build review*) |
| Codex: A working, B's server live and bound to B before T1, B admitted at T1, selected at S, no events between | B's connection lives | — | no timer opens; at S: A's connections foreign, its state gone untold, B connected — not working, A's proof not carried over |
| Codex: the same, B's last server closes at T2 between T1 and S | none of B's connections lives from T2 | — | the close held; at S: B failing, "server gone", from T2, told at S; A's state gone untold |
| Codex: the answer delivered after B's hello and status, all timed before it | each event's own time | — | the same outcomes as in time order |
| no hello within the timer | no hello was seen | whether a server started and failed before its hello | failing, "no hello observed"; told |
| a hello at the timer's very end | a server started in time | — | connected |
| a failed startup status while a server lives | a server is connected | which server the status was about | unchanged |
| the tool fails, then the shell fails, then the shell works | the shell works now | the tool | shell; main told |
| no tool at launch, then the shell fails | the shell fails | — | no channel; main told |
| a late shell success from a closed interval | it worked then | now | ignored by its event time |
| the inbox that would carry a notice cannot be written | — | whether the recipient knows | the fixed notice retried with its ID |
| a shell-advising notice to the worker fixed, its write failed, then a denial | the block is set | — | that notice dropped under the mailbox lock; nothing advises the shell |
| main restarts during a suppressed failure | — | — | the new epoch gets the current category |
| the harness exits, or the wrapper accepts SIGTERM or SIGHUP | — | — | the record frozen; no notice; a notice fixed and not yet written is dropped |
| a server closes, and the harness exits within a heartbeat | — | — | not a failure: the close was the end's |
| a close, then later failures, folded after them | — | — | the interval starts at the close; the class is the latest failure's |

The sequences above are what the generated tests of the channel check against the
text: each one's display, notices and their count are asserted, not only the
transitions the implementation makes.
