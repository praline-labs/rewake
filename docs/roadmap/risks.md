# Risks

| risk | mitigation |
|---|---|
| Claude Code's private socket protocol changes or gets disabled remotely | delivery is isolated in the adapter; on socket failure, a clear `failed`, not silence |
| owned-server thread identity is unavailable or ambiguous | fail explicitly; retain accepted reports when their notification fails; terminal ownership remains open |
| a fresh thread finishes before observer subscription | first input can start immediately; an observation gap reports an error without inventing a result |
| the wrapper died, the harness is alive | the session is treated as dead; delivery fails with a reason instead of going silent |
| identical text in a row gets dropped by the recipient | a short id in every message's header |
