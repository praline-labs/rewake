# agent-deck: worktrees, delivery and testing

[Back to prior art](../prior-art.md). The first half of
[agent-deck-ideas.md](agent-deck-ideas.md), which has the opening (what agent-deck is, the
commit read, the verdict, how to read **F:** and **I:**), the terminal areas, what not to
take and the open questions of all five areas. The design these ideas serve is
[stage4-space.md](stage4-space.md) and its [build steps](stage4-space-steps.md). Facts are
from agent-deck at 8d0f1fe1 (v1.16.28), read as code on October 13, 2026, nothing run.

## 1. Worktrees under the keeper (S4-7, S4-9)

Ownership is reversed. **F:** agent-deck makes the checkout itself with `git worktree add`
and removes it; rewake's keeper lets Claude Code create the checkout, lock it with the
reason `claude agent agent-<id> (pid <pid> start <ticks>)`, and remove an unchanged one at
the end ([stage4-space.md](stage4-space.md), section 7). agent-deck never git-locks. Most
of its creation machinery therefore does not apply.

**Does not apply (Claude Code decides it):** location and branch naming (git.go:377-431,
458-549), `.worktreeinclude` copying (worktreeinclude.go), sparse-checkout inheritance
(sparse.go), bare layouts, `checkout_git_config`, setup and destruction scripts with their
consent gate (scriptconsent.go; the lesson reduces to: rewake adds no repo-script hook),
merge-back and finish, `RemoveWorktree` with its `RemoveAll` fallback. **I:** that fallback
(`worktree remove --force` then `os.RemoveAll`) is how issue #1200 deleted a user's whole
repository (commit 77846aea): the thing to keep out. Rewake removes nothing and offers no
`space clean`; if one comes, dry run by default, candidates only from its own records, the
facts below, re-validation at the delete boundary, no force.

### Ideas worth taking

**Facts triple for a kept checkout; unknown is never clean.** **F:** agent-deck's cleanup
treats a worktree as an orphan only if it has no owner and `rev-list --count HEAD --not
--remotes` is 0 (nothing unpushed), `status --porcelain --untracked-files=normal` is empty,
and no process has its cwd inside (scan `/proc/*/cwd`, own first, `lsof` fallback); any
inspection error fails closed; `--force` can only authorize an already classified orphan
(worktree_cleanup_safety.go:51-146, 219-262; commit ceb9792f, #2023). **I:** `End.Checkout`
in the record gets the same facts as observed values: exists or gone; locked (reason, owner
alive or dead); `unpushed`; `dirty`; live process inside; each can be `unknown`, shown as
unknown. The reminder lists them, says which make removal unsafe, and names the commands.

**Stale lock by pid plus start ticks.** **I:** agent-deck has no lock model to copy. The
lock reason carries pid and start ticks; compare both with `/proc/<pid>/stat` field 22.
A pid alone is reused. Git's own `worktree prune` skips locked worktrees, so a stale lock
never heals: the reminder names `git worktree unlock <path>`. Their procStart parser counts
fields after the last `)` (claudesocket.go:140-151); do the same.

**Linked-worktree predicate before any destructive git command.** **F:** `IsLinkedWorktree`:
the absolute git dir's parent is named `worktrees`; for an orphaned entry, the `.git` is a
file pointing under `worktrees/`; any error or ambiguity gives false (git.go:731-763).
Rewake's own destructive step is the base move's `git reset --hard`. **I:** besides the
proof by agent id and pid through the lock reason, assert the path is a linked worktree and
not main's toplevel immediately before the reset, and never run it in main's own tree.

**Base as a SHA from main's HEAD.** **F:** agent-deck roots a new branch at a freshly
fetched `origin/<default>`; regression #973 (b3d32e39) found a branch inheriting a stale
local HEAD, a worker's PR with 414 files of unrelated diff (git.go:512, 969-983). **I:**
rewake deliberately uses main's HEAD (main's work in progress). Resolve it to a SHA, detached
HEAD allowed, fetch nothing, record the commit actually reset to so closing can tell
"unchanged" from "moved", and test with a stale `origin/main`, main's branch ahead, and a
detached HEAD (the #973 fixture, inverted).

**Do not start helpers with their cwd inside a checkout that can vanish.** **F:** #1713
(54bf098d): a tmux server inherited a cwd inside a worktree later removed; agent-deck now
spawns from `/` (internal/tmux/workdir_guard.go:66). **I:** the host and the wrapper keep
their cwd outside the checkout; set the worker's directory on the child only, and verify the
child's cwd exists after start. Before releasing the keeper, scan `/proc/*/cwd` (symlinks
resolved, own included); a hit blocks the release with the pid.

