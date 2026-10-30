// What the shell's thread and a face's worker say to each other
// (docs/face.md, "Hot reload"): sizes, keys and pings one way, lines and
// errors the other. Both ends import these types alone, so a reload of the
// face never needs the shell to change.

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
};

// FaceLine is one row of the face, its spans left to right; band paints the
// whole row in the band's background, as a prompt of the person's is.
export type FaceLine = { spans: Span[]; band?: boolean };

export type ToFace =
  | { kind: "start"; path: string; width: number; height: number }
  | { kind: "size"; width: number; height: number }
  | { kind: "scroll"; by: number }
  // wheel is one notch of the mouse wheel and when it came: the face turns
  // notches into rows by how fast they come.
  | { kind: "wheel"; up: boolean; at: number }
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
