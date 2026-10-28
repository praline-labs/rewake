import { expect, test } from "bun:test";
import { FaceModel } from "./model.ts";
import { render, wrap } from "./render.ts";

// The face draws the model's tail: a streamed block as it grows, a lost one
// marked, and scrolling back shows what is above.
test("the face draws the tail of the model, marks a gap, and scrolls back", () => {
  const m = new FaceModel();
  for (const line of [
    { op: "hello", v: 1 },
    { op: "begin", items: 1, turn: "working" },
    { op: "item", item: { id: "p", kind: "prompt", status: "settled", text: "count" } },
    { op: "end" },
    { op: "add", item: { id: "b", kind: "text", status: "streaming", text: "1\n2" } },
    { op: "append", id: "b", text: "\n3" },
    { op: "set", item: { id: "b", kind: "text", status: "gap", text: "1\n2\n3" } },
  ] as const)
    m.apply(line as never);
  const all = render(m, 40, 100).map((l) => l.text);
  expect(all).toEqual([
    "› count",
    "",
    "  1",
    "  2",
    "  3",
    "  [lost on the way — the native face has it]",
  ]);
  expect(render(m, 40, 2).map((l) => l.text)).toEqual([
    "  3",
    "  [lost on the way — the native face has it]",
  ]);
  expect(render(m, 40, 2, 3).map((l) => l.text)).toEqual(["", "  1"]);
  expect(wrap("ab界cd", 3)).toEqual(["ab", "界c", "d"]);
});