**One record per checkout path.** **F:** bug 08128e00 (#1449): finishing one of several
sessions stranded the others sharing a worktree (`OtherSessionsShareWorktree`, paths compared
after `EvalSymlinks`). **I:** the open refuses when another open record names the same
canonical path.

**Git is the truth about existence, the record about who it was for.** **I:** agent-deck
trusts its row and falls back to git for safety; the reminder does the reverse when they
disagree. `rewake space list` lists from records, since their cleanup scans one repository
(worktree_cmd.go:243-262) and misses the others.

**Re-read under the lease.** **F:** their cleanup prompt can wait forever, so at the delete
boundary it takes an exclusive flock, reloads ownership and re-inspects (worktree_cmd.go
:206-262). **I:** rewake's lease serves; re-read state under it before writing a verdict
because the list can be minutes old.

### Traps their history found

- #1200 / 77846aea: deletion predicate from path convention removed the main tree. Take it
  from git, fail safe.
- #1396 / c53fa98b: bookkeeping after irreversible steps failed and orphaned the row. Rewake
  SP2 (closed from observation) is stronger.
- #1398 / 1b7be2f5: an include-walk entered sibling worktrees and nested a skeleton in each
  new one, gigabytes. Stop at any directory with its own `.git`.
- #2102 / eafc6bd1: Claude Code's folder-trust dialog killed a launch in a new checkout
  path; they seed trust in the harness's own config. **I:** do not copy; check live whether a
  worker in `.claude/worktrees/agent-<id>` meets the dialog.
- #1029 / 6a1645eb, 162c80b9: copying work in progress or gitignored files into a new
  checkout is dangerous by default; opt-in there.

### Checklist

- S4-7: linked-worktree assertion; explicit fail-closed "holds no work" proof (commits
  beyond base, porcelain with untracked); SHA from main's HEAD; lock reason compared by pid
  and start ticks; lock reason string and the reset commit kept in the record; cwd outside;
  uniqueness by canonical path; live checks (open questions 3 and 4); never reset in main's
  tree.
- S4-9: the triple written to `End.Checkout`; reminder text; clearing by observation (seen
  gone from `git worktree list` and the branch gone or merged; a prune counts as gone); the
  dead-main case with a stale lock; pre-release cwd scan; no rewake-side removal.
- Tests carried over: a reused main tree is never listed removable; an orphan with an
  unpushed commit, a dirty untracked file, a live pid, and an inspection error are each
  protected; classification happens before any force-like option.

## 2. Delivery to Claude Code's messaging socket (S4-5, S4-6)

**F:** agent-deck sends to a Claude Code session's inbound socket (Claude Code 2.1.259 and
later, by their reading of the binary) only as an opt-in transport, falling back to tmux
typing; the code is internal/send/claudesocket.go and cmd/agent-deck/session_send_transport.go.
Their wire facts are agent-deck's claims from a plan document absent from the clone
(claudesocket.go cites sections of it); rewake's own live verification
([research-launch.md](../research-launch.md), [delivery-adapters.md](../delivery-adapters.md))
disagrees on one point below.

### Ideas worth taking

