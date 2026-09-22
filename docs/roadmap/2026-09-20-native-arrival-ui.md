# Native arrival UI — accepted, September 20, 2026

Two independently reviewed prototypes established downstream-only display and the
fixture correction needed for remote /resume. The owner observed idle/exec/stream
rows and later accepted the lifecycle check without a reported picker anomaly.
Transient display is explicitly sufficient; persistence/reconstruction/replay of UI
rows is deferred, while durable mail and report accounting remain mandatory.

The integration review reproduced the five checks and native synthetic cases with
no defect. The owner accepted the new `Ran rewake notice --display-only` label,
installed the reviewed binary and restarted main/write. UI-INSTALLED-OK returned as
an automatic native finished report and was read; the owner confirmed visible rows.
[The accepted contract](../native-mailbox-ui.md) retains native Ran, deferred streaming
and independent delivery/read/report truth. [Review history](../reviews-native-ui-2026-09-20.md)
keeps the initial permission-override refusal and limited owner timing evidence.
Final closeout changes only documentation after review; no new runtime behavior or
UI persistence mechanism is added. Other harness behavior is unchanged.
