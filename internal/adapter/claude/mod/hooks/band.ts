// A band above Claude Code's input with the session's permission mode at its
// left and its name with its context at its right, the same line rewake's face
// draws as the input's rule (docs/face.md, "The look"; shell/src/face/rule.ts),
// so a native pane and a face pane tell the same. It only informs: its code
// answers no verdict and no prompt, and a hook of it that fails is dropped
// with the band by the engine (docs/claude.md, "The band").
import type { Register } from "claude-code";

// Mode is a permission mode as the terminal program names it under its input.
interface Mode {
  label: string;
  color: string;
}

// modes are the harness's permission modes by the id its classic events
// report; the default one says nothing.
const modes: Record<string, Mode> = {
  auto: { label: "⏵⏵ auto mode", color: "warning" },
  plan: { label: "⏸ plan mode", color: "planMode" },
  acceptEdits: { label: "⏵⏵ accept edits", color: "autoAccept" },
  bypassPermissions: { label: "⏵⏵ bypass permissions", color: "error" },
  dontAsk: { label: "⏵⏵ don't ask", color: "error" },
};

// kilo is a count as the strip writes it: 950, 46k, 1M, 1.5M.
export function kilo(n: number): string {
  if (n >= 1_000_000) return `${Math.round(n / 100_000) / 10}M`;
  if (n >= 1000) return `${Math.round(n / 1000)}k`;
  return String(n);
}

// contextOf is the context as the face's rule shows it: 46k/272k, and –/272k
// before the harness counted it; nothing without a window.
export function contextOf(tokens: number | undefined, window: number | undefined) {
  if (!window) return undefined;
  return `${tokens === undefined ? "–" : kilo(tokens)}/${kilo(window)}`;
}

const segmenter = new Intl.Segmenter();

// wide says a character takes two cells: the East Asian wide blocks and the
// pictographs.
const wide = (c: number) =>
  (c >= 0x1100 && c <= 0x115f) ||
  (c >= 0x2e80 && c <= 0xa4cf) ||
  (c >= 0xac00 && c <= 0xd7a3) ||
  (c >= 0xf900 && c <= 0xfaff) ||
  (c >= 0xfe30 && c <= 0xfe6f) ||
  (c >= 0xff00 && c <= 0xff60) ||
  (c >= 0xffe0 && c <= 0xffe6) ||
  (c >= 0x1f300 && c <= 0x1faff);

// cell is the width of a text in terminal cells, a grapheme at a time.
function cell(text: string): number {
  let n = 0;
  for (const { segment } of segmenter.segment(text)) {
    n += wide(segment.codePointAt(0) ?? 0) ? 2 : 1;
  }
  return n;
}

// cutName is the name within room cells, cut with … where it is longer.
function cutName(name: string, room: number): string {
  if (cell(name) <= room) return name;
  let out = "";
  for (const { segment } of segmenter.segment(name)) {
    if (cell(out + segment) > room - 1) break;
    out += segment;
  }
  return `${out}…`;
}

// Row is the band's text: the mode at the left, the gap, the right part.
export interface Row {
  mode?: Mode;
  gap: number;
  right: string;
}

// rowOf lays the band out in cells columns as the face's rule does: the mode
// is dropped first, then the context, then the name is cut. At least a cell
// lies between the mode and the right part; undefined where nothing fits.
export function rowOf(
  cells: number,
  name: string | undefined,
  context: string | undefined,
  mode: Mode | undefined,
): Row | undefined {
  const tries: [Mode | undefined, string | undefined][] = [];
  if (mode) tries.push([mode, name ? context : undefined]);
  if (name && context) tries.push([undefined, context]);
  for (const [shown, ctx] of tries) {
    const right = name ? `${name}${ctx ? ` · ${ctx}` : ""}` : "";
    const left = shown ? cell(shown.label) : 0;
    const gap = cells - left - cell(right);
    if (gap < (shown && right ? 1 : 0)) continue;
    return { mode: shown, gap, right };
  }
  if (!name || cells < 2) return undefined;
  const right = cutName(name, cells);
  return { gap: cells - cell(right), right };
}

// The session's facts, set by the hooks that hear them; the render hook only
// reads them, so it waits for nothing.
export class Band {
  private name?: string;
  private mode?: string;
  private tokens?: number;
  private window?: number;
  private redraw: () => void = () => {};

  // begin makes the band live for a rewake run: name is the session's.
  begin(name: string | undefined, redraw: () => void) {
    this.name = name || undefined;
    this.redraw = redraw;
    this.redraw();
  }

  get live(): boolean {
    return this.name !== undefined;
  }

  // moded takes the mode a classic event reports.
  moded(mode: string | undefined) {
    if (!mode || mode === this.mode) return;
    this.mode = mode;
    this.redraw();
  }

  // measured takes the context the harness counted.
  measured(tokens: number | undefined, window: number | undefined) {
    if (tokens === this.tokens && window === this.window) return;
    this.tokens = tokens;
    this.window = window;
    this.redraw();
  }

  row(cells: number): Row | undefined {
    if (!this.name) return undefined;
    const context = contextOf(this.tokens, this.window);
    return rowOf(cells, this.name, context, this.mode ? modes[this.mode] : undefined);
  }
}

// serveBand draws the band; its facts are told by moded and sample.
export function serveBand(on: Parameters<Register>[0], band: Band) {
  on("ui.render", { component: "AbovePrompt" }, ($, e, next) => {
    if (!band.live || e.props.hasSurvey) return next(e);
    const row = band.row(e.props.bodyColumns);
    if (!row) return next(e);
    const { Box, Text } = $.ui.resolve(e);
    const parts = [];
    if (row.mode) parts.push(Text({ color: row.mode.color, children: row.mode.label }));
    parts.push(Text({ dimColor: true, children: " ".repeat(row.gap) + row.right }));
    return Box({ flexDirection: "row", children: parts });
  });
}

// sample reads the context the harness counted now, for the band.
export async function sample(
  usage: () => Promise<{ context?: { tokens?: number; window?: number } }>,
  band: Band,
) {
  try {
    const { context } = await usage();
    band.measured(context?.tokens, context?.window);
  } catch {
    // The band keeps the last figures.
  }
}
