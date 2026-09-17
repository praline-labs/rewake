# Process, writer-lock and native resume probes

Completed September 17, 2026. This evidence extends the
[ownership investigation](thread-ownership-investigation.md); it does not repair
selection or the open P2 findings R14-1/R14-2. In owner-run observations, A and B
denote the two user conversations; matrix cell labels are independent. Private
process identities and thread UUIDs are omitted.

## Evidence grades

- **Owner/capture:** saved process, descriptor and whitelisted thread metadata,
  plus the owner's explicit confirmation of the visible dialogue.
- **Stat:** later file metadata only, without an app-server connection or content
  reads. It is distinct from the earlier API-assisted snapshots.
- **Control:** isolated empty files on the same filesystem, without a native
  harness, used to test inode/descriptor reuse and timestamp behavior.
- **Binary/source:** installed executable fingerprint and source literals,
  compared with current and historical source. This does not prove build equality.
- **Native build verified:** four controlled resume cases on the fingerprinted
  executable in a disposable fixture with validated hard isolation.
- **Inference/open:** attribution of the earlier owner-run event to a runtime
  branch or exact syscall/process remains unverified. No live syscall trace ran.

The saved evidence files are named below for provenance; they and the temporary
scripts are not part of the repository. The original process/stat investigation
read no transcript, auth or configuration contents. Later permission covered only
the two test conversations' service metadata, as described below; it does not
relax the production boundary against transcript reading. Native UI actions
belonged to the owner; the later native matrix used disposable empty fixtures.

## Observed transitions

**Owner/capture, /new:** the corrected `before-fd.json` / `after-fd.json` pair
kept wrapper, TUI and server PIDs stable, and TUI descriptor targets unchanged.
The server acquired another writer-lock descriptor while retaining the prior
lock; another service child appeared. The earlier `before-host.json` collector
only traversed the main OS thread's children and missed descendants. It is not
full FD evidence; the corrected traversal covers children of every OS thread.

**Owner/capture, first resume:** `before-resume.json` / `after-resume.json` is a
cold test. A had already unloaded before the baseline, and its lock reappeared
on resume. It cannot establish continuously loaded or warm-switch behavior.

**Owner/capture, later resume:** both A and B were loaded/idle at the endpoints
of `before-warm.json` / `after-warm.json`, about 10.29 seconds apart. The owner
confirmed visible B before and A after. B was already displayed before the
experiment; the initial /resume B may have been a no-op. Its unchanged timestamp
is **not** evidence that a proven preceding switch to B failed to touch the lock.

The same evidence nevertheless rejects the point-in-time latest-mtime rule:

| Capture | Owner-confirmed visible dialogue | Newest writer-lock mtime |
|---|---|---|
| before-warm | B | A: wrong visible target |
| after-warm | A | A: matches this endpoint |

This conclusion needs no earlier A-to-B transition. It concerns the confirmed
visible dialogue, not an independently queried internal primary_thread_id.
A later positive match cannot erase the earlier counterexample.

## Same inode and FD did not mean the same file lifetime

**Stat:** A retained the same endpoint inode/FD numbers while mtime/ctime advanced;
B's lock and both rollout-stat snapshots stayed unchanged. A later GNU stat
follow-up found the current A object's birth time inside the capture interval,
exactly matching its new mtime/ctime. Its other metadata still matched after-warm.
B's birth time remained at its earlier creation. This supports file replacement
with a reused inode, not an in-place TUI-focus timestamp refresh. A service child
also changed, but parent/command metadata alone does not prove its thread role.

**Control:** close/unlink/create on the same filesystem immediately reused both
the inode number and FD number while advancing timestamps. Conversely, opening
with create but no truncation and then failing a competing flock left timestamps
unchanged. O_TRUNC before a failed flock and explicit utime changed mtime/ctime
without changing a held inode. Those controls show non-unique effects; they are
not evidence that either of the latter operations caused the native observation.

The owner-run captures are endpoint samples, not a syscall trace. Process/FD collection
precedes the metadata RPCs, and file stats follow them. The collector sends only
initialize, loaded-list and thread/read(includeTurns=false), and intentionally
performs no lock write/touch. Its possible native observer effects have not been
isolated by a live control. Exact initiating PID, syscall sequence and the
runtime-replacement branch remain untraced; loaded endpoints do not prove a
continuous file, descriptor or backend runtime between them.

## Build provenance and a candidate explanation

**Binary:** the inspected CLI 0.154.0 executable is a stripped static PIE of
262858016 bytes, SHA-256:

```text
3188814c35471432d4123203e0eb38e5bddc60226e3d7ddf0e59e649ea140022
```

Embedded `thread-store/src/local/writer_lock.rs` paths, warning locations and the
old directory-creation error literal correspond to the source layout before
`73a1148c9` (parent `20f109ead`). Reference `44b901161` has moved/refactored this
module into `rollout/src/writer_lock.rs`, whose source-path literal was not found
in the binary. These are build-family clues, not an exact source revision. The
earlier owner-run live server's executable fingerprint was not independently
established. The later isolated matrix verified its own executable hash.
Version `0.154.0` alone does not prove equivalence to the reference checkout.

