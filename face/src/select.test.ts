// What the person copies out of the face (docs/face.md, "Copying"): the text
// a selection holds, not its drawing, and the part a click is on.
import { expect, test } from "bun:test";
import type { FaceLine } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { partOf } from "./parts.ts";
import { Pointer, pointAt } from "./pointer.ts";
import { bodyOf, linesOf } from "./render.ts";
import { copyOf, highlighted } from "./select.ts";
import { toggles } from "./unfold.ts";

const drawn = (lines: FaceLine[]) => lines.map((l) => l.spans.map((s) => s.text).join(""));

// all is a selection over every row, from the first cell to past the last.
const all = (rows: FaceLine[]) => ({
  anchor: { row: 0, col: 0 },
  focus: { row: rows.length - 1, col: 1000 },
});

const reply = (text: string): Item => ({ id: "r", kind: "text", status: "settled", text });

test("a selection copies a reply's text: no gutter, a wrapped line joined back, a list as written", () => {
  const rows = linesOf(reply("one two three four five\n\n- an item that wraps\n> said"), 14);
  expect(drawn(rows)).toEqual([
    "● one two",
    "  three four",
    "  five",
    "  ",
    "  - an item",
    "    that wraps",
    "  ▎ said",
  ]);
  expect(copyOf(rows, all(rows))).toBe("one two three four five\n\n- an item that wraps\nsaid");
  // From the middle of a row to the middle of another.
  expect(copyOf(rows, { anchor: { row: 0, col: 6 }, focus: { row: 1, col: 6 } })).toBe("two three");
  // Backwards is the same selection.
  expect(copyOf(rows, { anchor: { row: 1, col: 6 }, focus: { row: 0, col: 6 } })).toBe("two three");
});

test("a selection over an edit copies its lines, without the signs or the padding", () => {
  const edit: Item = {
    id: "e",
    kind: "tool",
    status: "done",
    name: "Edit",
    text: JSON.stringify({ file_path: "/x.ts", old_string: "  a = 1", new_string: "  a = 2" }),
    output: "updated",
  };
  const rows = linesOf(edit, 40);
  expect(drawn(rows)[2]).toBe(`     -  a = 1${" ".repeat(27)}`);
  expect(copyOf(rows, all(rows))).toBe(
    "Update(/x.ts)\nAdded 1 line, removed 1 line\n  a = 1\n  a = 2",
  );
});

test("a selection over a table copies each row's cells by their text, wrapped cells whole", () => {
  const rows = linesOf(reply("| name | what |\n|---|---|\n| a | one two three |\n| b | x |"), 22);
  expect(drawn(rows)).toContain("  │      │ three   │");
  expect(copyOf(rows, all(rows))).toBe("name\twhat\na\tone two three\nb\tx");
});

test("a highlight marks the text it covers and leaves the gutter as drawn", () => {
  const rows = linesOf(reply("abc def"), 20);
  const [row] = highlighted(rows, 0, { anchor: { row: 0, col: 0 }, focus: { row: 0, col: 3 } });
  expect(row?.spans.map((s) => [s.text, s.selected ?? false])).toEqual([
    ["● ", false],
    ["ab", true],
    ["c def", false],
  ]);
});

test("a selection lights the blank rows inside it and copies them", () => {
  const rows = linesOf(reply("one\n\ntwo\n\n```\na\n\nb\n```"), 20);
  expect(drawn(rows)).toEqual(["● one", "  ", "  two", "  ", "  a", "  ", "  b"]);
  const lit = highlighted(rows, 0, all(rows));
  // Every row, a blank one included, holds a lit cell.
  expect(lit.map((l) => l.spans.some((s) => s.selected))).toEqual(rows.map(() => true));
  expect(copyOf(rows, all(rows))).toBe("one\n\ntwo\n\na\n\nb");
  // A selection that ends before a blank row does not light it.
  const part = highlighted(rows, 0, { anchor: { row: 0, col: 0 }, focus: { row: 0, col: 9 } });
  expect(part[1]?.spans.some((s) => s.selected)).toBe(false);
  // Across two items the blank row between them is lit and copied.
  const body = bodyOf(model(reply("one"), reply("two")), 20).rows;
  expect(drawn(body)).toEqual(["● one", "", "● two"]);
  expect(highlighted(body, 0, all(body))[1]?.spans.some((s) => s.selected)).toBe(true);
  expect(copyOf(body, all(body))).toBe("one\n\ntwo");
});

const model = (...items: Item[]) => {
  const m = new FaceModel();
  m.apply({ op: "begin", items: items.length, turn: "idle" });
  for (const item of items) m.apply({ op: "item", item });
  m.apply({ op: "end" });
  return m;
};

