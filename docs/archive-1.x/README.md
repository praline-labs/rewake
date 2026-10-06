# The archive of 1.x

The documents of 1.x code that stage 3 of 2.0 removes, moved here unchanged by the
step that removed their code (`docs/v2/stage3.md#documents`). An archived document is
never edited: it records how 1.x worked, and the records that link to it keep their
bytes too.

A moved document keeps its links as they were written, from the directory it left. The
documentation tests resolve them from there and resolve a record's link to a moved path
through the table below; a forge's file view does not, so a link inside an archived
document may open a path that is no longer there. The table says where each document
was.

| Document | Path before the move | Step | SHA-256 of its bytes |
|---|---|---|---|
| [protocol-cutover.md](protocol-cutover.md) | `protocol-cutover.md` | S2 | `0d7c397ca98c219d51bd411a8bde5f7614fe0aa777bf9c4e718f20ac23fdac38` |

- [protocol-cutover.md](protocol-cutover.md) — how a mailbox passed from the protocol of
  builds before September 30, 2026 to the one of turn-end journals: the launch order, the
  states of the successor, run records named by boot and epoch, the upgrade the
  automatic cutover was bounded by and the look for earlier-build writers within it, and
  what a build refused or held for a run of an earlier build. Removed in S2 with the
  conversion and the cutover; open it to read a record that names them.