**Commit rule: committed or unknown, never a second transport.** **F:** two error kinds.
`Unavailable{Reason}` is a condition observable strictly before any byte is written and
allows a fallback; `CommittedError` is a failed `conn.Write`, including a write deadline,
which "may or may not have reached the inbox" and is never retried on another transport
(claudesocket.go:24-78, 395-409; tests claudesocket_test.go:223, :251). The dial (5 s) and
the size check sit on the first side; the 5 s write deadline covers a stuck receiver. The
selector declines before any attempt for targets other than Claude Code, a bare message starting with
`/` (slash commands arrive as literal text), and an unknown conversation id. **I:** this is
the shape of rewake's E8 treatment (an unproven outcome is stated and stops, never
papered over with a second route). A short write leaves an unterminated line the receiver
cannot parse, so "probably not delivered", but that is not provable: record unknown, do not
fall back.

**Vet the target before the first byte.** **F:** order: session id non-empty, protocol
number exactly 1 (not `>=`, a bump may change the frame), socket path, `kill(pid,0)`,
process start equal to field 22 of `/proc/<pid>/stat` (fields counted after the last `)`),
socket is a socket, owned by our uid, key file when present with no group or other bits
(claudesocket.go:205-315). Several per-pid records can carry one conversation id (resume,
fork, another account), so only records whose pid is in the target's process tree count;
none is `pid_not_in_pane_tree`, several `ambiguous_record`: refuse to guess (434-455).
**I:** rewake fixes the path itself with `--messaging-socket-path` and owns the listener,
so identity is by construction; the mod reading `CLAUDE_CODE_MESSAGING_SOCKET` from its
environment ([design-claude.md](design-claude.md)) inherits the "child of a child" hazard, which is the one
place their vetting applies.

**Where rewake is ahead: receipts.** **F:** the server never writes back on any path; a nil
error means "bytes written to a live, identity-verified endpoint that had an inbox bound,
nothing stronger"; `--wait` probes busy status just before the write and admits the residual
race: "only an in-band receipt keyed to the message id could close it", and the harness's
inbox provides none (claudesocket.go:346-355; session_send_transport.go:~340-380). **F, rewake:**
the receiver opens a new connection to the frame's `from` address and sends
`peer_message_status` (`held`, `delivered`, `expired`, `denied`, `dropped`), verified against
the writer's pid, uid and start (research-launch.md). That is the one point on which the two
disagree, and it is a real step ahead: a byte-accepted line can still be held for approval,
dropped as a duplicate within 30 s or by a queue cap, or refused, and agent-deck sees none of
it while its documents call it `queued_socket`. Rewake's `priority` also has `now`, `next`
and `later` where theirs always sends `next`.

**`session_id` binding.** **F:** a frame whose `session_id` does not match the live
conversation is silently dropped and looks like success (claudesocket.go:206-210). Rewake's
1.x frame carries none, and `/clear` gives the session a new id (research.md). **I:** check
live what the receiver does with no field, with a stale one, and across `/clear`; if rewake
ever adds the field, a stale value is a silent drop (open question 6).

**Serialize senders per target.** **F:** `AcquireSendLock`, one sender at a time per target;
if the wait expires the state is `target_busy` and nothing was typed. **I:** a lock timeout
is "not sent", with nothing written (T9: every wait has a bound).

**Frame bound before dialing.** **F:** a line over 1 048 576 characters destroys the
receiver's connection, so the sender refuses first (`ReasonTooLarge`, 384-387). **I:** the
notice is bounded by T5; assert no partial write.

### The ledger's hard cases as a test checklist

**F:** the comms ledger (internal/comms, docs/comms.md, comms-matrix.md) is default off,
phased, dual-written beside the old inbox. It claims "the ledger and the inbox each answer the
sender exactly once" for tagged sends only, keeps an evidence ladder (`durable`, `attempted`,
`transport_accepted`, `context_observed`, `application_acked`, "a timeout is a reconciliation
trigger, never a promotion"), and runs a matrix whose XFAIL status passes only when the
measured value equals an enumerated baseline: a missing binary, an ssh failure or invalid
JSON is never XFAIL (tests/comms_matrix/runner.py:29-36). **I:** rewake's rules are
stricter (an effect is proven done, proven not done, or unknown, and unknown stops the
mailbox: effects.md, D1-D8), so copy the test discipline, not the ledger. The mapping to
rewake's rules below was made by rule titles only, not from their full text; verify each
before saying "covered".

