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
fork at :4934–5007). The source reading alone did not establish that an old
`--add-dir` survives; a live probe since has (below).
The TUI still supplies `Some(config.workspace_roots.clone())` for resume and fork
(:2142, :2186). Cold server resume restores saved runtime roots only when that
request field is absent (:3765–3813), so the TUI's current root list wins over
saved roots. Local config builds that list from cwd and configured roots without
rewake's omitted grant (`core/src/config/mod.rs:3435–3470`). A thread started
under write therefore still needs live verification of effective Git access
when continued; do not promise metadata access solely from its original role.

**[verified live; 0.155.1 and 0.157.1; September 26, 2026]** A cold `thread/resume`
that omits the roots field restores the roots the thread last saved, a root added by
`turn/start` on an established history included; a cold `thread/fork` drops it
([research-codex.md](research-codex.md#runtime-workspace-roots)). A resume through the
terminal supplies its own list, per the source above, so what survives depends on who
resumes. Rewake's directory grant counts on neither: it is given at delivery and taken
back after the report ([grants.md](grants.md#how-long-a-grant-lives)).

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

## Grant with delivered work

**Owner correction, September 19, 2026:** [explicit --grant-git intent](git-grants.md)
replaces the September 17 automatic per-task behavior. Attachment and role alone
add no roots; eligible delivered tasks request them only when main chose the flag.

Only an explicit --grant-git task/question to an eligible recipient calls `thread/read` with
`includeTurns=false`. Loaded metadata includes current `environments`, each with
`environmentId`, `cwd` and `runtimeWorkspaceRoots`
(`app-server-protocol/src/protocol/v2/thread_data.rs:204–212`,
`protocol/v2/environment.rs:13–17`; the loaded metadata path is
`request_processors/thread_processor.rs:2799–2855,6201–6218`). No transcript or
turn history is requested. The permission read has a 500 ms budget within the
existing delivery deadline; a failed read must not consume that entire deadline.

The adapter supports one local environment. Missing, null, malformed, multiple
or remote selections cannot safely supply a complete replacement list: delivery
continues without the field and records one line explaining the skipped grant.
It resolves the thread environment's cwd, not the wrapper's launch cwd, through
the existing metadata resolver. It preserves every returned root in its original
order and appends only missing gitdir/commondir paths. A broad ancestor does not
prove access to protected Git metadata; the exact metadata roots are required.
Already present metadata needs no field. No-flag tasks, general, notify and completion reports
never trigger this feature. No sandbox policy, profile, approval, cwd or other
permission setting is sent. Effective access remains subject to the selected
policy; adding a root does not turn a read-only profile into a writable one.

**Steering, source `44b901161`:** `turn/start` builds the runtime-root update
before `start_or_steer_turn` (`request_processors/turn_processor.rs:586–651`).
Core accepts persistent settings on both Started and Steered
(`core/src/session/turn_input.rs:8–9,95–113,242–281`). On Started they apply before
creating the new context (:119–165). On Steered they update persistent settings
only: the active context retains its permissions (:190–198). Rewake therefore
sends the field on steer too and notes that the grant is for subsequent turns.
It does not retry rejected requests or initiate another turn to acquire access.

**Manual TUI turns, same snapshot:** `tui/src/app/thread_routing.rs:864–873`
passes the TUI's local workspace roots to `app_server_session.rs:1339–1350`,
which always sends `runtimeWorkspaceRoots`. `thread/settings/updated` lacks that
field (`protocol/v2/thread.rs:296–318`); TUI sync updates policy and retargets cwd
but does not replace its roots (`tui/src/app/thread_settings.rs:247–262`,
`chatwidget/settings.rs:454–487,507–524`). Consequently a human-started turn can
replace the adapter's grant with the TUI's older list. Every explicitly granted task reads
a new snapshot and restores missing metadata if needed. The historical startup wording
summarizes the lack of an unconditional launch grant, not a promise that the TUI
preserves server roots.

The API has no atomic append or revision precondition. The adapter preserves its
fresh snapshot. The metadata read and turn/start are now ordered under the same
gateway reservation against native TUI
selection/settings requests. A lost binding refuses delivery rather than applying
a prior thread's roots to its replacement. This is not a global server transaction
against clients that bypass the wrapper gateway.
Fake-server tests cover roles, exact unions, worktrees, unreadable snapshots,
steer, repeated tasks, manual replacement and thread changes. Mutations that
narrow roots, grant general access or add other directories must fail these tests.
Live permission and transport acceptance remains with the owner.

## Continuation discovery

**[resume, snapshot 44b9011; CLI 0.154.0; September 17, 2026]**
Cold resume returns ThreadResumeResponse only to its caller, without thread/started
(`thread_processor.rs:4083–4133`). Its upsert at :4008 can broadcast
thread/status/changed via `thread_status.rs:223–251` and
`outgoing_message.rs:737–746`, without a thread subscription. Name changes also
broadcast thread/name/updated (:672), but are not guaranteed on resume. The former observer used metadata hints plus loaded-list polling. The integrated
gateway instead sees the caller's request and matching reply, so no discovery or
observer resume is required; see [gateway selection](archive-1.x/gateway.md).

