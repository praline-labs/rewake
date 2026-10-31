---
description: The design of rewake's face as code of its own in rewake's repository, built into rewake's release — its contract, which copy runs, live reload, what stays in the shell; for acceptance, not built.
---

# The face as a plugin: the design

Not yet built. The owner (October 31, 2026): the face is a plugin, part of rewake's project in code
of its own — and then, the same day: not published, built into rewake. [face.md](face.md) stays
true but where this page moves it.

**The code.** `face/` at the root, beside `shell/`: the worker's side of `shell/src/face/` with its
tests — `worker.ts`, `model.ts`, `render.ts` and the modules it draws with, `pointer.ts`,
`select.ts`, `scroll.ts`, `wheel.ts`, `ticker.ts`, `progress.ts` — and the contract, `messages.ts`
and `mirror.gen.ts` (`tools/protogen` writes there). It imports no package, only Bun's runtime; its
own `package.json` holds its checks, `bun run check` and `bun test`. No version line, no upload:
rewake's release builds it.

**How it ships.** Embedded, as today: `shell/build.ts` compiles `face/src/worker.ts` into the
shell as its second entrypoint. A file beside the shell would add a lookup, a file the packages
and the gate's allowlist carry, and a way for the two to come apart, for nothing a person can use:
nothing replaces it but `REWAKE_FACE`, which works either way. The gate starts the installed
shell's embedded face (`rewake-shell --face-check`), since a moved entrypoint breaks only in a
compiled shell.

**The contract.** The worker reads the mirror's socket itself and speaks `ToFace` and `FromFace`
with the shell. `contract` in `messages.ts` names their shape, 1 from this change, raised with any
change of it; `start` carries the shell's, and a worker that speaks another posts why and draws
nothing. `host.ts` holds every `FromFace` to the contract — kinds, field types, known tones and
marks, the sizes of strings and the ranges of numbers, a bound on rows and spans — and stops a face
that sends anything else as one that threw: a face from another checkout cannot throw in the
shell's thread, and a failing face never fails the shell.

**Which copy runs.** `REWAKE_FACE`, the path of a worker script, when the shell's environment sets
it; else the shell's own: the embedded one in a compiled shell, `face/src/worker.ts` of the
checkout in a shell run from its sources (`shell/rewake-shell`, as `rewake2` runs it). The shell
watches the directory of the copy it runs from a file, recursively, and starts every face again
150 ms after the last change, as it does now; the embedded copy is watched by nothing.

**What stays in the shell.** `faces.ts` (which copy, the watch, the faces' lifetime), `host.ts`
(the worker, its ping, the contract's check), `view.tsx`, `rule.ts` and `theme.ts`, with
`host.test.ts` and `view.test.tsx`: the input, the switch and the painting work when the face does
not. The mirror model, its socket and the mirrors of the mod and the pi extension stay in Go and
TypeScript beside them, as they are: they produce what the face reads, with the session's build.

**The build and the room.** The face takes no lease or lock, calls no state primitive and runs no
`rewake`; one build per room ([rules/state.md](rules/state.md#the-builds-identity)) is untouched.

**Rejected.** A package of its own on npm, with its own release and version line, an installed copy
looked for beside rewake's and watched there: the owner, October 31, 2026, «а мы можем не
публиковать ее? а просто встроить в rewake?» — one release is enough. Its modules imported in the
shell's thread: they are neither unloaded nor stopped ([face.md](face.md#hot-reload)). A face named
by a project, a template or the room: code the shell would run from a repository or another
session.

Probe, October 31, 2026 (Bun 1.4.0): an executable from `bun build --compile` started a Worker from
a `.js` file outside it, by path or `file:` URL, and from a `.ts` file with a relative import.