| Hard case (agent-deck) | Test for rewake (**I:**, rule by title) |
|---|---|
| One turn event replayed 100 times is one record; flapping replayed 5 times is one record (`recentKeys`, a window of 4096; ledger.go:465-500) | Feed `turn.complete` N times, before and after a restart, and the same key with different content: one report, the second a recorded conflict, not an overwrite (E7) |
| Restart mid-burst re-emits a committed turn (failures.py:61-90) | Journal recovery does not republish a swept report (E1/E8; the effects journal test) |
| Crash points around commit: spool crash after commit before unlink, short write, fsync failure (comms.md "Recovery at every boundary") | Iterate every cut point of a send through `rewakefault`, asserting at most one delivery and a stated unknown otherwise; generate the cut list from the rule file |
| Parent removed, completion becomes `_unowned`, must drain exactly once (failures.py:124-161) | The addressee's room is gone: the report stays visible and drains once |
| 65 turns against a 64-turn cap, the last dropped and dead-lettered (failures.py:252-267) | Inbox cap exceeded: the next one is refused loudly at send, never dropped on read (T5, E5) |
| Log unlinked while open, daemon keeps writing to a deleted inode (failures.py:213-250) | Delete the mailbox directory under a live writer: `ErrNoLease` path, not a silent write |
| Conductor's own turn must not address itself (scenarios.py:84-107) | Main's `turn.complete` never creates a report to itself |
| Approval dialog open: nothing may press Enter into it (failures.py:336) | The inbound gate `held` path; covered through receipts, check the order inversion `expired` before `held` |
| State file lost or restored from an older copy is an explicit gap, never a silent skip (comms.md) | Any cursor-like state: restore an older copy and expect a stated gap |
| The 30-minute queue expiry is not tested and the document says so | Each bound a test does not prove is a `Gap:` line, as `docs/rules_test.go` already requires |
| Soak report prints `null/UNKNOWN`, exit code 2 is HOLD, never pass (comms-lab-soak.md) | Adopt HOLD as an exit code in `checksummary` for live-only tiers |
| OpenCode consumer never connected in production, a working fake server proved nothing | "A fixture proves the fixture": a scenario needs a control that fails |

### Edge cases to check against existing tests first

**I:** from the above; a number is the case, the rule is by title.

1. A socket closing inside the receipt window: no receipt plus a reset must not read as
   `delivered`; "delivered from silence" stays revisable until the late-word window ends
   (C3: silence proves nothing).
2. A receiver alive but not draining: the write deadline becomes `failed`, the message is
   still owed, no second send on the same id without a decision (D6).
3. A short write: recorded unknown, not failed, no other transport.
4. A frame over 1 MiB: refused before dialing.
5. Dial refused with the harness alive is pending, dead is failed; add the race of a socket
   replaced between probe and dial.
6. Identical notice within 30 s and a queue of 51: distinct `<task-id>` per notice, a resend
   after `dropped` keeps its ancestry.
7. A hold released after the sender exited: the wrapper listens, order inversions and two
   receipts in one burst.
8. The gate holding what arrives in the first seconds after the socket appears: the early
   notice is kept back and the delay is not a retry.
9. A conversation change between enqueue and delivery: the report goes to the new
   conversation once; the old turn's is not published for the new one.
10. A child of the session inherits the socket variable: a command started from a tool call
    must not reach the grandparent (SP3/SP9 hold the hosted part; add the mod path).
11. Two senders to one target: ordering, and a lock timeout is "not sent".
12. A target busy at send: the interim-wake rule (O1) keeps the task open when the message
    lands inside a running turn.
