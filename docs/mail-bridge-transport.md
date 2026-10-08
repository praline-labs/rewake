# The mail tool through a harness's own process

From S7 a harness may carry the mail tool's calls itself, without a tool server of
rewake's: the harness registers the tools and its own process asks the wrapper's endpoint
to run each call (`docs/v2/design-api.md#tooltransport`). The fixture is the first harness
to do so ([stage3-fixture.md](v2/stage3-fixture.md#the-tool-transport)); the Claude Code
mod is meant to be the next. The rules of [mail-bridge-server.md](mail-bridge-server.md)
hold unchanged — T1–T11 in [rules/tools.md](rules/tools.md) bind every transport — and this
page says where the path differs.

## The tools

The CLI builds one descriptor per command its allowlist lets through — `inbox`, `send`,
`pending`, `whoami`, `retry`, `list` (`cli/bridge_descriptors.go`, from `toolFlags` in
`cli/bridge_surface.go` and the command table). A descriptor names the command, its
summary, and its parameters: a flag that takes no value (a switch), a flag with a value,
or a positional word. What follows from a descriptor alone lives in
`internal/bridge/descriptor.go`, so no transport imports the CLI: the JSON Schema a harness
registers (one property per parameter, nothing else), the command's words for a call's
arguments, and the digest of the whole set. The words are the CLI's normalized order —
flags sorted by name, then `--` and the positionals — so a descriptor's words parse back to
the same command the shell would run.

## The launch

The CLI lends the wrapper its descriptors and its check of a call's words beside the two
functions every transport uses (`wrap.MailTool`). A harness that carries its own calls
names the transport its calls are bound under (`ToolTransport()`); the wrapper starts the
endpoint under it, keeps no channel record (that record is the tool servers'), and offers
the backend the tools, the endpoint's path, and a function that names the process the
endpoint serves (`OfferTools`). Under `--no-mail-tool` nothing is offered and no endpoint
starts.

The fixture's backend hands the tools and the path to its program in the tool transport's
probe; the program answers the names it registered and their digest. The capability is
live only when both are those offered, and only while it is live does the backend name
the program's process — by pid and start time — to the endpoint; a withdrawal or the
program's end names none. The probe also says whether the program can show the result it
handed the model; one that cannot turns reads off (`ToolReadsOff`), so a reading tool
refuses before its first effect (T6).

## A call

1. The program reports the call it observed — its own call id, the turn, the tool and the
   arguments — and the backend hands it to the endpoint's neutral input (`CallSeen`), the
   words built from the descriptor. Only once that is answered does the program ask.
2. The program connects to the endpoint and says hello as `transport`; the endpoint takes
   one request per connection. The peer of the connection (`SO_PEERCRED`) must be exactly
   the process the backend named, alive with its start time — never a descendant, since a
   command the model runs is one — and no capability is involved.
3. The request is read within its bound before it is parsed (`maxCall`), and the peer is
   checked again as it arrives: the hello's proof does not carry over to a request sent
   after the transport was withdrawn or its process ended, which runs nothing. Then the
   tool is found among those offered, its arguments become words, and the CLI's own check
   refuses what the shell would. A refusal runs nothing and gets no ticket.
4. The endpoint issues the call's ticket from the request's binding — conversation, turn,
   call id, and the transport's declaration that its turn ids are never reused — matched
   with the observed call as on every transport, and runs the words in a child of its own
   image with the ticket on fd 3 (`endpoint/run.go`, the server's steps). The child
   confirms the ticket as any bridge-mode CLI does; the endpoint takes that confirmation
   from its own child, which runs below the wrapper rather than the harness.
5. Before answering, the endpoint checks the peer again: a transport replaced while the
   call ran gets no answer. The answer passes the endpoint's one encoder and bound
   (`endpoint/answer.go`); one that does not fit is replaced whole. At most four calls run
   at once; a fifth is answered busy.
6. The program reports the result it handed the model (`CallResult`), which acknowledges a
   read when it proves the whole answer arrived.

The program's turn starts and ends reach the same input (`TurnStarted` with the
program's own time of the start, `TurnEnded` after the end's capture), so a call is bound
only inside a turn the program was seen to start and not yet seen to end.

## Closing

As the wrapper ends, `Close` refuses new calls and drops every connection but a
transport's call under way and the confirmation of a child the endpoint runs, then waits
for the calls it took: each runs to its answer, under its child's deadline, and only then
does `Close` return, as the old server finished its calls at the end of its input. The
socket stays open meanwhile, to the endpoint's own children alone, so a call taken before
its child started, greeted the endpoint or asked for its confirmation still confirms; it
closes once the last such call ends. An answer goes to its connection with a write
deadline; at most `bridge.ResultCap` bytes, it fits a socket's buffer, so a transport that
stopped reading never holds the endpoint. An acknowledgment in flight is waited for the same way.

## Faults and the rig

The endpoint's own steps pass the state directory's fault seam under names qualified by the
context's path — `<path>/ticket`, `<path>/start`, `<path>/answer`, `<path>/acknowledge` —
so a rig can hold or kill the harness's process at one of them for one endpoint among
several. `test/toolrig` is that rig: the fixture's program is the harness's process, the
test plays the wrapper, and the faults reach the program, the child and these steps. It
holds the oracles `bridge/server`'s rig held, transport-neutral
([rules/tools.md](rules/tools.md#the-carried-order-and-fault-tests)).

## Until S8

The child's steps in `endpoint/run.go` repeat `bridge/server/child.go`: the server and its
rig stay beside the endpoint's path until S8 removes them with the Codex injection.
