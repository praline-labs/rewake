# Unified check runner — future work

Owner decision, September 19, 2026. Add one development command that runs the five
required checks and explicitly selected integration scenarios. Reuse current tests,
isolated fixtures and the exact production mirror; no large new framework, private
handoff-path dependency, owner credentials or real model calls.

Normal output is a short PASS/FAIL summary plus summary.json. Record exact source/
build identity, check names, durations, exit codes, failing test names and log paths.
Open detailed logs only for failures, with a bounded excerpt in the summary. Preserve
failed runs when retrying. Missing tools, errors, skips and unrun manual acceptance
must remain visible; none counts as success.

Use actual message/receipt assertions for delivery, groups, peek/selected reads,
roles, epochs, side behavior and state. Synthetic local responses may support isolated
wrapper/native-server fixtures. Terminal rendering and model-consumption claims stay
separate unless that run establishes them.

This is a separate future task, not an extension of the current batch repair.
Native notifications remain the next priority after current review and acceptance.
No runner implementation or new acceptance is claimed here.