13. A notice that starts with `/` arrives literally; never let one begin with `/`.
14. The addressee gone and the cap exceeded, per the table.

## 3. Testing

### Screen goldens without tmux

**F:** tools/visualcheck drives the real binary in a private tmux server per size with a
seeded sandbox, a fixture harness script, 21 key steps and 15 named frames at 80x24,
120x40 and 200x50; goldens are plain text from `capture-pane -p` under
`testdata/golden/<step>_<WxH>.golden`; the statuses are PASS, DIFF and MISSING, and MISSING
is a difference, never a silent pass (tools/visualcheck/README.md; golden.go:26-71).
**F, what is worth copying:**

- Poll on screen predicates with a deadline and cancellation, not sleeps (poll.go:11-35).
- A fixed-length random sandbox root (`/tmp/vc-<10 hex>`), on purpose: the path is drawn in
  dialogs and a different length shifts a border column no scrub can undo (sandbox.go:56-78).
- A golden mismatch is not retried and is kept as DIFF so later steps still run; retries are
  for navigation only (capture.go:157-200).
- Regeneration only through an explicit flag and test name; the ordinary test skips when it
  is set (main_test.go:41, 140-160). The reviewer gate is convention, not code.
- A `clippedDialogBorder` check at regeneration: every top-left corner needs its
  bottom-left, so a clipped frame cannot become a golden (golden.go:57-71).
- An ordered scrubber (scrub.go:19-65): UUIDs, short hex ids, versions, clock times, dates,
  ages, shell prompts, socket names, the temp path, VS16 padding. **I:** its rules have no
  unit tests and some are broad (any `\d+m` is an age), which can hide a number regression.
- A redraw probe after detach: `immediate`, `settled` and `repainted` panes, failing when a
  title appears more than once (capture.go:99-165).

**I:** the bulk of its 2 700 lines compensates for tmux's live buffer and Bubble Tea's
differential redraw (debounce, hard kick, arithmetic navigation). With x/vt the grid is a
deterministic function of the byte stream and most of that disappears; what remains is "when
is the frame done". The design for `internal/screentest`:

1. `Start(t, argv, env, cols, rows)` on a PTY pair, sized by `Setsize` and by the emulator's
   `Resize`; a goroutine feeds output to the emulator and copies its replies back to the PTY
   (otherwise programs that query DA1 or CPR wait).
2. Snapshots from the grid: plain rows with trailing blanks trimmed and placeholder cells
   skipped (no VS16 padding scrub is needed; test the width table as its own unit), and an
   optional attribute sidecar (`row,col-range attrs`) only for screens where an attribute is
   the point, such as the selected row or dim ghost text.
3. Waiting: `WaitFor(ctx, pred)` over a consistent grid copy, then `Settle(quiet 50 ms,
   max 2 s)` meaning no PTY bytes for the quiet period; every wait has a deadline and prints
   the current grid on timeout (T9).
4. Sizes 80x24, 120x40, 200x50 (a 60x20 as well); goldens `testdata/screens/<view>_<WxH>.golden`;
   a missing one fails; update only by an explicit test name and `-args -screens.update`.
5. Scrubbers as an ordered slice with a positive and a negative sample per rule; anchor
   durations to a label (`ago`, `took`) instead of scrubbing bare numbers; a fixed-length
   temp root by construction (`/tmp/rw-<10 hex>`); scrub ids, build ids, pids, socket
   paths, versions, host and user.
6. Determinism: the fixture harness, `TERM=xterm-256color`, `COLORTERM=truecolor`, a fixed
   `LANG` and `TZ`, the clock injected by environment where a view prints times.
