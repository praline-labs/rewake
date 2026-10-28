// What the shell's thread and a face's worker say to each other
// (docs/face.md, "Hot reload"): sizes, keys and pings one way, lines and
// errors the other. Both ends import these types alone, so a reload of the
// face never needs the shell to change.
import type { FaceLine } from "./render.ts";

export type ToFace =
  | { kind: "start"; path: string; width: number; height: number }
  | { kind: "size"; width: number; height: number }
  | { kind: "scroll"; by: number }
  | { kind: "prompt"; id: string; text: string }
  | { kind: "ping"; n: number };

// link is the face's socket to the wrapper: connecting, live with a whole
// snapshot, or lost and connecting again.
export type Link = "connecting" | "live" | "lost";

export type FromFace =
  | { kind: "lines"; lines: FaceLine[]; turn: string; link: Link; prompts: boolean }
  | { kind: "error"; text: string }
  | { kind: "answer"; id: string; ok: boolean; detail?: string }
  | { kind: "pong"; n: number };
