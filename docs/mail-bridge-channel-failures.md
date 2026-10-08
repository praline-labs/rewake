# Failure points of the mail channel

The points the rules of [mail-bridge-channel.md](mail-bridge-channel.md) meet, one row
each: what the wrapper has proven at that point, what stays unknown, and what rewake
does. The terms — connections, the failure interval, the policy block, the shell
observation, the categories shown — are that document's. Since S8 (October 8, 2026) the
rows are the neutral alphabet's: the points of Codex's conversations, of a server's own
report and of a call no hook observed left with the 1.x adapter
([archive/1.x/codex](../archive/1.x/codex/README.md)). Every row is held by a test named
after its Point: `internal/channel/table_test.go`, with the few that need the wrapper
mapped there to theirs; `TestEveryFailurePointOfTheTableIsHeld` reads this table and
fails on a row no test names.

| Point | Proven | Unknown | Rewake does |
|---|---|---|---|
| nothing observed before the timer's end | — | whether a server is starting | starting; nothing told |
| a hello, no call yet | a server reached the endpoint | whether calls are permitted | connected; nothing told |
| a policy denial (signal of L4) | the harness refuses this tool | — | denied; main told once; the worker told nothing |
| the block, then a disconnect, a refused hello, a hello or a timer | — | — | denied stays; no fallback advice |
| a long call started, a denial recorded, the old call answers | the old call ran | whether the policy permits now | denied stays: its ticket is older than the block |
| a denial of one call by the person's hook | that call refused | whether others will be | unchanged, the call's own answer |
| a call times out with its child alive | the harness gave up waiting | the effect | unchanged; the receipt holds the outcome |
| the tool committed, the answer lost, the server gone | the effect is recorded | — | failing; the worker told "no repeats", the briefing names `retry` |
| the same failure several times keeps one interval | the tool fails | — | one interval, its start kept |
| two servers, one closes | the other is live | — | unchanged |
| an old server's EOF after a newer hello | it ended | — | ignored: the newer one lives |
| the last of two servers closes, the older one | no server lives | — | failing, "server gone" |
| a connection's hello delivered after another connection's close | it was live then | — | that close is no failure, whatever was told meanwhile |
| a refused hello from the harness's tree before any hello | a server of the run was refused | why it was built so | failing, "server refused"; told |
| a refused hello delivered after a later hello | it was refused first | — | failing from its time; the hello noted as a reconnection |
| a stray process's refused hello while a server works | it holds no capability | who it is | logged, unchanged |
| a server gone an hour after its last call | it closed | — | failing, "server gone"; told: no harness is taken to start it again |
| no hello within the timer | no hello was seen | whether a server started and failed before its hello | failing, "no hello observed"; told |
| a hello at the timer's very end | a server started in time | — | connected |
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
