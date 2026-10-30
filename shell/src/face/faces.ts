// The faces of the shell's panes (docs/face.md, "The face"): a pane whose
// session's wrapper serves a mirror can show rewake's face over its native
// one, which stays mounted, sized and fed underneath, so its pty never sees a
// change. Run from its sources, the shell reloads every face when a file of
// the face changes; a compiled shell has no sources to watch.
import { existsSync, watch } from "node:fs";
import { createSignal } from "solid-js";
import { FaceHost, type FaceState } from "./host.ts";

export type FaceMode = "rewake" | "native";

type Face = { host: FaceHost; path: string };

const faces = new Map<string, Face>();
const [states, setStates] = createSignal(new Map<string, FaceState>());
const [modes, setModes] = createSignal(new Map<string, FaceMode>());
const [drafts, setDrafts] = createSignal(new Map<string, string>());

// A compiled shell holds the face as a second entrypoint, named from the
// bundle's root (build.ts); from the sources it is the file beside this one.
const compiled = import.meta.url.includes("$bunfs");
const workerUrl = compiled ? "./face/worker.ts" : new URL("./worker.ts", import.meta.url).href;
const sources = new URL(".", import.meta.url).pathname;
const reloadAfter = 150;
let watching = false;

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
export const promptFace = (id: string, text: string) => faces.get(id)?.host.prompt(text) ?? false;

export function reloadFaces() {
  for (const face of faces.values()) face.host.start();
}

export function endFaces() {
  for (const face of faces.values()) face.host.end();
}

// watchSources reloads the faces 150 ms after the last change of a file of
// the face, only where the sources are on disk.
function watchSources() {
  if (watching || !existsSync(sources)) return;
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
