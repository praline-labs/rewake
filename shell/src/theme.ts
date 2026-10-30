// The shell's own colours, from the person's palette where their terminal
// gave it (docs/shell.md, "The bar"), with fallbacks that read on a dark
// and a light background alike: a terminal that answers nothing must not get
// black on black.
import type { TerminalColors } from "@opentui/core";
import { standInForeground } from "./colors.ts";

export type Theme = {
  // main and worker are the buttons' own colours, main's apart from the
  // workers'; idle is the background of a button not focused.
  main: string;
  worker: string;
  idle: string;
  // edge is a worker's lit button edge (docs/shell.md, "The bar"), the one
  // colour for the focused button and the others.
  edge: string;
  // text is the closing screen's words, and background the terminal's own
  // when it said, for the layers drawn over the panes; the shell's rows draw
  // their words in the terminal's own foreground.
  text: string;
  background?: string;
  // band is the background of the shell's own rows — the strip, the bar,
  // the hints — a shade off the terminal's background that sets them apart
  // from the programs' last lines without a row of their own; none when the
  // terminal did not say its background.
  band?: string;
  // faint is the separators' colour, and what has gone stale: the renderer
  // draws no dim over the terminal's own foreground, so it is a colour of its
  // own, halfway to the background where the terminal said both.
  faint: string;
  // faintOnIdle is faint words on a button's grey, a closed brief's title:
  // faint itself is that grey where the terminal did not say both colours.
  faintOnIdle: string;
  // selection is the colours of a selection over the shell's own rows, the
  // ones a pane's has: the widget swaps a selected cell's colours, and a
  // pane's text is mostly the default foreground on the default background.
  selection: { bg: string; fg: string };
  // palette is the sixteen colours as the terminal said them, a hole where
  // it did not: a colour drawn by its index is the terminal's own on the
  // screen, but rewake screen knows only what was said.
  palette: (string | null)[];
  // look is the fixed colours rewake's face draws with, Claude Code's own
  // (docs/face.md, "The look"): its dark set, or its light one on a terminal
  // whose background is light.
  look: Look;
};

// Look is the colours of Claude Code's own terminal theme that its screen
// draws in whatever the terminal's palette, each named by the element it
// colours there.
export type Look = {
  // success is a finished call's dot and a completed letter's.
  success: string;
  // error is a failed call's dot and its error text.
  error: string;
  // warning is a killed letter's dot, and the face's mark of a gap.
  warning: string;
  // code is inline code in a reply.
  code: string;
  // spinner is the working line's glyph and verb, and shimmer the lighter
  // shade its verb pulses towards.
  spinner: string;
  shimmer: string;
  // rule is the grey of the rules around the input and of the title on them.
  rule: string;
  // plan and edits are the names of plan mode and of accepting edits under
  // the input; auto mode's is warning's, bypassing permissions error's.
  plan: string;
  edits: string;
  // added and removed are the backgrounds of a diff's lines.
  added: string;
  removed: string;
};

// The two sets as Claude Code's dark and light themes define them.
export const looks: { dark: Look; light: Look } = {
  dark: {
    success: "#4eba65",
    error: "#ff6b80",
    warning: "#ffc107",
    code: "#b1b9f9",
    spinner: "#d77757",
    shimmer: "#eb9f7f",
    rule: "#888888",
    plan: "#48968c",
    edits: "#af87ff",
    added: "#225c2b",
    removed: "#7a2936",
  },
  light: {
    success: "#2c7a39",
    error: "#ab2b3f",
    warning: "#966c1e",
    code: "#5769f7",
    spinner: "#d77757",
    shimmer: "#f59575",
    rule: "#999999",
    plan: "#006666",
    edits: "#8700ff",
    added: "#69db7c",
    removed: "#ffa8b4",
  },
};

const fallback = {
  main: "#5f87d7",
  worker: "#87af5f",
  idle: "#585858",
  text: "#a8a8a8",
  edge: "#ffd75f",
};

// bandShare is how far the band moves from the background towards the ink
// on it: enough to read as a band, not as a bar.
const bandShare = 0.06;

// faintShare is how far faintOnIdle moves from the ink on the grey towards
// the grey: far enough to read as faint, not so far that it fades into it.
const faintShare = 0.4;

export function themeOf(colors: TerminalColors | undefined): Theme {
  const p = colors?.palette ?? [];
  const background = colors?.defaultBackground ?? undefined;
  const selected = colors?.defaultForeground ?? standInForeground;
  const worker = p[2] ?? fallback.worker;
  const idle = p[8] ?? fallback.idle;
  const text = colors?.defaultForeground ?? p[7] ?? fallback.text;
  return {
    main: p[4] ?? fallback.main,
    worker,
    idle,
    edge: standOut([p[11] ?? fallback.edge, p[3] ?? fallback.edge, text], [worker, idle]),
    text,
    background,
    faint:
      background && colors?.defaultForeground
        ? mix(colors.defaultForeground, background, 0.5)
        : (p[8] ?? fallback.idle),
    faintOnIdle: mix(ink(idle), idle, faintShare),
    band: background && mix(background, ink(background), bandShare),
    selection: { bg: selected, fg: background ?? ink(selected) },
    palette: [...p],
    look: background && ink(background) === "#1c1c1c" ? looks.light : looks.dark,
  };
}

function channels(hex: string): number[] {
  const v = /^#?([0-9a-f]{6})$/i.exec(hex)?.[1] ?? "000000";
  return [0, 2, 4].map((i) => parseInt(v.slice(i, i + 2), 16));
}

// mix is a moved share of the way from a to b.
export function mix(a: string, b: string, share: number): string {
  const [x, y] = [channels(a), channels(b)];
  return `#${x
    .map((c, i) =>
      Math.round(c + ((y[i] ?? c) - c) * share)
        .toString(16)
        .padStart(2, "0"),
    )
    .join("")}`;
}

function luminance(hex: string): number {
  const [r, g, b] = channels(hex).map((n) => {
    const c = n / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r! + 0.7152 * g! + 0.0722 * b!;
}

export function contrast(a: string, b: string): number {
  const [x, y] = [luminance(a), luminance(b)].sort((m, n) => n - m);
  return (x! + 0.05) / (y! + 0.05);
}

// standOut is the candidate whose weakest contrast with the backgrounds is
// the strongest, the first of equals.
function standOut(candidates: string[], backgrounds: string[]): string {
  const weakest = (c: string) => Math.min(...backgrounds.map((b) => contrast(c, b)));
  return candidates.reduce((best, c) => (weakest(c) > weakest(best) ? c : best));
}

// ink is the text for a background: near black or near white, whichever
// reads better on it.
export function ink(background: string): string {
  return contrast(background, "#1c1c1c") >= contrast(background, "#eeeeee") ? "#1c1c1c" : "#eeeeee";
}

// inkOr is colour on the background when it reads there, else ink.
export function inkOr(colour: string, background: string): string {
  return contrast(colour, background) >= 3 ? colour : ink(background);
}
