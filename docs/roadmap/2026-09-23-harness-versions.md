# A named harness version, cached and run in a container — September 23, 2026

The owner asked, on September 21, 2026, to learn what a new harness version breaks
before updating their own installation, never by testing on it; and, when this was
scheduled, for two things explicitly: downloaded binaries are cached rather than
fetched on every start, and the version under test is easy to name. This closes the
schema half of that queue entry. The behaviour half stays queued in
[work-queue.md](../work-queue.md).

**What was built.** `tools/harnesscache`, one Go program over two packages the suite
imports as well. `cache` resolves a selector — an exact version, `latest`, or
`installed`, which asks the harness on PATH for `--version` and then fetches that
version like any other — against the npm registry, downloads the tarball once to a
file, compares it with the registry's sha512 `dist.integrity`, and only then unpacks
it, accepting regular files and directories alone and writing through an `os.Root`; the
version's directory gets its name only by a rename at the very end. A download cut
halfway, a digest that does not match, a tarball naming a path outside itself, any
link, device or fifo, or a package whose executable moved leaves no version behind; a directory a killed fetch left is listed as
unfinished and never used. Nothing is removed automatically: `list` and `remove` are
the only ways a version goes. `container` runs a cached version in a fresh container
from a small image: the version mounted read-only, a tmpfs HOME, a read-only root, the
caller's user id, every capability dropped, no new privileges, limits on processes and
memory, no network unless one is named, one writable directory at most, and the
container removed when it exits — or, within a bounded wait, when the context ends,
because killing the docker client leaves its container running. A path containing a
comma is refused, since docker's `--mount` would read it as more fields.

**How the suite takes a version.** `REWAKE_CODEX_VERSION`, read only under
`REWAKE_WORKFLOW`. Unset, the schema case uses the installed codex, as before. Set, the
version is resolved and fetched once in `TestMain`, before any case starts, within half
of go test's `-timeout` (at most ten minutes), and the schema is generated from it in
the container. A version that cannot be resolved, fetched or run is a failure of the
run, recorded in the run record with the version as typed and the reason, and the
schema case is red with the same reason — never unsupported, and never the installed
codex instead, because a run asked about one version and answering about another is
green about the wrong thing. The run record now says what the run was checked against,
as it happened: the schema from which codex, and whether this run downloaded it or
found it cached; the scenarios, in both columns, against the fixture. Both the `-v`
line and the summarizer print it.

**The five ordinary checks start no harness.** Before this change they did: the direct
shape tests ran the installed codex's schema generation, with the owner's HOME, on every
`go test ./...`. They now need the suite switch like every scenario. So a harness binary
the owner installed is run only under `REWAKE_WORKFLOW`: with no version named, the
schema case runs the installed codex to generate the schema, as it always did; with a
version named, the only contact is the `installed` selector asking for `--version`.

**Where things live.** The cache is `~/.cache/rewake/harness/<harness>/<version>/`
(the user's cache home, or `REWAKE_HARNESS_CACHE`), each version beside a
`manifest.json` naming its tarball, integrity and executable. The image is
`rewake-harness:<12 hex>`, the hex taken from the recipe's text: an edited recipe is a
new tag built on first use, an unchanged one reuses the image. It is `alpine:3.22`,
pinned by digest, with nothing installed, because both harnesses run on musl: Codex is
static, and Claude Code's musl build takes its libc from this image, so the digest pins
Claude Code's runtime as well. [research-launch.md](../research-launch.md) has the
package layouts that decided it.

**Measured, on this machine.** Codex 0.155.1: 142 MB downloaded, 370 MB unpacked; the
tool's first fetch took 54 s, inside the suite 13 s, and a second fetch 0.00 s with no
request. Codex 0.156.0: 149 MB, 387 MB, 25 s. Claude Code 2.1.280 (musl): 103 MB,
227 MB, 60 s. The image: 12.7 MB, built from a base already present.
Generating the schema in the container: under a second. The suite, run four times
through the summarizer, took 2m3s unset, 2m18s naming 0.155.1 for the first time,
2m3s naming it again, and 2m26s naming 0.156.0; the schema case itself took 0.1 s,
14.7 s, 0.4 s and 26.2 s — the scenarios run in parallel, so a download shows in the
case, and only partly in the run.

**What 0.156.0 showed.** Green: every message the shim sends still matches the schema
0.156.0 generates, and the fixture accepts no delivery that schema refuses. Its schema
is not the same as 0.155.1's — 857 types instead of 851; `ThreadRollbackParams` and
its response gone; `disabledPluginIds` added to `ThreadStartResponse`,
`TurnStartParams` and four more, `daybreakEnabled` to `ThreadStartParams`, and nested
changes inside `Thread`, `ThreadItem` and `UserInput` that add no top-level field. The
check passing says none of it requires a field the shim leaves out or refuses one the
shim sends. That is the boundary this tier has: it would have been red on
a new required field or a removed one, and it says nothing about behaviour.

**Found in review, before landing.** A chain of symbolic links wrote outside the unpack
directory: the first version checked each link's target from the link's own directory
and unpacked while the digest was still being computed, so an archive of two links
and a file placed the file one level above the cache before any check could refuse it.
It now verifies first and refuses links, and the reviewer's archive is a test. The
suite read the version variable during the five ordinary checks, and the first
download ran inside the schema case's deadline and could outlive go test's own; both
moved as described above.

**What stays open.** A real harness of a named version against a local responder,
for both harnesses — the tier that would catch a change of behaviour. The fetch and
the container are shaped for it: the cache fetches Claude Code already, and the
container takes a network mode. Not built: the responder, and a way to point each
harness at it for one launch without editing its configuration. The base image moves
only when its digest in the recipe is edited.
