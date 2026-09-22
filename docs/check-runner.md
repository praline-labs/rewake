# Check automation — research before implementation

Owner intent, September 19–20, 2026: one development command should run the five
required checks and selected workflow tests without filling agent context with logs.
The tests should exercise the actual feature process.

A second requirement arrived on September 21, 2026, and it changes the architecture
rather than adding to it: the suite has to stay convenient as harnesses are added.
A scenario is therefore written once and parameterised by harness, the way the feature
map gives a new harness a column rather than a copy of every row. A design that would
require rewriting scenarios for a third harness is disqualified, however cheap it looks
for two.

The same day set the priority between the two existing columns. The owner's account:
Codex was developed further and checked more, Claude Code was made first and came
easier, and something may not work right in Claude Code while Codex must not be
broken.
Because most of the mailbox is shared code, the Codex column is a regression gate and
the Claude Code column is where defects are expected to be found.

The answer to this research task is [check-runner-proposal.md](check-runner-proposal.md),
September 21, 2026. No runner is implemented or accepted by either document.

Native model-context delivery and the arrival UI are installed and accepted through
`4cfd3cf`. They are no longer a pending prerequisite or the next implementation
priority. Their fixtures provide evidence and lessons for this investigation, not a
ready-made general testing framework.

## Research deliverable

Two decisions, the first now answered:

1. **Architecture and scenarios.** Inventory reusable checks/fixtures, map feature
   claims to observable invariants, identify evidence gaps and propose a small suite.
   Decide process boundaries, result ownership, synchronization, artifact collection
   and how the suite proves its own negative cases. Document alternatives and costs.
   Delivered as [check-runner-proposal.md](check-runner-proposal.md).
2. **Runner implementation.** After that proposal is reviewed, implement the smallest
   orchestration needed for the chosen workflows. Reuse existing Go tests and mature
   fixtures; do not build a broad framework or tests that merely reproduce current
   implementation branches.

The investigation should produce a harness-by-scenario matrix linked to the
[feature map](harness-features.md). Use registered harness identities and the map's
capability/acceptance boundaries. A scenario needs a stated invariant, prerequisites,
observation points, evidence tier and reason for every unsupported or unrun cell.
Shared user contracts may use different native mechanisms; feature presence does not
mean identical protocol or equally strong acceptance evidence.

Orchestrator decision, September 21, 2026: the matrix is generated from declared
capabilities, not maintained by hand. A scenario asks whether its harness can be
observed doing the thing, never which harness it is. That keeps a third column to one
fixture plus a capability set, and keeps an unsupported cell honest — it names the
capability and the HF row it comes from. `unsupported` means the capability is absent
(**missing** in the map); a row marked **impl?** is implemented but unobserved, so its
scenario must run rather than be skipped.

## Output and evidence contract

Normal agent-facing output is a short console summary plus `summary.json`. Successful
checks do not stream detailed logs. Read a full log only after a failure or to answer
a specific evidence question; link directly to the relevant case artifact. Emit
bounded progress notices for long phases, not subprocess chatter.

Each result should retain:

- Check/workflow name, harness, scenario/variant and evidence tier.
- Result category, expected outcome, observed outcome and a concrete reason.
- Duration, subprocess exit code, timeout/interruption and failing assertion/test
  names. Exit 0 alone is never the workflow acceptance criterion.
- Source identity: commit plus a manifest/digest of the exact tested working tree,
  including uncommitted changes. Record exclusions from the production mirror.
- Build identity: executable hash and relevant build arguments. Native fixtures also
  record the native version/hash and any reference-source revision; do not assume
  source/binary equivalence from a version string.
- Case-specific evidence paths: scoped trace, message/receipt identities, native
  request/terminal summaries, effective configuration, cleanup and owner observations
  when applicable. A generic log path is not sufficient proof of an invariant.
- Observation provenance: which process saw or performed each step, including who
  invoked inbox. Retain separate native turn status and human-observation fields.

Use distinct results: `pass`, `fail`, `skip`, `unsupported`, `incomplete`, `not-run`.
A skip is an intentional selection/policy decision with a reason; unsupported means
that the harness lacks the required capability; not-run means no attempt was made;
incomplete means an attempt lacks required evidence. Missing tools or prerequisites
must be visible and cannot silently produce an aggregate pass. A required workflow
with missing evidence is not green. An optional paid/manual case may remain not-run
without being represented as accepted.

An expected-failure control may pass its assertion while recording a failed native
turn or refused admission. That is a successful negative control, not successful
model work. A happy-path failed/interrupted terminal, missing correlation, cleanup
failure or late endpoint error prevents a happy-path pass.

Finalize results only after owned subprocesses and endpoint handlers finish and
cleanup is checked. Keep attempts separate: a retry must not overwrite the failing
run or change an unknown outcome into success by repeating an accepted request.

Proposed MVP output budgets to validate during research: at most twelve normal
summary lines for the five checks plus three workflows; failure excerpts at most
twenty lines each and 8 KiB combined. Logs remain complete in artifacts. Confirm
that an agent can choose the next diagnostic from summary.json without loading
successful transcripts or broad directories of logs.