7. Views first: the space's view (sessions, lights, owed reports, an open notice), `rewake
   attach` (relayed frame, resize, detach), the host wrapper's first draw, refusal screens,
   the gate's hold screens, each at three sizes. After an attach ends, assert from the
   emulator that alt screen is off, the cursor visible, mouse and bracketed paste off and the
   kitty stack empty: the check their `AttachWithOptions` exists for.
8. Redraw probe: a key and its opposite, `immediate` after a cursor-row predicate, `settled`
   after `Settle`, `repainted` after a size nudge; assert `settled == repainted` cell for
   cell, no row duplicated in the list column, and the cursor on the expected row. A
   difference is a differential-redraw bug in rewake's own renderer.
9. An executable reviewer gate: an update writes the goldens and a `PENDING_REVIEW` list; a
   normal run fails while it exists; the reviewer's step removes it (the review chain of
   `AGENTS.md`). Every golden has a row naming the rule or document that justifies it.
10. Failure output in `.scratch/`: expected, got and a diff, so the orchestrator reviews
    without running.
11. A canary for the emulator: a known byte stream (dim, VS16, CJK, alt screen, scroll
    region, OSC title, DECSTBM) against a tiny golden, so a library update that changes width
    or wrap fails one unit and not every screen. Live harness frames never enter this tier;
    they belong to the dated research records, with the version in the name.

Gate: the tier joins the five checks only if a frame costs under a second; otherwise it sits
behind a switch like `REWAKE_WORKFLOW`, and `docs/testing.md` and the documentation map are
updated with it.

### A state-detection corpus, for a harness without a mod

**F:** two layers (docs/status-detection.md): the harness's own signals win while fresh
(Claude Code hooks, a Codex notify hook with 20 s and 5 s freshness, pi's turn hooks),
under a frame classifier over the visible pane: title spinner, busy cue within the last 12
to 25 lines, a 6 s spinner grace, a one-sample debounce on a flip away from busy, menu
chrome counted only with a selected numbered option (a reply may end in "Do you want"),
and a `PermissionRequest` hook holding `waiting` for 5 s because the dialog is drawn just
after it. The corpus is `internal/tmux/testdata/status_corpus` (118 real frames, letters
and digits scrubbed to `x`, glyph structure kept) with `labels.tsv` (`file tool expected
note`); `TestStatusCorpus_NoFalseRunning` is the headline invariant (no waiting or error
frame classifies as active) and a note "known blind spot" reports without failing
(pane_corpus_test.go:15-130). **F:** the ghost-suggestion bug (#2651, 8d0f1fe1): a
suggestion in an empty composer is drawn dim (or first cell inverse, rest dim); only the
attribute tells it from a typed draft, and a guard that took it for a draft typed it back
as text (internal/send/suggestion.go:9-40).

**I:** rewake's cells carry attributes directly, so no `capture-pane -e` parsing. For a
harness with neither mod nor extension, a documented fallback ladder: harness signals, then
the title (OSC 0/2 through the title callback), then the composer plus a busy cue near the
bottom, then dialog chrome, then hysteresis. Vocabulary `busy | idle | dialog | unknown`,
with `unknown` explicit. State from the screen never settles a report; the turn boundary
does (C2: connected is not working, C3). Build it only when a harness needs it, from a
corpus of vt-grid captures with an attribute sidecar and the harness version in each note.

### The delivery tests, as a set

**F:** the delivery tests of internal/send (the report counted 19) pin one refusal or outcome
each; the named ones: the server writing nothing back is still success
(claudesocket_test.go:212), a write failure after the dial is a committed error (:223), a
write deadline is a committed error (:251). **I:** copy the shape, one test per refusal
reason (stale record, start mismatch, foreign owner, loose key file mode, torn key pair,
ambiguous tree, wrong protocol number, as the vetting order lists them), each asserting the
reason and that no byte was written.

### Checklist

- S4-5/S4-6: the commit rule for the mod endpoint and the receipt retry; the edge-case list
  above checked against existing tests; the cut-point iteration over `rewakefault`.
- S4-4/S4-8: `internal/screentest`, the canary, goldens at three sizes, the probes; the
  PENDING_REVIEW gate; `docs/testing.md` and the map updated with the tier.
- Later, if needed: the state-detection fallback and its corpus.
