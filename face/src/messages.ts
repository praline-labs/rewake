// What the shell's thread and a face's worker say to each other
// (docs/face.md, "Hot reload"): sizes, keys and pings one way, lines and
// errors the other. The face is code of its own (face/), and the shell
// imports these types, the shapes of what a face posts and the contract's
// number alone, so a reload of the face never needs the shell to change.

import { permissionModeValues, thinkingLevelValues } from "./mirror.gen.ts";
import { count, flag, list, type Of, one, optional, type Shape, serial, text } from "./shape.ts";

// contract is the version of these messages, raised with any change of
// them: start carries the shell's, and a face that speaks another draws
// nothing. The shell holds what a face sends to it to the shapes below
// (shell/src/face/check.ts).
export const contract = 7;

// refusal is a face's answer to a start of another contract. Its words, and
// the shapes of start, ping, pong and error, are the one part no contract
// changes: a shell of any contract reads the face's number from it and keeps
// the face it runs (docs/face.md, "Hot reload").
export const refusal = (face: number, shell: number) =>
  `this face speaks contract ${face}, the shell ${shell}`;
export const refusedBy = (text: string): number | undefined => {
  const words = /^this face speaks contract (\d+), the shell \d+$/.exec(text);
  return words ? Number(words[1]) : undefined;
};

// tones are a span's colours by their meaning; the shell's thread picks the
// colour from the person's theme (view.tsx).
const tones = ["text", "faint", "code", "accent", "done", "error", "gap"] as const;
export type Tone = (typeof tones)[number];

// marks are a span's backgrounds by their meaning: a diff's added or
// removed line.
const marks = ["added", "removed"] as const;
export type Mark = (typeof marks)[number];

// span is a run of one style within a row.
const span = {
  text: text("text"),
  tone: optional(one(tones)),
  bold: optional(flag),
  italic: optional(flag),
  underline: optional(flag),
  mark: optional(one(marks)),
  // blink is a running call's dot, shown and hidden on the face's clock.
  blink: optional(flag),
  // gutter is drawing, not text: a mark, an indent, a diff's sign or a
  // table's lines, left out of a selection and of what it copies.
  gutter: optional(flag),
  // join marks a row that goes on with the line above, broken to fit the
  // pane: what the break swallowed, said between the two when copied.
  join: optional(text("name")),
  // cell is the column of a table's cell, copied by its text.
  cell: optional(count),
  // code is the fenced block of a reply the span is in, by its order.
  code: optional(count),
  // selected is a cell of the person's selection.
  selected: optional(flag),
} as const satisfies Shape;
export type Span = Of<typeof span>;

// faceLine is one row of the face, its spans left to right; band paints the
// whole row in the band's background, as a prompt of the person's is. part
// names what a click on the row copies (select.ts), and lit is a row of the
// part a click just copied.
const faceLine = {
  spans: list(span, "spans", "a span"),
  band: optional(flag),
  part: optional(text("name")),
  lit: optional(flag),
} as const satisfies Shape;
export type FaceLine = Of<typeof faceLine>;

export type ToFace =
  | { kind: "start"; contract: number; path: string; width: number; height: number }
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
  // escape is the person's Esc: it drops the selection there is, else it
  // interrupts the session's working turn where the session takes it, the
  // answer coming back under its id.
  | { kind: "escape"; id: string }
  // unfold opens everything the face folds, or closes it again: the
  // person's ctrl+o, kept by the shell across a reload.
  | { kind: "unfold"; all: boolean }
  // thinking is the person's Alt+T: the session's thinking level moves
  // on where the session takes it, the answer coming back under its id.
  | { kind: "thinkingVisible" }
  | { kind: "thinking"; id: string }
  | { kind: "workmode"; id: string }
  | { kind: "ping"; n: number };

// Pointing is what the pointer did with the left button: pressed, dragged
// and let go of it.
export type Pointing = "down" | "drag" | "up";

// links are the face's socket to the wrapper: connecting, live with a whole
// snapshot, or lost and connecting again.
const links = ["connecting", "live", "lost"] as const;
export type Link = (typeof links)[number];

// fromFace are the shapes of what a face posts, by their kind.
export const fromFace = {
  // mode is the session's permission mode, and level its thinking level,
  // both as the mirror says them. workMode is a separate runtime stance,
  // shown on pi's input rule; level stays on the strip.
  lines: {
    lines: list(faceLine, "rows", "a row"),
    turn: text("name"),
    link: one(links),
    prompts: flag,
    mode: optional(one(permissionModeValues)),
    level: optional(one(thinkingLevelValues)),
    workMode: optional(text("name")),
  },
  error: { text: text("text") },
  // copy is text for the person's clipboard: a selection, or the part a
  // click was on, by its name.
  copy: { text: text("copy"), what: text("name") },
  answer: { id: text("name"), ok: flag, detail: optional(text("text")) },
  pong: { n: serial },
} as const satisfies Record<string, Shape>;

type Shapes = typeof fromFace;
export type FromFace = { [K in keyof Shapes]: { kind: K } & Of<Shapes[K]> }[keyof Shapes];
