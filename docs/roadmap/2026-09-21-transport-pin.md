# The Codex transport pin — done, September 21, 2026

The version the app-server transport was last observed working against moved from
0.154.0 to 0.155.1, and the string now lives in one constant,
`lastObservedServerVersion` in `internal/harness/codex/server.go`; the launch note
and the test fixture both read from it, the fixture repeating the literal on purpose
so that a typo fails a test instead of matching itself. The constant was first named
`verifiedServerVersion` and renamed the same day: two probes on a version — the
ordinary path and steer — are an observation, not a verification, and the name said
more than the evidence did. A mismatch produces a note, never a refusal: an
unobserved version is a reason to warn, not to stop a launch the owner asked for.

What was observed on 0.155.1, and what was not, is in [research.md](../research.md)
under the tag of that version; the schema facts are in
[research-protocol.md](../research-protocol.md). Open: every fact tagged 0.154.0 stays
unrechecked on 0.155.1, and moving the pin again means walking the fixture types
against the new schema by hand. Checking a new version in a disposable environment
before the owner installs it is queued in [work-queue.md](../work-queue.md).
