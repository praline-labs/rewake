# Milestone 10. Rooms — done, September 16, 2026

**Owner decision:** rooms isolate session discovery, addressing and delivery.
`--room <name>` is launch-only and defaults to `default`; agent commands inherit
`REWAKE_ROOM`. `REWAKE_DIR` remains the shared root; `rooms/<room>/` holds each room's
sessions, mailboxes and sockets. Old root-level records are ignored.

Role choice and name publication share a room lock. Without a role flag,
launches now always use general; the September 17 decision supersedes automatic
main selection. Explicit --main refuses with the live main's name when occupied.
General and write can start first. Main/write are eligible for explicit grants;
list, whoami, the launch note and intro identify the room and resolved role.

**Acceptance, verified:** two rooms with identical session names cannot see or
message each other; task notices and final reports stay in their originating
room. The original first-session election checks are historical and superseded.
Current checks require every unflagged launch to use general and concurrent
explicit --main claims to have one winner. They use isolated state and fake
harnesses, plus regression and mutation tests.
