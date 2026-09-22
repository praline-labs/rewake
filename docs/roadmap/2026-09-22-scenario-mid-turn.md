# The mid-turn scenario, as built — September 22, 2026

What building the third selected scenario taught. The contract it was built to is
section 3 of [check-runner-scenarios.md](../check-runner-scenarios.md).

The question belongs to the server, not to the product. rewake sends the same request
whether the recipient is idle or working — a `turn/start` — and the server decides
between starting a turn and steering the input into the one already running. So the
fixture had to learn that fork before the scenario could exist, and what was
established about it is in [research-protocol.md](../research-protocol.md): a steered
`turn/start` answers with the running turn's id and nothing else changes, no second
`turn/started` follows it, and a mailbox delivery can steer despite its empty input
list because a tool output is submitted as a response item rather than as user input.

The shim now plays that fork, and it also sends `thread/status/changed` when a turn
begins and ends. That second part was missing rather than new: the adapter reads
exactly that notification for its telemetry, so until now every fixture session looked
idle while it worked. Both messages go through constructors the shape check reads, and
the check gained a case for the status notification — its `active` variant requires
`activeFlags` beside the type, which a fixture inventing the message would have left
out.

The pending operation is the recipient's own. Under one switch the session holds its
first turn open, writes down that it opened an operation, waits for something to be
steered in, reads its mail again, and writes down that it closed it. The sender sends
the second letter only once the room's telemetry says the recipient is working, read
through its own `rewake list`. So the order the scenario judges — opened, steered,
closed — comes from one session's record of its own work, and no step of it is timed.

Three controls, one of them a mutation of the product. A letter sent only after the
recipient is idle again is not a mid-turn delivery, and the observation must fall: that
control is the contract's own, and it breaks nothing, which is the point — an
observation that cannot fail says nothing. The held operation failing instead of
answering is the other, and it must take down the original turn's outcome rather than
the delivery. The product mutant makes the wrapper decide active from a status
snapshot and wait while it says working — the thing the comment in
`internal/harness/codex/gateway/reservation.go` says must not be done, because native
start-or-steer decides that atomically.

That mutant taught something the scenario then had to be able to see. Waiting for idle
does not delay a delivery: the reservation has a three-second bound of its own, so the
delivery expires and the message is marked failed. A wrapper that waited for the turn
to end would therefore not deliver late — it would not deliver at all. The scenario
learned to tell those apart by having the sender record what became of each letter it
sent, accepted or refused with the reason its own rewake gave. Without that record a
letter refused and a letter still in flight are the same silence, and the controls
spent their whole window waiting for one that would never come.

Two observations have no control today. The order of the pending operation rests on
the recipient's record alone. And "the arrival created no second outcome" has none
yet, though one exists to be written: a mutant that minted a report on every delivery
instead of on the terminal event would steer normally and hand the sender two
outcomes for one turn. What cannot be built is a *crosswise* cell for it — the three
control worlds steer nothing, so in each of them the question has no subject and the
answer is that it cannot be judged, which is a red pair rather than an observation
surviving. So the crosswise check runs over the observations that do have controls,
each under the worlds of the others, and requires them to stand there.

The crosswise check is over observations rather than over pairs of controls, because
two of the three controls name the same observation and pairing control with control
ran that one twice under one world, under two names for one question.
