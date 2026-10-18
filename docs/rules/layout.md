# Rules: the layout

What each layer of the tree may import and where a harness may be named. The
import rule is held by `depguard` in [.golangci.yml](../../.golangci.yml); the
layout and its reasons are in [design.md](../v2/design.md#the-layout), and each
package's place in [code.md](../code.md).

## The import rule

A layer always imports itself, and an adapter its own parts. Test imports obey
these same boundaries:

| Layer | May import |
|---|---|
| `infra/*` | the standard library and other `infra` packages, acyclic |
| `core/*` | `infra` |
| `adapter` | `core`, `infra` — the values the interfaces carry |
| `adapter/<name>` | `adapter`, `core`, `infra`; never another adapter |
| `adapter/catalog` | `adapter` and every `adapter/<name>` |
| `host` | `adapter` (the interfaces), `core`, `infra`; never an adapter |
| `cli` | `host`, `adapter`, `core`, `infra`; never `adapter/catalog` or an adapter |
| `cmd/rewake` | `cli`, `adapter/catalog` |

Every layer may import the standard library. Every layer but `infra` may also
import another module's package, each dependency its own decision (`AGENTS.md`,
"Code"). `github.com/charmbracelet/` stays confined to
`internal/host/terminal`, the package it was taken for
([stage4-space.md](../v2/stage4-space.md#the-terminal-library)), restored there on
October 18, 2026 for the workers' panel, and kept for the host mode that replaced it
([host-mode.md](../v2/host-mode.md)).
The confinement includes tests and excludes child packages of `terminal`.

The lint run checks the files selected by its build tags, including tests. The
configured run includes `rewaketest`; a different tag set is checked with
`golangci-lint run --build-tags <tags> ./...`. Unlike the former layout test,
lint does not enumerate eight builds or reject a directory merely for being
outside the named layers. A new layer is a design change and needs its rule in
`.golangci.yml`. An adapter gets a rule allowing its private subtree and an
exclusion from `adapter-new`: depguard cannot express "the current adapter" as
an import prefix. Until that rule is added, the fallback allows only the shared
layers, never another adapter. `test/` and `tools/` are drivers outside the layers.

The mail functions that run under a mailbox lock their caller holds keep the
callers checked in `internal/layout_callers_test.go`, over every production tag
set. The lock is not reentrant; a caller outside its section would otherwise
run unlocked with no warning from the lock itself.

## Harness names outside the adapters

No Go file under `core`, `host`, `cli` or `infra` names a harness — its id or
title, or a word that belongs to one harness such as `mcp__`, `CLAUDE_`, `app-server`,
`.claude`, `.codex`, `.agents` — in an identifier, a string or a comment: a comment that
explains core code by one harness's behaviour is the first step toward a branch on it.
Since the owner's decision of October 17, 2026 no test reads the words; the import rule
keeps every adapter package out of the core, and review holds the rest.

Decided by the owner, October 18, 2026: the import check moves to depguard and
the separate prohibition of adapter registries is removed.
