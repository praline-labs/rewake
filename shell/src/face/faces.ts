// The faces of the shell's panes (docs/face.md, "The face"): a pane whose
// session's wrapper serves a mirror can show rewake's face over its native
// one, which stays mounted, sized and fed underneath, so its pty never sees a
// change. The face is code of its own (face/): the shell runs the worker
// REWAKE_FACE names, or its own, and reloads every face when a file of the
// copy it runs from disk changes; a compiled shell's own copy is embedded and
// watched by nothing.
import { existsSync, watch } from "node:fs";
import { dirname, resolve } from "node:path";
import { createSignal, untrack } from "solid-js";
import type { FromFace, Pointing, ToFace } from "../../../face/src/messages.ts";
import { faultOf } from "./check.ts";
import { FaceHost, type FaceState } from "./host.ts";

export type FaceMode = "rewake" | "native";

type Face = { host: FaceHost; path: string };

const faces = new Map<string, Face>();
const [states, setStates] = createSignal(new Map<string, FaceState>());
const [modes, setModes] = createSignal(new Map<string, FaceMode>());
const [drafts, setDrafts] = createSignal(new Map<string, string>());
const [inputs, setInputs] = createSignal(new Map<string, InputView>());

// A compiled shell holds the face as a second entrypoint, named from the
// bundle's root (build.ts); from the sources it is the checkout's face/.
// REWAKE_FACE, the path of a worker script, is read from the shell's own
// environment, never from a session's or the room's.
const compiled = import.meta.url.includes("$bunfs");
const embedded = "./face/src/worker.ts";
const named = process.env.REWAKE_FACE
  ? resolve(process.env.REWAKE_SHELL_CWD || process.cwd(), process.env.REWAKE_FACE)
  : undefined;
const workerFile =
  named ??
  (compiled ? undefined : new URL("../../../face/src/worker.ts", import.meta.url).pathname);
export const workerUrl = workerFile ?? embedded;
const sources = workerFile ? dirname(workerFile) : undefined;
const reloadAfter = 150;
let watching = false;
// copier is the shell's one path to the person's clipboard, the terminal's
// OSC 52 (main.tsx); no face has another.
let copier: (text: string) => boolean = () => false;
export const copyWith = (copy: (text: string) => boolean) => {
  copier = copy;
};

const put = <T>(map: Map<string, T>, id: string, value: T | undefined) => {
  const next = new Map(map);
  if (value === undefined) next.delete(id);
  else next.set(id, value);
  return next;
};

// faceOf is a pane's face as drawn: absent for a pane whose session serves
// no mirror.
export const faceOf = (id: string) => states().get(id);
export const modeOf = (id: string): FaceMode | undefined =>
  states().has(id) ? (modes().get(id) ?? "rewake") : undefined;
export const faceShown = (id: string) => modeOf(id) === "rewake";
export const draftOf = (id: string) => drafts().get(id) ?? "";
export const setDraft = (id: string, text: string) => setDrafts((m) => put(m, id, text));
// InputView is the input as the person's window lays it out: its rows, and
// how many rows of its text are hidden above and under them. Every window of
// the pane draws the input so.
export type InputView = { rows: number; above: number; under: number };
const oneRow: InputView = { rows: 1, above: 0, under: 0 };
export const inputOf = (id: string) => inputs().get(id) ?? oneRow;
export const inputRowsOf = (id: string) => inputOf(id).rows;
export const setInput = (id: string, view: InputView) => {
  // Read untracked: an effect that sets it does not depend on it.
  const was = untrack(() => inputOf(id));
  if (was.rows !== view.rows || was.above !== view.above || was.under !== view.under)
    setInputs((m) => put(m, id, view));
};

// takeMirrors gives each pane its session's mirror socket as the room reads
// it; a face starts for a new socket and ends with its pane or its socket.
export function takeMirrors(mirrors: [string, string | undefined][]) {
  const wanted = new Map(mirrors.filter((m): m is [string, string] => m[1] !== undefined));
  for (const [id, face] of faces) {
    if (wanted.get(id) === face.path) continue;
    face.host.end();
    faces.delete(id);
    setStates((m) => put(m, id, undefined));
  }
  for (const [id, path] of wanted) {
    if (faces.has(id)) continue;
    const host = new FaceHost({
      path,
      url: workerUrl,
      onState: (s) => setStates((m) => put(m, id, s)),
      copy: (text) => copier(text),
    });
    faces.set(id, { host, path });
    host.start();
    watchSources();
  }
}

// toggleFace is the prefix's v on a pane with a face: rewake's or native.
// Shown again after a failure, a stopped face starts again.
export function toggleFace(id: string) {
  const face = faces.get(id);
  if (!face) return;
  const next: FaceMode = modeOf(id) === "rewake" ? "native" : "rewake";
  setModes((m) => put(m, id, next));
  if (next === "rewake" && !face.host.running) face.host.start();
}

export const sizeFace = (id: string, width: number, height: number) =>
  faces.get(id)?.host.size(width, height);
export const scrollFace = (id: string, by: number) => faces.get(id)?.host.scroll(by);
export const wheelFace = (id: string, up: boolean) => faces.get(id)?.host.wheel(up);
export const pointFace = (id: string, type: Pointing, x: number, y: number, focused: boolean) =>
  faces.get(id)?.host.mouse(type, x, y, focused);
export const unselectFace = (id: string) => faces.get(id)?.host.unselect();
export const unfoldFace = (id: string) => faces.get(id)?.host.unfold();
export const escapeFace = (id: string) => faces.get(id)?.host.escape();
export const promptFace = (id: string, text: string) => faces.get(id)?.host.prompt(text) ?? false;

// checkFace starts the face this shell runs and waits for its answer to a
// ping: `rewake-shell --face-check`, the release gate's proof that a compiled
// shell carries a face that loads, since a moved entrypoint breaks only
// there.
export function checkFace(within = 5000): Promise<string | undefined> {
  return new Promise((done) => {
    let worker: Worker;
    try {
      worker = new Worker(workerUrl);
    } catch (err) {
      return done(`the face does not start: ${String(err)}`);
    }
    const end = (why?: string) => {
      clearTimeout(timer);
      worker.terminate();
      done(why);
    };
    const timer = setTimeout(
      () => end(`the face did not answer within ${within / 1000} s`),
      within,
    );
    worker.addEventListener("error", (event) => {
      event.preventDefault();
      end(`the face failed: ${event.message || String(event)}`);
    });
    worker.addEventListener("message", (event: MessageEvent<unknown>) => {
      const fault = faultOf(event.data);
      if (fault) return end(`the face sent what the contract does not allow: ${fault}`);
      if ((event.data as FromFace).kind === "pong") end();
    });
    worker.postMessage({ kind: "ping", n: 1 } satisfies ToFace);
  });
}

export function reloadFaces() {
  for (const face of faces.values()) face.host.start();
}

export function endFaces() {
  for (const face of faces.values()) face.host.end();
}

// watchSources reloads the faces 150 ms after the last change of a file in
// the directory of the copy run from disk.
function watchSources() {
  if (watching || !sources || !existsSync(sources)) return;
  watching = true;
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    watch(sources, { recursive: true }, () => {
      clearTimeout(timer);
      timer = setTimeout(reloadFaces, reloadAfter);
    });
  } catch {
    // No watch: a face is reloaded by the switch alone.
  }
}
