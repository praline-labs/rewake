---
description: The design of rewake's face as a package of its own, released and updated apart — its contract, which copy runs, live updates, trust, what stays in rewake; for acceptance, not built.
---

# The face as a plugin: the design

Not yet built. The owner (October 31, 2026): the face is a plugin, part of rewake's project in code
of its own, updated apart. [face.md](face.md) stays true but where this page moves it.

**The package.** `face/` at the root, published as `@praline-labs/rewake-face` on a version line of
its own from 1.0.0: the worker's side of `shell/src/face/` with its tests — `worker.ts`,
`model.ts`, `render.ts` and the modules it draws with, `pointer.ts`, `select.ts`, `scroll.ts`,
`wheel.ts`, `ticker.ts`, `progress.ts` — and the contract, `messages.ts` and `mirror.gen.ts`
(`tools/protogen` writes there). It imports no package, only Bun's runtime, so `face/build.ts`
bundles it into one `worker.js` (not minified: an error names a line), uploaded with
`package.json` and `LICENSE`, one build for every platform. `tools/release face <version>` is its
gate, rewake's rules ([install.md](install.md#releasing-to-npm)): a clean tree on `origin/main`,
the version in `face/package.json` and free by `E404` alone, an allowlist both ways, the manifest
holding the source's numbers, and the packed file run by HEAD's shell against a fixture's socket.
`--publish` only on the owner's word.

**The contract.** The worker reads the mirror's socket itself and speaks `ToFace` and `FromFace`
with the shell. Two numbers: the mirror's `version`, on `hello`, and `contract` in `messages.ts`,
the messages with their spans and tones, 1 from this change; each changes with any change of what
it names. The manifest lists both, `"rewake": {"contracts": [1], "mirror": [1]}`; `start` tells
the worker the shell's contract. The shell checks the manifest before it runs a copy, and `host.ts`
holds every `FromFace` to the contract — kinds, field types, known tones and marks, a bound on rows
and spans — stopping a face that sends anything else as one that threw, so code of another release
cannot throw in the shell's thread.

**Which copy runs.** The **bundled** copy, the face of rewake's commit, a second entrypoint of the
compiled shell as today; an **installed** one, the first `node_modules/@praline-labs/rewake-face`
up from the shell's directory, symlinks followed, as pi is found — `npm install -g
@praline-labs/rewake-face` puts it beside an npm install; or the package directory `REWAKE_FACE`
names, in its place. A copy whose manifest does not read, whose lists miss the shell's numbers or
whose `worker.js` is missing is refused. `REWAKE_FACE`, accepted, runs; else the higher version,
the bundled on a tie; one copy for every face of a shell. A copy not bundled that fails to load or
to answer a ping before its first frame gives way to the bundled one; when none runs the pane stays
native and the shell goes on; a failure after a frame is [face.md](face.md#hot-reload)'s. The strip
names the copy, `dev rewake 1.3.0`, and a refused one leaves a row under the input, `face 1.4.0
not used: speaks contract 2, this shell 1`. Not `rewake list`: the pick is the shell's, from its
environment, and the CLI could pick otherwise; main sees the strip with `rewake screen`.

**Updating live.** The compiled shell watches the scope directory where the installed copy is or
would appear, not the package's, which npm may replace whole, and `REWAKE_FACE`'s. After a second with
no event it reads the manifest and hashes `worker.js`; a change, or a copy come or gone, picks again
and starts every face again, losing only the scroll, since the model is the wrapper's. A copy caught
half-written fails to load and gives way to the bundled one until the next change. Where no such
directory exists, a later install is found by the next shell.

**Trust.** A Bun Worker confines nothing; the face runs with the person's rights, so what limits it
is where it comes from: the shell's own build, a `node_modules` above the install, where pi is
found too, or the shell's own environment (as `REWAKE_SHELL`, `REWAKE_PI`). Never
the project's directory or `.rewake/`, the room, a letter, the mirror or a session's environment: a
cloned repository brings no code the shell runs. The package is rewake's code, reviewed here, bound
by the owner's rule of October 28, 2026: the conversation goes to the shell's thread only.

**The build and the room.** The face takes no lease or lock, calls no state primitive, runs no
`rewake`; it reads a socket any process of the user may open. One build per room
([rules/state.md](rules/state.md#the-builds-identity)) stands: the build is the Go image, the room
keeps `bin/rewake`, and the face, like the shell, is outside both. It alone reaches a room of an
older build, by design; the manifest holds what must match, and `hello`'s version, a guard of one
build until now, is raised by any change of the mirror's lines.

**What stays in rewake.** The mirror model, its socket and the view's field, the mod's and the pi
extension's mirrors: they produce what the face reads and change with the session's build. In the
shell, `faces.ts` (pick, watch, lifetime), `host.ts` (the worker, its ping, the contract's check),
`view.tsx`, `rule.ts` and `theme.ts`, with `host.test.ts` and `view.test.tsx`: the input, the
switch and the painting work when the face does not.

**Working from sources.** The shell run from its sources (`shell/rewake-shell`, as `rewake2` runs
it) runs the checkout's `face/src/worker.ts`, or `REWAKE_FACE`'s copy, looks for no installed one
and watches `face/src/`: an edit is drawn by the next face. The face's tests run in `face/`.

**Costs.** Two releases and version lines, a second gate mode, a check of every frame, `face/` in
`tools/land` and `AGENTS.md` beside `shell/`; a face listing a contract must run on every shell
speaking it, older Bun included, and the gate tries only HEAD's. **Rejected:** versions of
`@praline-labs/rewake` (the one-name rule is one release's builds; a face's line would fall into
its ranges and tags); a dependency of rewake's entry (npm nests it, updated only with rewake); its
modules imported in the shell's thread (neither unloaded nor stopped); the source tree as the
upload (it runs, but one file is fewer to catch half-written); a face named by a project, a
template or the room (another's code); `rewake face update` (npm fetches; rewake fetches nothing).

Probe, October 31, 2026 (Bun 1.4.0): an executable from `bun build --compile` started a Worker from
a `.js` file outside it, by path or `file:` URL, and from a `.ts` file with a relative import.
