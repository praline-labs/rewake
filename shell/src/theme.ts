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
  // selection is the colours of a selection over the shell's own rows, the
  // ones a pane's has: the widget swaps a selected cell's colours, and a
  // pane's text is mostly the default foreground on the default background.
  selection: { bg: string; fg: string };
};

const fallback = { main: "#5f87d7", worker: "#87af5f", idle: "#585858", text: "#a8a8a8" };

// bandShare is how far the band moves from the background towards the ink
// on it: enough to read as a band, not as a bar.
const bandShare = 0.06;

export function themeOf(colors: TerminalColors | undefined): Theme {
  const p = colors?.palette ?? [];
  const background = colors?.defaultBackground ?? undefined;
  const selected = colors?.defaultForeground ?? standInForeground;
  return {
    main: p[4] ?? fallback.main,
    worker: p[2] ?? fallback.worker,
    idle: p[8] ?? fallback.idle,
    text: colors?.defaultForeground ?? p[7] ?? fallback.text,
    background,
    faint:
      background && colors?.defaultForeground
        ? mix(colors.defaultForeground, background, 0.5)
        : (p[8] ?? fallback.idle),
    band: background && mix(background, ink(background), bandShare),
    selection: { bg: selected, fg: background ?? ink(selected) },
  };
}

function channels(hex: string): number[] {
  const v = /^#?([0-9a-f]{6})$/i.exec(hex)?.[1] ?? "000000";
  return [0, 2, 4].map((i) => parseInt(v.slice(i, i + 2), 16));
}

// mix is a moved share of the way from a to b.
function mix(a: string, b: string, share: number): string {
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

function contrast(a: string, b: string): number {
  const [x, y] = [luminance(a), luminance(b)].sort((m, n) => n - m);
  return (x! + 0.05) / (y! + 0.05);
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
