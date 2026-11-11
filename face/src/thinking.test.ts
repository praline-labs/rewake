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
  expect(lines[0]?.spans[1]).toEqual({ text: "Planning the edit", tone: "faint", italic: true });
  expect(lines[0]?.spans[0]).toMatchObject({ text: "∴ ", gutter: true });
  expect(text(linesOf(block(""), 40))).toEqual(["∴ Thinking"]);
  expect(text(linesOf(block("  \n"), 40))).toEqual(["∴ Thinking"]);
  const long = block(Array.from({ length: 30 }, (_, i) => `line ${i}`).join("\n\n"));
  const folded = text(linesOf(long, 40));
  expect(folded).toHaveLength(11);
  expect(folded[10]).toMatch(/^ {2}… \+\d+ lines \(ctrl\+o to expand\)$/);
});

// Brief 117: thinking is one faint stream. The markdown still lays it out —
// headings, inline code and bold keep their rows — but no span takes another
// colour, bold or underline from it.
test("thinking keeps its layout but every span is one faint tone", () => {
  const block: Item = {
    id: "th",
    kind: "thinking",
    status: "settled",
    text: "# Top\n\n**bold** `code` plain\n- an item",
  };
  const lines = linesOf(block, 40);
  expect(text(lines)).toEqual(["∴ Top", "  ", "  bold code plain", "  - an item"]);
  for (const span of lines.flatMap((line) => line.spans).filter((s) => !s.gutter)) {
    expect(span.tone).toBe("faint");
    expect(span.bold).toBeUndefined();
    expect(span.underline).toBeUndefined();
    expect(span.code).toBeUndefined();
  }
  // Italic stays: the whole block is one faint italic presentation.
  for (const span of lines.flatMap((line) => line.spans))
    if (!span.gutter) expect(span.italic).toBe(true);
});

// The control: an ordinary reply of the same words keeps its markdown styles.
test("a reply keeps its code and bold styles", () => {
  const reply: Item = {
    id: "t",
    kind: "text",
    status: "settled",
    text: "# Top\n\n`render.ts` is **needed**",
  };
  const styles = linesOf(reply, 40).flatMap((line) => line.spans);
  expect(styles.find((s) => s.text === "render.ts")?.tone).toBe("code");
  expect(styles.find((s) => s.text === "needed")?.bold).toBe(true);
  expect(styles.find((s) => s.text === "Top")).toMatchObject({ bold: true, underline: true });
});
