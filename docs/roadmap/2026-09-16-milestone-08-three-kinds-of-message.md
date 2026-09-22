# Milestone 8. Three kinds of message — done, September 16, 2026

The owner's call after the first real rounds with Codex: a heads-up should not
wake anybody back, a question should block until it is answered, and work is
reported by ending the turn with the result rather than by another send.

- `task` (default): the reader owes a report; the report is its final message.
- `question` (`--question`): the same, and `send` blocks until the answer and
  prints it; the answer is not announced to the asking agent a second time.
- `notify` (`--notify`): owes nothing.
- The intro and the guide say how to answer: end the turn with the result.
- Each kind is its own file in `internal/cli`, listed in one table.

In Codex the notice keeps the 🟢: Codex strips control characters from a user
message, so a coloured `●` like Claude Code's is not possible there.

Earlier findings and fixes are in the [review history](../reviews.md) and [its later part](../reviews-later.md).
