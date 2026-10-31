// What the shell's thread and a face's worker say to each other
// (docs/face.md, "Hot reload"): sizes, keys and pings one way, lines and
// errors the other. Both ends import these types alone, so a reload of the
// face never needs the shell to change.

import type { PermissionMode } from "./mirror.gen.ts";

// Tone is a span's colour by its meaning; the shell's thread picks the
// colour from the person's theme (view.tsx).
export type Tone = "text" | "faint" | "code" | "accent" | "done" | "error" | "gap";

// Mark is a span's background by its meaning: a diff's added or removed line.
export type Mark = "added" | "removed";

// Span is a run of one style within a row.
export type Span = {
  text: string;
  tone?: Tone;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  mark?: Mark;
  // glow is how far an accent moves towards its lighter shade, 0 to 1: the
  // working verb's pulse.
  glow?: number;
  // blink is a running call's dot, shown and hidden on the face's clock.
  blink?: boolean;
  // gutter is drawing, not text: a mark, an indent, a diff's sign or a
  // table's lines, left out of a selection and of what it copies.
  gutter?: boolean;
  // join marks a row that goes on with the line above, broken to fit the
  // pane: what the break swallowed, said between the two when copied.
  join?: string;
  // cell is the column of a table's cell, copied by its text.
  cell?: number;
  // code is the fenced block of a reply the span is in, by its order.
  code?: number;
  // selected is a cell of the person's selection.
  selected?: boolean;
};

// FaceLine is one row of the face, its spans left to right; band paints the
// whole row in the band's background, as a prompt of the person's is. part
// names what a click on the row copies (select.ts), and lit is a row of the
// part a click just copied.
export type FaceLine = { spans: Span[]; band?: boolean; part?: string; lit?: boolean };

export type ToFace =
  | { kind: "start"; path: string; width: number; height: number }
  | { kind: "size"; width: number; height: number }
  | { kind: "scroll"; by: number }
  // wheel is one notch of the mouse wheel and when it came: the face turns
  // notches into rows by how fast they come.
  | { kind: "wheel"; up: boolean; at: number }
  | { kind: "prompt"; id: string; text: string }
  // mouse is the pointer over the conversation, in its cells from the top
  // left, past its edges while a drag goes on.
  // A click copies only on a focused face: the first one focuses it.
  | { kind: "mouse"; type: Pointing; x: number; y: number; focused: boolean }
  // unselect drops the selection, as a key does.
  | { kind: "unselect" }
  | { kind: "ping"; n: number };

// Pointing is what the pointer did with the left button: pressed, dragged
// and let go of it.
export type Pointing = "down" | "drag" | "up";

// link is the face's socket to the wrapper: connecting, live with a whole
// snapshot, or lost and connecting again.
export type Link = "connecting" | "live" | "lost";

export type FromFace =
  // mode is the session's permission mode, which the input's rule shows.
  | {
      kind: "lines";
      lines: FaceLine[];
      turn: string;
      link: Link;
      prompts: boolean;
      mode?: PermissionMode;
    }
  | { kind: "error"; text: string }
  // copy is text for the person's clipboard: a selection, or the part a
  // click was on, by its name.
  | { kind: "copy"; text: string; what: string }
  | { kind: "answer"; id: string; ok: boolean; detail?: string }
  | { kind: "pong"; n: number };