**Source:** current `rollout/src/writer_lock.rs:43–87,174–194` and historical
`20f109ead:thread-store/src/local/writer_lock.rs:40–88,169–192` both open without
truncation, try an advisory lock, then remove the file when its guard drops.
Neither inspected implementation has a TUI-focus timestamp refresh. Paths here
are relative to the upstream Rust workspace and tied to those revisions.

Current `app-server/src/request_processors/thread_processor.rs:4248–4286` and
the historical file around `4325–4360` can shut down an idle loaded runtime with
no subscribers when resume overrides count as a mismatch, then recreate it.
Source at `thread_processor.rs:103–222` treats non-null config, permissions,
baseInstructions or developerInstructions as a mismatch by presence alone, even
when values are equal; omitted/null config differs from an empty object. The
branch also requires direct input, idle status, a non-running agent and completed
shutdown. The incoming resume client attaches after that decision.

Ordinary TUI resume uses the normal settings constructor, which supplies config
(`tui/src/app/session_lifecycle.rs:1203–1253`;
`tui/src/app_server_session.rs:2095–2161`). Its config helper always includes
web_search (`:1833–1900`); remote cleanup does not remove it (`:1903–1920`).
The presence rule therefore does not require a user-visible setting change.
These are source findings at `44b901161`, cross-checked against `20f109ead`;
they do not establish the request used in the earlier live event.

## Native resume matrix, September 17, 2026

**Native build verified:** CLI 0.154.0 with the SHA-256 above reproduced all four
predicted outcomes, one execution per cell. The hash was checked inside the actual
fixture before launching the existing executable. Each target was loaded/idle,
legacy and empty; the creator unsubscribed before the tested resume. Where
specified, a separate fixture client retained exactly one subscription.

| Cell | Resume config | Retained subscriber | Runtime/lock outcome | Lock birth time | Endpoint inode |
|---|---|---|---|---|---|
| A | omitted | none | reused | unchanged | same |
| B | `{}` | none | recreated | new | same, reused |
| C | `{}` | one | reused | unchanged | same |
| D | omitted | one | reused | unchanged | same |

B's birth/mtime/ctime advanced together; native status passed through notLoaded
then idle, with the persistent thread identity unchanged. The other cells kept
all three timestamps. Retaining a diagnostic subscriber can therefore change
native behavior; it is not a neutral observation of this branch.

Before native execution, checks inside the actual bwrap fixture confirmed distinct
network/mount/PID namespaces, no external interface/default route, absent owner
homes/project/state/managed config, no credentials or proxy variables, read-only
binary/runtime mounts and fresh private state. Control RPCs were allowlisted;
51 requests included zero model-generating calls, and monitoring observed no
turn/item/active-thread activity. No history was read. External model endpoints
were unreachable; internal attempted HTTP was not traced. Disposable state was
removed after terminating only fixture children. A preceding driver parser error
stopped before any cell; a fresh isolated fixture ran the successful matrix.

This upgrades the conditional recreation path from source hypothesis to verified
behavior of this build. It does **not** attribute the earlier owner-run A/B event:
its resume config, subscriber state at admission and running executable fingerprint
were not captured. Nor does an empty legacy fixture establish every history mode.
Background acquisition and cached selection still prevent equating writer lifecycle
with TUI focus. Evidence: `native-matrix-report.md`, `result.json` and
`validation-summary.json` in the temporary research artifacts.

## Narrow test-service metadata observation

The owner separately authorized examination of service metadata only for test A/B.
Derived service/context comparisons found no new resume events and equal last-turn
settings. Recorded last-turn settings establish neither current runtime settings
nor the current resume request, and cannot disprove a presence-based mismatch.
The absence of expected checkpoints limits attribution to the inspected source
path. The native matrix did not read history and leaves that discrepancy unresolved.
No permission to inspect other conversations or production transcript contents
follows from this exception.

## Supported conclusion and remaining choices

Stat, FD and lock metadata are lifecycle hints, not a reliable selected-dialogue
contract. A change may invalidate a cached assumption; neither a change nor its
absence authorizes delivery to a particular conversation or closes the
selection/read-to-delivery race. Latest-lock-mtime routing is not justified.

Only rewake may change; native TUI/server modifications remain outside the owner's
scope. Existing-hook registration, explicit binding and the restricted
[intent gateway](thread-ownership-investigation.md#practical-rewake-only-intent-gateway-research)
remain unselected options, not implemented features. R14-1 and R14-2 remain open
P2 findings independently of this probe. No ownership repair or new live transport acceptance is claimed.

An optional 30-second owner-only inotify/birth-time watch and a narrowly filtered,
metadata-only syscall-trace recipe were prepared. Neither ran against the live
probe. They are not automatically queued work; any further experiment remains a
separate owner-controlled choice. It must separate genuine switches, no-ops and
collector-only activity without reading content or restarting sessions.
Coordination waits for finished reports rather than inspecting intermediate work.