## Observable workflow invariants

The principal chain is sender, durable mailbox, native admission, recipient inbox
read, causal report, then sender-side native report notice. Verify each edge with
correlated message IDs, epochs, selected thread/generation and ACK turn where the
adapter provides them. Do not infer one edge from a later convenient signal.

| Scenario family | Required observations and boundary |
| --- | --- |
| Task and report | Real send, expected mailbox state, accepted native input, actual inbox consumption, report correlated to the read task, and matching report notice at the sender. Reading a report through CLI is not proof of its native announcement. |
| Idle and busy | Positive native evidence of idle start or genuine active work; for same-turn claims, match the original turn and admission ACK. A status snapshot or scheduled sleep alone is insufficient. |
| Batches and late arrivals | Fixed admitted member IDs/count/preview; later arrivals belong to another batch; old unread mail is not announced again. Reads and obligations remain individual. |
| Failure and recovery | Distinguish rejected, accepted and unknown ACK outcomes. Check allowed retry/wake behavior without blind replay. Stopped work is not automatically replayed; recovery observations retain the original causal scope. |
| Epoch and selection | Restart, changed epoch/generation, primary changes and side views cannot redirect old work or let a side settle primary obligations. Exercise only relevant selected boundaries, not every Cartesian combination. |
| State and privacy | Telemetry freshness/provenance, compaction start/completion and visibility rules follow actual observed scope. Do not infer departure from unknown process identity or read unrelated transcripts for convenience. |
| UI isolation | Arrival display is downstream-only, creates no phantom outcome or extra model context, and cosmetic loss leaves delivery accepted. Native rendering timing and cache behavior need their own evidence. |
| Permissions | Granted versus ungranted work; additive existing roots remain intact, roles do not imply grants, and effective native policy matches the selected source. |
| Cleanup | Owned processes/handlers end, sockets/registry/private state are cleaned as required, and late failures are reflected before final classification. |

A controller invoking the real recipient CLI can prove mailbox consumption and causal
receipt/report accounting. It does not prove that the model chose to read inbox.
Record `read performed by controller` explicitly. A model-driven read needs evidence
of that action from the model/tool path. A fake ACK proves only the simulated admission
contract; neither model comprehension nor terminal appearance follows from it.

## Evidence tiers and matrix

| Tier | What it can establish | What remains separate |
| --- | --- | --- |
| Pure Go | Deterministic domain invariants, refusal reasons, scope guards, queue bounds, receipt accounting and targeted races. | A native process accepting the wire shape or a model following it. |
| Protocol fixtures | Process wiring, framing, correlation and controlled ordering/failures through local doubles. | Native parser/behavior, model choice and rendering. |
| Pinned real native with local stub | Actual wrapper/native process flow, effective policy, real tool lifecycle and serialized context against a scripted endpoint. | Real-model semantics and actual TUI appearance. |
| Real-model semantic | Task understanding and model/tool choices under a stated setup, with strict marker/data provenance where relevant. | Other prompting conditions, comprehensive timing or visual behavior. |
| Owner TUI | Visible rows, interaction, deferral and observed transitions. | Unrecorded exact event order, durable cache semantics or exhaustive recovery coverage. |

Populate the harness-by-scenario matrix from [harness-features.md](harness-features.md)
and these tiers. Record the highest justified evidence for each cell and its gaps,
not a single ambiguous tested boolean. An unsupported native surface does not justify
emulating a different feature and reporting parity. Unknown cells remain research
items until a concrete observation or documented capability resolves them.

The default suite must be no-account and enforce isolation before native launch, and
the requirement differs by tier — owner decision recorded by the orchestrator,
September 21, 2026, after the first proposal showed that a single requirement for all
tiers could not be met honestly.

Every tier requires fresh private HOME/config/state, no owner credentials, config or
session mounts, a local-only endpoint, no inherited identity or proxy, and verified
binaries; required native helper dependencies live inside that boundary. The free
tiers may substitute the harness with a shim on `PATH` in place of separate
network/mount/PID boundaries: no real credentials are nearby, no network is used, and
a substituted harness does not reach outside. Those boundaries stay mandatory for the
paid tier, where real credentials live alongside. Failure to establish the isolation
its tier requires stops the case; it never falls back to the real account.

Paid semantic and manual TUI checks are explicit opt-ins with cost/time bounds and
separate result scope. No typing proxies or automated input into a person's screen.
A protocol client can drive a synthetic case, but must not be labelled a real TUI.

**The cheapest model, always.** Owner requirement, September 21, 2026: automated
testing done through a real harness runs on the cheapest models. A case that starts a
real harness with a real model sets, in that case's environment, the cheapest model
available and the lowest reasoning effort. The values come from the environment of the
run; they are not in this repository, and neither is any model name.

