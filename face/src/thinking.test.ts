import { expect, test } from "bun:test";
import type { FaceLine } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { linesOf } from "./render.ts";

const text = (lines: FaceLine[]) => lines.map((l) => l.spans.map((s) => s.text).join(""));

// Brief 54: pi's own screen draws the summary the provider streams as the
// thinking, in markdown, faint and italic; `∴ Thinking` stands for a block
// that sent no text.
test("a thinking block with text shows it faint and italic, a long one folds", () => {
  const block = (text: string): Item => ({ id: "th", kind: "thinking", status: "settled", text });
  const lines = linesOf(block("**Planning the edit**\n\nRead the file, then change it."), 40);
  expect(text(lines)).toEqual(["∴ Planning the edit", "  ", "  Read the file, then change it."]);
  expect(lines[0]?.spans[1]).toEqual({
    text: "Planning the edit",
    tone: "faint",
    italic: true,
    bold: true,
  });
  expect(lines[0]?.spans[0]).toMatchObject({ text: "∴ ", gutter: true });
  expect(text(linesOf(block(""), 40))).toEqual(["∴ Thinking"]);
  expect(text(linesOf(block("  \n"), 40))).toEqual(["∴ Thinking"]);
  const long = block(Array.from({ length: 30 }, (_, i) => `line ${i}`).join("\n\n"));
  const folded = text(linesOf(long, 40));
  expect(folded).toHaveLength(11);
  expect(folded[10]).toMatch(/^ {2}… \+\d+ lines \(ctrl\+o to expand\)$/);
});
