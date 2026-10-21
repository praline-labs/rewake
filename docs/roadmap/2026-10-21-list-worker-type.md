# `rewake list` shows a worker's type

October 21, 2026, on main's decision ([worker-types.md](../worker-types.md#in-rewake-list)). **Done.**
`space open` records the type's name as the optional `worker.type` of the space record (additive, `omitempty`;
older records read as none, the text stays in the spec). `rewake list` joins session to space by `worker.run` and
adds a `Type` column after `Role` only when some session has one (`-` otherwise), and `type` under `--json`.
Tests: the column's presence and its `-`, the key on disk. **Open.** review-pi's acceptance of the record change;
not run live.