// clicked is what a click on the row drawn as `row` copies.
// keyOf is the part of the drawn row, clicked what a click on it copies: a
// row that opens or closes copies nothing, as the worker takes it.
function keyOf(items: Item[], row: string, width = 40) {
  const rows = bodyOf(model(...items), width).rows;
  const line = rows.find((l) => drawn([l])[0]?.trimEnd() === row);
  if (!line) throw new Error(`no row ${row} in ${JSON.stringify(drawn(rows))}`);
  return line.part;
}

function clicked(items: Item[], row: string, width = 40) {
  const key = keyOf(items, row, width);
  return key === undefined || toggles(key) ? undefined : partOf(items, key);
}

test("a click copies the whole part it is on", () => {
  const source = "Run it:\n\n```sh\nmake all\n```\n\n**done**";
  expect(clicked([reply(source)], "  done")).toEqual({
    text: source,
    what: "the reply",
  });
  expect(clicked([reply(source)], "  make all")).toEqual({
    text: "make all",
    what: "the code block",
  });
  const bash: Item = {
    id: "b",
    kind: "tool",
    status: "done",
    name: "Bash",
    text: JSON.stringify({ command: "make all" }),
    output: "1\n2\n3\n4\n5\n6",
  };
  const shown = reply("ok");
  // A run's line and a cut row open and close rather than copy (unfold.ts).
  expect(keyOf([bash, shown], "  Ran 1 shell command")).toBe("fold b");
  expect(clicked([bash, shown], "  Ran 1 shell command")).toBeUndefined();
  // Brief 56: a thinking summary's rows copy its text, as written.
  const thought: Item = { id: "th", kind: "thinking", status: "settled", text: "**Plan** it" };
  expect(clicked([thought], "∴ Plan it")).toEqual({ text: "**Plan** it", what: "the thinking" });
  const edit: Item = {
    id: "e",
    kind: "tool",
    status: "done",
    name: "Edit",
    text: JSON.stringify({ file_path: "/x", old_string: "old", new_string: "new" }),
    output: "The file was updated",
  };
  expect(clicked([edit], "● Update(/x)")).toEqual({ text: "/x", what: "the call's input" });
  expect(clicked([edit], "     -old")).toEqual({ text: "old", what: "the old lines" });
  expect(clicked([edit], "     +new")).toEqual({ text: "new", what: "the new lines" });
  const mcp: Item = {
    id: "m",
    kind: "tool",
    status: "done",
    name: "mcp__s__t",
    text: JSON.stringify({ q: 1 }),
    output: "a\nb\nc\nd\ne\nf",
  };
  expect(keyOf([mcp], "     … +3 lines")).toBe("more m");
  expect(clicked([mcp], "  ⎿  a")).toEqual({
    text: "a\nb\nc\nd\ne\nf",
    what: "the call's output",
  });
  expect(clicked([mcp], "● s - t (MCP)(q: 1)")?.text).toBe('{\n  "q": 1\n}');
  // An open run's latest call: a click on what it works on copies its command.
  const running: Item = { ...bash, status: "running", output: undefined };
  const m = model(running);
  m.turn = "working";
  const row = bodyOf(m, 40).rows.find((l) => l.part?.startsWith("input"));
  expect(drawn([row ?? { spans: [] }])[0]).toBe("  ⎿  $ make all");
  expect(partOf([running], row?.part ?? "")).toEqual({ text: "make all", what: "the command" });
});

// A press and its release on one cell is a click; any motion to another cell
// first makes it a drag, whose release copies the selection.
test("a click is told from a drag by the cell the pointer leaves", () => {
  const p = new Pointer();
  const view = { top: 10, shown: 5, height: 8, width: 20 };
  p.down(view, 4, 4, true);
  expect(p.drag(pointAt(4, 4, view), 4, 4)).toBe(false);
  expect(p.up()).toEqual({ clicked: 11 });
  p.down(view, 4, 4, true);
  expect(p.drag(pointAt(5, 4, view), 5, 4)).toBe(true);
  expect(p.drag(pointAt(4, 4, view), 4, 4)).toBe(true);
  expect(p.up()).toEqual({
    selected: { anchor: { row: 11, col: 4 }, focus: { row: 11, col: 4 } },
  });
  // A click on a face not yet focused only focuses it.
  p.down(view, 4, 4, false);
  expect(p.up()).toBeUndefined();
  // A click on the blank above a short conversation is on no row, though a
  // drag from there starts at its first.
  p.down(view, 4, 1, true);
  expect(p.up()).toBeUndefined();
  // Past the view's edges a drag reaches the first row's start or the last
  // row's end; above a short conversation, its first row.
  expect(pointAt(3, -2, view)).toEqual({ row: 10, col: 0 });
  expect(pointAt(3, 9, view)).toEqual({ row: 14, col: 20 });
  expect(pointAt(3, 1, view)).toEqual({ row: 10, col: 0 });
});
