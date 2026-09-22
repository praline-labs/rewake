# A role-shaped first page — done, September 21, 2026

An agent now reads what its own role does, in the briefing it is launched with and again
in `rewake guide`, which recognizes the session calling it and opens with that role's
moves. A caller that is not a session sees the general map, unchanged.

Both come from one place, `internal/role.Playbook`, the `Play` field of a role: the
heading, the ordered steps and the limits are written once and rendered twice. A copy would have drifted the
way copies here have drifted before, and it would have drifted unevenly — the briefing
arrives first and is read once, the guide is consulted later and often, so a
disagreement between them would be settled in favour of whichever the reader saw first.
A test requires the briefing to carry every step and limit of the playbook.

What prompted it: on September 21, 2026 an executor sent every report by hand *and* let
the turn report it, all day. `flow.md` and `delivery.md` both described the mechanism
correctly; no instruction said what to do about it. The playbook says it in one line —
ending the turn is what sends the report — and the guide repeats it to the same session
later.
