// Breaking styled text into rows of the pane's width (docs/face.md, "The
// face"): at spaces, as a terminal program does, a word cut only when it is
// wider than the row.
import type { Span } from "./messages.ts";

// clean is a text as the face can draw it: a tool's colours and cursor moves
// would put cells where the face's widths do not count them, so escapes and
// control characters go, a carriage return keeps what was drawn over last,
// and a tab is two spaces.
export function clean(text: string): string {
  return (
    text
      // biome-ignore lint/suspicious/noControlCharactersInRegex: a terminal's escapes are what goes.
      .replace(/\x1b\[[0-?]*[ -/]*[@-~]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?|\x1b[@-_]?/g, "")
      .split("\n")
      .map((line) => line.replace(/\r+$/, "").split("\r").pop() ?? "")
      .join("\n")
      .replace(/\t/g, "  ")
      // biome-ignore lint/suspicious/noControlCharactersInRegex: the control characters are what goes.
      .replace(/[\x00-\x09\x0b-\x1f\x7f]/g, "")
  );
}

export const widthOf = (text: string) => Bun.stringWidth(text);

// A row being laid out, code point by code point with its span and width.
type Cells = { ch: string[]; at: number[]; w: number[] };

function cellsOf(spans: Span[]): Cells {
  const cells: Cells = { ch: [], at: [], w: [] };
  spans.forEach((span, i) => {
    for (const ch of span.text) {
      cells.ch.push(ch);
      cells.at.push(i);
      cells.w.push(widthOf(ch));
    }
  });
  return cells;
}

// spansOf is a row's spans; a row that goes on from the one above joins it
// with what the break swallowed, any other keeps the first span's join.
function spansOf(cells: Cells, from: number, to: number, spans: Span[], join?: string): Span[] {
  const out: Span[] = [];
  for (let i = from; i < to; i++) {
    const { join: _, ...span } = spans[cells.at[i] ?? 0] ?? { text: "" };
    const last = out[out.length - 1];
    const ch = cells.ch[i] ?? "";
    if (last && cells.at[i - 1] === cells.at[i] && i > from) last.text += ch;
    else out.push({ ...span, text: ch });
  }
  const first = out[0];
  const kept = join ?? (from === 0 ? spans[cells.at[0] ?? 0]?.join : undefined);
  if (first && kept !== undefined) first.join = kept;
  return out;
}

// wrapSpans breaks one line of spans (no newline in it) into rows at most
// width columns wide: at the spaces, which a break swallows, and inside a
// word only when the word alone is wider than the row.
export function wrapSpans(spans: Span[], width: number): Span[][] {
  const cells = cellsOf(spans);
  const n = cells.ch.length;
  const rows: Span[][] = [];
  let start = 0;
  // join is what the last break swallowed, for the row after it.
  let join: string | undefined;
  while (start < n) {
    let used = 0;
    let end = start;
    // The last space that fits: a break there keeps the word whole.
    let lastSpace = -1;
    while (end < n && used + (cells.w[end] ?? 0) <= width) {
      if (cells.ch[end] === " ") lastSpace = end;
      used += cells.w[end] ?? 0;
      end++;
    }
    if (end === n) {
      rows.push(spansOf(cells, start, end, spans, join));
      break;
    }
    // A cut lands mid-word unless what follows is a space or a space fits.
    let next = end;
    if (cells.ch[end] === " ") {
      next = end;
    } else if (lastSpace > start) {
      end = lastSpace;
      next = lastSpace;
    } else if (end === start) {
      // A character wider than the row still takes one.
      end = start + 1;
      next = end;
    }
    let trim = end;
    while (trim > start && cells.ch[trim - 1] === " ") trim--;
    rows.push(spansOf(cells, start, trim, spans, join));
    while (next < n && cells.ch[next] === " ") next++;
    join = cells.ch.slice(trim, next).join("");
    start = next;
  }
  if (rows.length === 0) rows.push([]);
  return rows;
}

// wrap breaks a plain text, newlines included, into rows of at most width
// columns.
export function wrap(text: string, width: number): string[] {
  return text
    .split("\n")
    .flatMap((line) => wrapSpans([{ text: line }], width))
    .map((row) => row.map((s) => s.text).join(""));
}
