# Main closes a worker it raised

October 19, 2026, by the owner's decision of that day: a worker is never switched or restarted in
place; when another is needed, main closes it and raises a new one from a template. **Done.**
`rewake space close <name>`, refused from any role but main and for a space of another run of
main, moves the record to closing with "main closed it with rewake space close"; the space host
sees it at its next tick and ends the worker by the sequence of main's end (SP10). The command
answers once the record is closed or kept, within 20 s; a task still owed reads as lost in
`--awaited`. No restart verb. Held by `TestFixtureSpaceClose` on the fixture.
**Open.** Review; a live run with a Claude Code worker.
