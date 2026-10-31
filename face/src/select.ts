// What a selection over the face holds and copies (docs/face.md, "Copying"):
// the text, not the drawing. A gutter — a mark, an indent, a diff's sign, a
// table's lines — is neither highlighted nor copied; a row broken to fit the
// pane joins the row above with what the break swallowed; a table's row is
// copied by its cells' text, a tab between them; and a logical line loses its
// trailing blanks, as the terminal program copies its own.
import type { FaceLine, Span } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { widthOf } from "./wrap.ts";

// PartKind is what a click on a row copies whole (parts.ts): a reply, its
// n-th code block, a thinking block's text, a call's input, a call's output,
// an edit's old or new lines, a written file's lines. Two kinds copy nothing
// and open or close instead (unfold.ts): a run's line, "fold", and the row
// that says how many lines are cut, "more".
export type PartKind =
  | "reply"
  | `code${number}`
  | "thinking"
  | "input"
  | "output"
  | "old"
  | "new"
  | "content"
  | "fold"
  | "more";

// partKey names a part by its kind and its item's id, a run's by its first
// call's.
export const partKey = (kind: PartKind, item: Item) => `${kind} ${item.id}`;

// Point is a cell: its row counted from the conversation's first, its column
// from the pane's left edge.
export type Point = { row: number; col: number };

export type Selection = { anchor: Point; focus: Point };

const before = (a: Point, b: Point) => a.row < b.row || (a.row === b.row && a.col <= b.col);

// ordered is a selection's first and last cell.
export const ordered = (s: Selection): [Point, Point] =>
  before(s.anchor, s.focus) ? [s.anchor, s.focus] : [s.focus, s.anchor];

const graphemes = new Intl.Segmenter();

// pieces is a span cut into its characters, each with its column.
function* pieces(spans: Span[]): Generator<{ span: Span; i: number; text: string; col: number }> {
  let col = 0;
  for (const [i, span] of spans.entries())
    for (const { segment } of graphemes.segment(span.text)) {
      yield { span, i, text: segment, col };
      col += widthOf(segment);
    }
}

// columns is what a selection takes of one row: from a column to one, both in.
function columns(row: number, s: Selection): [number, number] | undefined {
  const [first, last] = ordered(s);
  if (row < first.row || row > last.row) return undefined;
  return [row === first.row ? first.col : 0, row === last.row ? last.col : Infinity];
}

const isText = (span: Span) => !span.gutter;

// textOf is a row's text within the columns, its gutters left out.
function textOf(line: FaceLine, from: number, to: number): string {
  let text = "";
  for (const p of pieces(line.spans))
    if (isText(p.span) && p.col >= from && p.col <= to) text += p.text;
  return text;
}

// drawingOnly is a row of lines and marks with no text, a table's rule: it
// adds nothing to a copy, where an empty row adds an empty line.
const drawingOnly = (line: FaceLine) =>
  line.spans.every((s) => s.gutter) && line.spans.some((s) => s.text.trim() !== "");

// copyOf is the text a selection over the rows holds.
export function copyOf(rows: FaceLine[], s: Selection): string {
  const [first, last] = ordered(s);
  const lines: string[] = [];
  // A table's row drawn over several rows is gathered first: its cells,
  // each wrapped in its own column.
  let cells: string[] | undefined;
  const flush = () => {
    if (cells) lines.push(Array.from(cells, (c) => c ?? "").join("\t"));
    cells = undefined;
  };
  for (let r = Math.max(first.row, 0); r <= Math.min(last.row, rows.length - 1); r++) {
    const line = rows[r];
    if (!line) continue;
    if (line.spans.some((span) => span.cell !== undefined)) {
      cells ??= [];
      for (const span of line.spans)
        if (span.cell !== undefined)
          cells[span.cell] =
            `${cells[span.cell] ?? ""}${cells[span.cell] ? (span.join ?? "") : ""}${span.text}`;
      continue;
    }
    flush();
    if (drawingOnly(line)) continue;
    const [from, to] = columns(r, s) ?? [0, Infinity];
    const text = textOf(line, from, to);
    const join = line.spans.find((span) => span.join !== undefined)?.join;
    const last = lines.length - 1;
    if (join !== undefined && r > first.row && last >= 0) lines[last] += join + text;
    else lines.push(text);
  }
  flush();
  return lines.map((line) => line.trimEnd()).join("\n");
}

// highlighted is the rows shown, the first of them the conversation's row
// top, with the selection's cells marked: a span cut where the selection
// begins or ends, a gutter left as drawn.
export function highlighted(rows: FaceLine[], top: number, s: Selection | undefined): FaceLine[] {
  if (!s) return rows;
  return rows.map((line, n) => {
    const range = columns(top + n, s);
    if (!range) return line;
    const [from, to] = range;
    const spans: Span[] = [];
    let at = -1;
    // A blank row has no text cell to mark: the selection that holds its place
    // lights one cell there, as a blank row between paragraphs is lit natively.
    if (!drawingOnly(line) && !line.spans.some((span) => isText(span) && span.text !== "")) {
      const edge = line.spans.reduce((w, span) => w + widthOf(span.text), 0);
      return from <= edge && edge <= to
        ? { ...line, spans: [...line.spans, { text: " ", selected: true }] }
        : line;
    }
    for (const p of pieces(line.spans)) {
      const selected = isText(p.span) && p.col >= from && p.col <= to;
      const last = spans[spans.length - 1];
      if (last && at === p.i && (last.selected ?? false) === selected) last.text += p.text;
      else spans.push({ ...p.span, text: p.text, selected: selected || undefined });
      at = p.i;
    }
    return { ...line, spans };
  });
}

// lit is the rows shown with those of one part marked, the part a click
// just copied.
export const lit = (rows: FaceLine[], part: string | undefined): FaceLine[] =>
  part === undefined
    ? rows
    : rows.map((line) => (line.part === part ? { ...line, lit: true } : line));
