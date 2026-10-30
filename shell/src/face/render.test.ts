import { expect, test } from "bun:test";
import type { FaceLine } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { fill, linesOf, render, turnLine } from "./render.ts";
import { wrap } from "./wrap.ts";

const text = (lines: FaceLine[]) => lines.map((l) => l.spans.map((s) => s.text).join(""));

const model = (...items: Item[]) => {
  const m = new FaceModel();
  m.apply({ op: "begin", items: items.length, turn: "idle" });
  for (const item of items) m.apply({ op: "item", item });
  m.apply({ op: "end" });
  return m;
};

// The face draws the model's tail: a streamed block as it grows, a lost one
// marked, and scrolling back shows what is above.
test("the face draws the tail of the model, marks a gap, and scrolls back", () => {
  const m = new FaceModel();
  for (const line of [
    { op: "hello", v: 1 },
    { op: "begin", items: 1, turn: "idle" },
    { op: "item", item: { id: "p", kind: "prompt", status: "settled", text: "count" } },
    { op: "end" },
    { op: "add", item: { id: "b", kind: "text", status: "streaming", text: "1\n2" } },
    { op: "append", id: "b", text: "\n3" },
    { op: "set", item: { id: "b", kind: "text", status: "gap", text: "1\n2\n3" } },
  ] as const)
    m.apply(line as never);
  expect(text(render(m, 40, 100))).toEqual([
    "❯ count",
    "",
    "● 1",
    "  2",
    "  3",
    "  [lost on the way — the native face has it]",
  ]);
  expect(text(render(m, 40, 2))).toEqual(["  3", "  [lost on the way — the native face has it]"]);
  expect(text(render(m, 40, 2, 3))).toEqual(["", "● 1"]);
});

// Seen live on 2.11.0: a reply broke inside a word ("fil" / "es").
test("a text breaks at spaces and cuts only a word wider than the row", () => {
  expect(wrap("how many files are there", 12)).toEqual(["how many", "files are", "there"]);
  expect(wrap("abcdefghij kl", 4)).toEqual(["abcd", "efgh", "ij", "kl"]);
  expect(wrap("ab界cd", 3)).toEqual(["ab", "界c", "d"]);
  const reply: Item = { id: "t", kind: "text", status: "settled", text: "a **bold** word `code`" };
  const [row] = linesOf(reply, 12);
  expect(row?.spans.find((s) => s.text === "bold")?.bold).toBe(true);
  expect(text(linesOf(reply, 12))).toEqual(["● a bold", "  word code"]);
});

// Seen live on 2.11.0: a letter showed as the notification's XML.
test("a letter rewake delivered shows its summary, not the notification", () => {
  const letter: Item = {
    id: "l",
    kind: "prompt",
    status: "settled",
    origin: "peer",
    text: "<task-notification>\n<task-id>rewake-1</task-id>\n<status>completed</status>\n<summary>Rewake: main task, 1 new message\n  ↳ Do it (a preview only: run rewake inbox)</summary>\n</task-notification>",
  };
  const lines = linesOf(letter, 80);
  expect(text(lines)).toEqual([
    "● Rewake: main task, 1 new message ↳ Do it (a preview only: run rewake inbox)",
  ]);
  expect(lines[0]?.band).toBeUndefined();
});

// Seen live on 2.11.0: a call showed its input as JSON.
test("a tool's head is its name and main argument, its output folded", () => {
  const call = (name: string, input: string, output?: string): Item => ({
    id: name,
    kind: "tool",
    status: "done",
    name,
    text: input,
    output,
  });
  const bash = call(
    "Bash",
    '{"command":"ls docs | head -5","description":"list"}',
    "a\nb\nc\nd\ne",
  );
  expect(text(linesOf(bash, 40))).toEqual([
    "● Bash(ls docs | head -5)",
    "  ⎿  a",
    "     b",
    "     c",
    "     … +2 lines",
  ]);
  expect(linesOf(bash, 40)[0]?.spans[1]).toEqual({ text: "Bash", bold: true });
  expect(text(linesOf(call("Read", '{"file_path":"/a/b.ts"}'), 40))).toEqual(["● Read(/a/b.ts)"]);
  expect(text(linesOf(call("Probe", '{"n":1}'), 40))).toEqual(['● Probe({"n":1})']);
  // An input cut to its bound no longer reads as JSON and shows as it is.
  expect(text(linesOf(call("Bash", '{"command":"l'), 40))).toEqual(['● Bash({"command":"l)']);
});

// Seen live on 2.11.0: the native face showed through the pane's last column.
test("every row the face draws covers the pane's width", () => {
  const m = model(
    { id: "p", kind: "prompt", status: "settled", text: "a prompt" },
    { id: "t", kind: "text", status: "settled", text: "界".repeat(30) },
    {
      id: "o",
      kind: "tool",
      status: "error",
      name: "Bash",
      text: "{}",
      output: "\x1b[31mred\x1b[0m\tx",
    },
  );
  for (const width of [9, 10, 23]) {
    const rows = fill(render(m, width, 30), width, 30);
    expect(rows).toHaveLength(30);
    for (const row of rows)
      expect(Bun.stringWidth(row.spans.map((s) => s.text).join(""))).toBe(width);
  }
});

test("the turn's line says it works, and how long the last one took", () => {
  const said = (line: FaceLine | undefined) => text(line ? [line] : []);
  expect(said(turnLine("working", { now: 75_000, since: 3_000 }))).toEqual(["✻ Working… (1m 12s)"]);
  const at = new Date(2026, 9, 30, 19, 5).getTime();
  expect(said(turnLine("idle", { now: 0, worked: { took: 37_400, at } }))).toEqual([
    "✻ Worked for 37s · done 7:05 PM",
  ]);
  expect(turnLine("idle", { now: 0 })).toBeUndefined();
});
