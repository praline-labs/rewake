# Owner check for native mailbox delivery

This is the corrected recipe after the September 20 [accepted owner and installed
checks](native-mailbox-acceptance.md). The original hashed research recipe remains
historical. Repeat only for a new acceptance need: real model turns spend account
quota. Use new test sessions, not the owner's working terminals or state.

For UI-only lifecycle checks, use the [corrected display recipe](native-mailbox-ui-check.md):
private config defaults, no CLI permission overrides. The real-account model-delivery
procedure below did not test remote /resume; its per-launch permission flags are not
suitable for that continuation check.

## Prepare an explicit test environment

The owner must choose permission to write the disposable workspace and shared
mailbox state. A fresh project with no trust/sandbox selection can default to
read-only; inbox then cannot write its lock/read receipts. The accepted run used
per-launch workspace-write. If the intended check is deliberately read-only, do
not override it using this recipe or call this existing acceptance sufficient.
No persistent configuration edit or automatic runtime grant is required.

Create a temporary directory with `workspace`, `state` and `bin` subdirectories.
Copy the reviewed executable to `bin/rewake`, preserving its executable mode and
verifying its expected SHA-256. Use a copy, not a symlink: the wrapper prepends its
own executable directory to PATH so ordinary `rewake inbox` uses that same binary.
This temporary copy is not an installation.

In each of TWO new terminals, use that same absolute temporary directory:

```bash
export MAILBOX_CHECK_DIR=/tmp/rewake-mailbox-owner-REPLACE
unset REWAKE_SESSION REWAKE_EPOCH REWAKE_ROOM
export REWAKE_DIR="$MAILBOX_CHECK_DIR/state"
cd "$MAILBOX_CHECK_DIR/workspace"
```

Start the orchestrator in the first terminal:

```bash
"$MAILBOX_CHECK_DIR/bin/rewake" --main --name acceptance-main codex \
  -c 'sandbox_mode="workspace-write"'
```

Start the worker in the second terminal:

```bash
"$MAILBOX_CHECK_DIR/bin/rewake" --write --name acceptance-worker codex \
  -c 'sandbox_mode="workspace-write"'
```

Keep ordinary tools/account configuration. The owner may choose an inexpensive
model using normal arguments and record that choice. Do not add a synthetic endpoint,
dynamic gate, tool restrictions or per-message instruction seed. Confirm that the
generated briefing was not skipped; custom developer instructions or --no-intro
invalidate the briefing-only premise. Do not replace user instructions to force it.

## Observe idle, then normal active work

1. Type no task or mailbox instructions into the worker. Ask main to send
   `Calculate 17 times 19 and give the result in one line.` to acceptance-worker-codex
   using `rewake send`, without --grant-git. The worker should wake, read inbox and
   return 323. Main should receive and read the automatic finished report.
2. Observe whether either notification appears as an ordinary user-message bubble;
   record reconnect/read-only problems and unexpected replies or loops.
3. Ask main to send a task to run an ordinary shell command that prints a start
   marker, sleeps 30 seconds and prints an end marker, then reports completion:
   `printf 'wait started\n'; sleep 30; printf 'wait finished\n'`.
   While the tool is running, send a notify asking to include the product of 23 and
   29 in the final result. Do not interrupt the tool or type into the worker.
   The final task report should include 667. Start/end markers improve visibility;
   they were not present in the already accepted sleep check.
4. Record timing honestly. If the notify arrives after completion, active delivery
   was not exercised. Owner observation without an event trace does not establish
   exact same-turn admission. No report should be manufactured for notify alone.
5. Exit only these new sessions with /quit. Record cleanup, ordinary tool behavior,
   permission/briefing selection, task/report results and terminal observations.
   Preserve bounded acceptance notes, not unrelated transcripts or credentials.

## After an authorized installation

Restart only the sessions the owner chooses. Check availability delivery, then send
one short task and verify its automatic finished report arrives as native mailbox
output and is read through inbox. Compare the installed executable hash with the
reviewed artifact. This checks installed task/report handling; it is not a substitute
for a recovery, side-selection or compaction matrix. This recipe authorizes no
installation or restart by itself.
