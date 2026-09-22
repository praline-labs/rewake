# Owner check for native arrival display

The [September 20 acceptance](native-mailbox-ui.md#evidence-and-installed-acceptance)
is complete. This describes what such a check consists of, for a future acceptance
need — not a request to rerun it.

The fixture that performed it was a Python harness in a local research package,
deleted on September 22, 2026 along with the rest of that package. It is not coming
back: a shim server, a client and a protocol check already exist in Go under
`test/workflow`, and a second stand in a second language would be a third copy of the
same knowledge. What the fixture *did* is written down here, because that part is
worth keeping and the code was not.

## What is being checked

One thing only, and it is the thing no suite can check: **whether a person sees the
arrival**. Everything around it — that the message was delivered, read, reported and
correlated — is machine-checkable and already checked elsewhere. This check exists
because a row on a screen is observed by an owner or not at all.

## What the fixture set up

- **A disposable session, isolated from everything.** New mount, PID and network
  namespaces; a private `HOME`, config and state directory; no network route beyond
  loopback; no `REWAKE_*` variable inherited. The runner asserted each of those before
  launching, and refused to start if any failed — an isolation that is merely intended
  is the kind that quietly is not there.
- **A local scripted endpoint instead of an account.** A small HTTP server answering
  the model protocol, so that the session runs, executes a command and streams text
  without a real account, a real model or a paid turn. It also asserted that the
  cosmetic display label never appeared in what was sent to the model — the row is for
  the screen, and a leak into the context would make it something else.
- **Permission defaults in the private user config, never on the argv:**

  ```toml
  approval_policy = "never"
  sandbox_mode = "workspace-write"

  [projects."/work/workspace"]
  trust_level = "trusted"
  ```

  The launch checked its own arguments for permission flags and refused if it found
  one. That is not fastidiousness: approval and sandbox overrides on a remote TUI's
  argv are what made the first prototype's `/resume` refuse with "Permission overrides
  are not supported when resuming a remote task".
- **Fingerprints before anything ran.** The binary under test and the event schema
  were compared against recorded hashes, and the installed native CLI against its own,
  so a run could not silently be of something else.

## What it did, and what the owner watched for

1. Wait for the session to report itself ready, then send one ordinary message to it.
   The owner watches for the arrival row — `Ran rewake notice --display-only` with the
   short notice under it. The label executes nothing and is not a CLI command.
2. For the activity phases, send a message while a command is running and again while
   text is streaming. The controller detected those states rather than guessing at
   timing — a marker file for the running command, an event for the stream.
3. For a lifecycle check, use `/new`, `/resume` and the session picker, and record
   refusals, repetitions or picker anomalies without inferring cache semantics from
   them. Persistence is not required: the accepted scope is a transient display.
4. Exit with `/quit`. The runner then removed the private state, confirmed the
   namespace runner was reaped, and recorded the exit code — a check that leaves
   processes behind has not finished.

The machine half never marked the rendering, the resume or the picker as successful:
only the owner sees the screen, so those stayed false in the result and were filled in
by the owner's own words. Streaming may defer a row until the text finishes; immediate
display was never promised.

## Afterwards

After a separately authorized installation and restart, send one short task between
the chosen sessions and verify three things: the automatic finished report, an actual
inbox read of it, and the visible row. Compare the installed hash to the reviewed
binary. No installation, restart, recovery test or UI-history reconstruction is
authorized by this recipe.
