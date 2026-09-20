# Owner check for native arrival display

The [September 20 acceptance](native-mailbox-ui.md#evidence-and-installed-acceptance)
is complete. This recipe is for a future acceptance need, not a request to rerun it.
It uses the preserved local UI fixture, which is outside release artifacts. One
new test terminal, a private HOME/config/state and a local scripted endpoint keep
this check independent of working sessions and real accounts/models.

From the checkout containing the reviewed fixture and its pinned binary:

```bash
python3 handoffs/native-notices-2026-09-19/work/ui-integration-1/fixture/run.py \
  --mode owner --case lifecycle
```

The runner validates binary/schema fingerprints and hard mount/network/PID isolation
before native launch. Its fresh private config contains top-level defaults:

```toml
approval_policy = "never"
sandbox_mode = "workspace-write"
```

The private workspace has an explicit trust selection. Both children read this
profile-less user-config layer. No approval/sandbox/profile override belongs in
remote TUI argv: those flags caused the first prototype's /resume refusal. No owner
configuration is edited, and the sandbox is not bypassed. This is fixture setup,
not a new production permission grant or proof of deliberate read-only inbox support.

1. Wait for the automatic idle row and local stub reply. Check
   `Ran rewake notice --display-only` and the short notice below it. The label does
   not execute a command and is not a new CLI command/flag.
2. If lifecycle behavior is part of the current check, use /new, /resume to return,
   and /agents. Record refusal, repetition or picker anomalies without guessing
   cache semantics. Persistence is not required by the accepted transient scope.
3. Exit with /quit and record the appearance, observations, exit/cleanup and printed
   result directory. Do not repeat probe-exec/probe-stream for a label-only check.

Lifecycle mode marks exec/stream not-run and returns awaiting-lifecycle-observation
when setup, idle delivery and cleanup succeed. It does not automatically mark
rendering/resume/picker successful: those observations come from the owner. Earlier
exec/stream evidence stays in its own run. Native streaming may defer a row until
text finishes; immediate display is not promised.

After a separately authorized installation/restart, send one short task between the
chosen sessions and verify its automatic finished report, actual inbox read and
visible Ran row. Compare the installed hash to the reviewed binary. No installation,
restart, broad recovery test or UI-history reconstruction is authorized by this recipe.
