# Milestone 9. Roles — done, September 16, 2026

The owner, after restarting under the new build: the session handing out work
got a report of its own turn back at its worker, and the two would wake each
other forever. Roles now live in a catalogue, `internal/role`, and the one that
hands out work has role `main`: it gets every report and reports
nothing. Main now requires explicit `--main`; omitted flags and explicit
`--general` keep reporting behavior. The `write` role now reports like a worker
and is eligible for explicit main-authorized Git metadata access, independently
of reporting. The September 19 decision removed automatic role-based grants.
