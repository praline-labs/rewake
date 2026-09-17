# Remote continuation permissions

**[source: snapshot `44b901161`; owner live startup failure with CLI 0.154.0,
September 17, 2026]** The first full wrapper resume failed before attachment:
`Permission overrides are not supported when resuming a remote task.`
`tui/src/app/startup.rs:446–460` checks remote resume; fork uses the same guard.
`tui/src/app/config_persistence.rs:49–80` rejects explicit approval policy,
approvals reviewer, sandbox mode, permission profile, default permissions,
additional writable roots or workspace roots. Session configuration layers are
also checked for `approval_policy`, `approvals_reviewer`, `sandbox_mode`,
`default_permissions`, `permissions`, `network` and `sandbox_workspace_write`.
Thus adding `--add-dir` for a committing role breaks remote continuation.
Rewake now generates none of these for resume/fork; its only generated `-c`
setting is the briefing. Caller overrides remain unchanged, with a warning.

Saved permission settings and runtime roots are distinct. Remote resume clears
approval/sandbox/profile overrides (`tui/src/app_server_session.rs:2151–2160`);
remote fork with InheritSaved does likewise at :985–992. The server restores
saved approval settings and named profile identity (`request_processors/thread_processor.rs:4142–4171`,
fork at :4934–5007). **This does not establish that an old `--add-dir` survives.**
The TUI still supplies `Some(config.workspace_roots.clone())` for resume and fork
(:2142, :2186). Cold server resume restores saved runtime roots only when that
request field is absent (:3765–3813), so the TUI's current root list wins over
saved roots. Local config builds that list from cwd and configured roots without
rewake's omitted grant (`core/src/config/mod.rs:3435–3470`). A thread started
under write therefore still needs live verification of effective Git access
when continued; do not promise metadata access solely from its original role.

The supported APIs do not offer an additive, policy-preserving root update at
idle attachment:

- `thread/settings/update` has no runtime-roots field
  (`app-server-protocol/src/protocol/v2/thread.rs:229–290`); its handler explicitly
  supplies no workspace-roots update (`request_processors/turn_processor.rs:925–941`).
- `turn/settings/update` only changes selected settings of an active turn,
  rejects unknown fields and has no roots field (`protocol/v2/turn.rs:39–72`).
- `turn/start.runtimeWorkspaceRoots` replaces the list for that turn and later
  turns (`protocol/v2/turn.rs:205–210`), but starts or steers model work. Using it
  at startup would violate the owner's no-automatic-input rule.
- `thread/resume` on an already loaded thread checks requested roots against the
  active list instead of applying them (`request_processors/thread_processor.rs:143–151`).
- `thread/settings/update.sandboxPolicy` is a whole legacy-policy replacement,
  not an additive root grant. It converts the session to a legacy permission
  profile (`core/src/session/session.rs:469–485`); projecting a named profile into
  it would change the policy contract.

Consequently committing roles emit the requested startup note:
`resumed thread keeps its stored permissions; commits need a thread started under --write`.
The note describes the absence of a new grant, not a verification of the resumed
thread's effective roots. No permission mutation or model turn is injected.
Fake TUI argv tests cover both continuations and all three roles; live acceptance
remains with the owner.

## Continuation discovery

**[resume, snapshot 44b9011; CLI 0.154.0; September 17, 2026]**
Cold resume returns ThreadResumeResponse only to its caller, without thread/started
(`thread_processor.rs:4083–4133`). Its upsert at :4008 can broadcast
thread/status/changed via `thread_status.rs:223–251` and
`outgoing_message.rs:737–746`, without a thread subscription. Name changes also
broadcast thread/name/updated (:672), but are not guaranteed on resume. Discovery
therefore uses metadata hints plus loaded-list polling, never history.

