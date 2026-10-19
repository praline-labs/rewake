// The shell's own colours, from the person's palette where their terminal
// gave it (docs/v2/shell.md, "The bar"), with fallbacks that read on a dark
// and a light background alike: a terminal that answers nothing must not get
// black on black.
import type { TerminalColors } from "@opentui/core";

export type Theme = {
  // main and worker are the buttons' own colours, main's apart from the
  // workers'; idle is the background of a button not focused.
  main: string;
  worker: string;
  idle: string;
  // text is the hint line's words; background is the terminal's own when it
  // said, for the layers drawn over the panes.
  text: string;
  background?: string;
};

const fallback = { main: "#5f87d7", worker: "#87af5f", idle: "#585858", text: "#a8a8a8" };

export function themeOf(colors: TerminalColors | undefined): Theme {
  const p = colors?.palette ?? [];
  return {
    main: p[4] ?? fallback.main,
    worker: p[2] ?? fallback.worker,
    idle: p[8] ?? fallback.idle,
    text: colors?.defaultForeground ?? p[7] ?? fallback.text,
    background: colors?.defaultBackground ?? undefined,
  };
}

function luminance(hex: string): number {
  const v = /^#?([0-9a-f]{6})$/i.exec(hex)?.[1];
  if (!v) return 0;
  const [r, g, b] = [0, 2, 4].map((i) => {
    const c = parseInt(v.slice(i, i + 2), 16) / 255;
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