The reason is not thrift for its own sake. This tier exists to see whether a model read
its mail and acted on it — not to judge how well it works. The weakest model answers
that, and the cheaper it is, the more often such a run can be afforded at all.

So an unset value is a reason not to run the case, not a reason to fall back: a harness
left to its own default picks an expensive model, which is the one outcome this rule
exists to prevent. The case is `not-run` with that reason, and `not-run` is never
acceptance.

rewake itself takes the model and reasoning effort for a launch from the environment
([launch.md](launch.md)), so a case sets two variables and needs nothing else — no
model name in a test, and no harness configuration touched.

## Lessons that the proposed tests must address

- **Wrong guard, green test.** Generic error assertions passed because a reservation
  expired. Use a live precondition and the specific expected refusal; removing the
  intended guard should fail that test, rather than pass through another guard.
- **Premature success.** A failed terminal, cleanup failure under exit 0, or endpoint
  error after an early pass was hidden by weak classification. Observe terminal
  identity/status, join handlers and validate cleanup before publishing a result.
- **Wrong report edge.** Sender CLI read succeeded before the sender's native report
  notice. Match the exact report ID/epochs on the sender's native connection/endpoint;
  another availability notice or an extra HTTP request does not satisfy this edge.
- **Permission premises changed.** A fresh temporary cwd selected read-only and inbox
  could not write its lock. Record effective policy, not only launch intent. UI
  continuation then rejected CLI permission overrides; the corrected isolated recipe
  keeps equivalent defaults in its private profile-less config, with clean TUI argv.
  Do not widen owner permissions or return those flags to remote continuation.
- **Alternative data path.** A marker visible in an initial task, another tool result
  or readable fixture file can undermine a comprehension claim. Audit all model-visible
  channels and allowed tool paths, then use negative controls for the claimed source
  boundary. A preserved wire string alone is not semantic consumption.
- **Fixture evidence promoted to UI success.** Synthetic clients and scripted replies
  did not render a terminal. Preserve explicit not-run/manual fields. Initial visible
  UI and no noticed repeat do not prove persistence, exact ordering or cache invalidation.

Use mutations/negative controls only where they test the claimed invariant: remove a
specific guard, substitute the wrong identity/epoch, inject a terminal failure, omit
the sender report notice, introduce a late cleanup error or expose an alternative
marker route. Each control should explain which misleading green result it prevents.
Avoid a blanket exponential matrix and implementation-mirroring pseudo-tests.

## Initial implementation: the selection, made September 21, 2026

Research selected three, and they are specified in
[check-runner-scenarios.md](check-runner-scenarios.md):

1. **Idle task to sender report notice** — the complete correlated chain, including
   explicit controller-versus-model ownership of inbox reading and final cleanup.
2. **Busy arrivals and fixed batches** — a genuine pending native operation, an
   admitted group, a late arrival, preserved membership and the real operation's own
   completion. Keep UI display evidence separate from work completion.
3. **Mid-turn delivery** — a message reaching a session that is already working, whose
   original task still reports its own result.

Uncertain-delivery recovery, the third candidate when this was written, is deferred to
fourth: it needs fault injection at the transport, which is worth attempting only once
the fixture interface has survived three scenarios. Its claim is delivery and the
absence of replay, not the `error` outcome of a broken turn.

The five existing Go checks remain required: gofumpt, go vet, staticcheck,
golangci-lint and race/shuffle tests. They are not replaced by workflow tests.
Extract only the reviewed fixture pieces needed for the selected workflows into a
repository-owned test location. A fresh checkout must run them without any ignored
handoff directory, private snapshot or installed owner binary. Decide the location
and small orchestration interface during research; do not create them in this task.
Keep version pinning, helper dependencies, isolation, teardown and evidence collectors
reviewable. Prefer a thin command over existing tools to a new test framework.

## Measurable acceptance for the future MVP

- A fresh checkout can run all five checks and the selected two or three workflows
  using documented dependencies and isolated state, without ignored/private inputs.
- Default runs make zero real-account/provider requests, contact no owner session and
  fail explicitly when isolation or a required pinned dependency cannot be established.
- Every workflow declares required observations and its evidence tier; happy-path pass
  requires all of them. Controller reads, fake ACKs and manual observations remain
  separately identifiable in the machine summary and the matrix.
- The selected negative controls reliably fail the intended happy-path assertion or
  pass an explicitly named expected-failure assertion. No wrong guard, terminal,
  report identity, late error or skipped required phase can leave a false green result.
- Summaries meet the measured output budgets and identify a failed case's precise
  evidence path. Raw successful logs do not enter normal console output.
- Duration, source/build/native identity, exit/result categories and cleanup evidence
  are present for every applicable check; inapplicable fields say not-used explicitly.
- A rerun retains the previous attempt and does not replay uncertain accepted work.
  Paid/manual cases remain explicit opt-ins; not-run is never converted to acceptance.
- Reviewers can reproduce the chosen workflows and explain what each does not prove.
  Expansion of the matrix follows a demonstrated gap, not availability of more fixtures.
