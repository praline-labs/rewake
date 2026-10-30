// A markdown table drawn with box lines, as Claude Code's terminal draws one
// (docs/face.md, "The look"): a rule between every row, the header bold, the
// columns shrunk and their cells wrapped when the table is wider than the
// row.

import type { Span } from "./messages.ts";
import { widthOf, wrapSpans } from "./wrap.ts";

type Align = "left" | "center" | "right";

export const separator = /^\s*\|?\s*:?-{3,}:?\s*(\|\s*:?-{3,}:?\s*)*\|?\s*$/;

export const cellsOf = (line: string) =>
  line
    .trim()
    .replace(/^\||\|$/g, "")
    .split(/(?<!\\)\|/)
    .map((cell) => cell.trim().replace(/\\\|/g, "|"));

const alignOf = (cell: string): Align =>
  cell.startsWith(":") && cell.endsWith(":") ? "center" : cell.endsWith(":") ? "right" : "left";

const spanWidth = (spans: Span[]) => spans.reduce((n, s) => n + widthOf(s.text), 0);

// A column is at least this wide.
const minColumn = 3;
// The terminal keeps this much of the row free beside a table; two columns
// of it are the reply's indent, which width has already left out.
const margin = 2;

// widthsOf is each column's width: its widest cell where the table fits;
// else its longest word, and the room left shared out by how much each
// column's widest cell exceeds that; else its longest word scaled down.
function widthsOf(ideal: number[], least: number[], room: number): number[] {
  const sum = (ns: number[]) => ns.reduce((a, b) => a + b, 0);
  if (sum(ideal) <= room) return ideal;
  if (sum(least) <= room) {
    const over = ideal.map((w, i) => w - (least[i] ?? 0));
    const total = sum(over);
    return least.map((w, i) =>
      total === 0 ? w : w + Math.floor(((over[i] ?? 0) / total) * (room - sum(least))),
    );
  }
  const scaled = least.map((w) => Math.max(Math.floor((w * room) / sum(least)), minColumn));
  // A column held at its least can leave the rest over: the widest gives way.
  while (sum(scaled) > room) {
    const widest = scaled.indexOf(Math.max(...scaled));
    if ((scaled[widest] ?? 0) <= minColumn) break;
    scaled[widest] = (scaled[widest] ?? 0) - 1;
  }
  return scaled;
}

// tableRows draws a table's lines, the second its separator, in at most
// width columns; inline is how a cell's markdown becomes spans.
export function tableRows(
  lines: string[],
  width: number,
  inline: (text: string, base: Span) => Span[],
  base: Span,
): Span[][] {
  const head = cellsOf(lines[0] ?? "");
  const aligns = cellsOf(lines[1] ?? "").map(alignOf);
  const body = lines.slice(2).map(cellsOf);
  const columns = head.length;
  const cells = [head, ...body].map((row, r) =>
    Array.from({ length: columns }, (_, c) =>
      inline(row[c] ?? "", r === 0 ? { ...base, bold: true } : base),
    ),
  );
  const column = (c: number, measure: (cell: Span[]) => number) =>
    Math.max(minColumn, ...cells.map((row) => measure(row[c] ?? [])));
  const longestWord = (cell: Span[]) =>
    Math.max(
      0,
      ...cell
        .map((s) => s.text)
        .join("")
        .split(/\s+/)
        .map(widthOf),
    );
  const ideal = Array.from({ length: columns }, (_, c) => column(c, spanWidth));
  const least = Array.from({ length: columns }, (_, c) => column(c, longestWord));
  const room = Math.max(width - 3 * columns - 1 - margin, columns * minColumn);
  const widths = widthsOf(ideal, least, room);
  const rule = (left: string, mid: string, right: string): Span[] => [
    { ...base, text: `${left}${widths.map((w) => "─".repeat(w + 2)).join(mid)}${right}` },
  ];
  const rows: Span[][] = [rule("┌", "┬", "┐")];
  cells.forEach((row, r) => {
    if (r > 0) rows.push(rule("├", "┼", "┤"));
    const wrapped = row.map((cell, c) => wrapSpans(cell, widths[c] ?? 1));
    const height = Math.max(...wrapped.map((w) => w.length));
    for (let i = 0; i < height; i++) {
      const out: Span[] = [{ ...base, text: "│" }];
      wrapped.forEach((cell, c) => {
        const part = cell[i] ?? [];
        const free = Math.max((widths[c] ?? 1) - spanWidth(part), 0);
        const before =
          aligns[c] === "right" ? free : aligns[c] === "center" ? Math.floor(free / 2) : 0;
        out.push({ ...base, text: ` ${" ".repeat(before)}` }, ...part);
        out.push({ ...base, text: `${" ".repeat(free - before)} │` });
      });
      rows.push(out);
    }
  });
  rows.push(rule("└", "┴", "┘"));
  return rows;
}
