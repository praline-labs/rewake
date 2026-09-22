# The batch-arrival scenario, as built — September 22, 2026

What building the grouping scenario taught. The contract it was built to is section 2
of [check-runner-scenarios.md](../check-runner-scenarios.md).

Two sessions again, both shims: the sender waits for
the recipient's accepted conversation — read from its own `rewake list` — and only
then sends two letters at once and a third three seconds later. Mail sent before
readiness waits in the mailbox and is folded into the first group by the readiness
refresh: a scenario that sent early would measure readiness and call it the window.
The recipient reads as the contract describes — an overview, then one member at a
time — and leaves the last member of a group unread until the next delivery. That
deferral gives the no-replay observation something to contradict; three seconds is
longer than the retry interval that gates a replay of settled mail, so a replay has
had its chance before the third letter arrives.

Every observation comes from the recipient's own records: which turn named which
members with what notice text, and each inbox call with its outcome. The two close
letters are found by text in its overview, so their ids and times are what the
recipient saw. A group names members in id order, not time order: the deferred one
is whichever came last in the notice, and the scenario asks the record rather than
assuming — the first version assumed, and waited for a read that had happened.

The controls mutate the product, not the fixture: the binary a control runs against
is rewake with one file changed, built through the toolchain's overlay
(`mutant_test.go`), so the working tree is never touched and nothing in the product
knows it can be mutated. Widening the window to ten seconds folds the third letter
into the group; reversing the comparison that picks the latest member previews the
earliest; making the overview branch mark what it lists refuses every read after it.
The replay needs two edits in one file, because the guard has two halves: the waiting
copy of a delivered message is removed once its notice is out, and a pass that still
finds one treats a recorded outcome as settled. The first version changed only the
second half and stayed green — with the copy gone there was nothing left to find —
so the file-level half is the one that holds, which is where the guarantee lives.

Each control is judged on the whole shape of its breakage — a delivery naming all
three letters, a group notice previewing the earliest, an overview that listed both
followed by refused reads, an id in two deliveries. There is no settling pause: the
anchor is all three letters listed in an overview and both close letters attempted,
which is what the judgement needs. The third letter belongs in that anchor because of
the replay: a replay is gated by the server's two-second retry interval while the
third letter is sent after three, so under the replay mutant every read is done
before the third letter exists. Judged there, the grouping question had a record that
could not answer it — and said so, which is the point of the next paragraph.

Each answer is one of three: broken, not broken, or not judgeable. A record the check
cannot read fails the pair instead of passing as an observation that held, and an
empty record counts as unreadable — it is what a renamed switch produces, and every
branch would otherwise report that nothing was announced twice and no delivery named
all three. Checked by breaking it: with the switch the recipient writes its
deliveries under renamed by one word, all twelve crosswise pairs answered "could not
judge", none of them "held".

The crosswise check runs every control against every other control's mutant and
requires the observation to stand there; a control that fires on somebody else's
breakage is not watching what it claims. Which way a pair must come out is stated in
its case rather than left to the caller — the first version reused the must-break
case for both directions and reported all twelve correct results as failures. One
case per pair, so each carries its own deadline: twelve sharing one exhausted it,
and the pair running last was blamed for a clock it never used. Switch:
`REWAKE_WORKFLOW_CROSS=1`.

A fixture boundary: the shim ignores an unknown switch, so a renamed one leaves its
control passing as a healthy case; making that a refusal is a separate decision.

Two rows of the [feature map](../harness-features.md) are claimed here, HF-07 and
HF-20, and one is not: HF-08 is `rewake inbox` with no arguments, a read of
everything waiting, which this scenario never runs and could not — a read-all would
consume the members whose separate reads it asserts.

Two observations have no control of their own: the third letter being announced
outside the group falls with the widened window, which is already the control for
membership, and one-read-per-member falls with nothing. Neither is mutated on its
own, so both rest on the healthy run alone.
