// The marks rows hang under (docs/face.md, "The look"): a mark before the
// first row and every other row hung under the text at its width.
import type { FaceLine, Span } from "./messages.ts";
import { widthOf, wrapSpans } from "./wrap.ts";

// marked puts a mark before the first row and an indent of its width before
// the others, both drawing, not text.
export function marked(rows: Span[][], mark: Span, band?: boolean): FaceLine[] {
  const indent = { text: " ".repeat(widthOf(mark.text)), gutter: true };
  const first = { ...mark, gutter: true };
  return rows.map((row, i) => ({ spans: [i === 0 ? first : indent, ...row], band }));
}

// hung puts a mark before the first row and hangs every other row, a wrapped
// one or one after a newline, under the text at the mark's width.
export const hung = (
  text: string,
  width: number,
  mark: Span,
  base: Span,
  band?: boolean,
): FaceLine[] =>
  marked(
    text
      .split("\n")
      .flatMap((line) => wrapSpans([{ ...base, text: line }], width - widthOf(mark.text))),
    mark,
    band,
  );
