// What the person opens (docs/face.md, "Unfolding"): the shell's key opens
// everything, a click one run or one cut part, and the row clicked stays
// where it stood.
import { expect, test } from "bun:test";
import type { FaceLine } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { bodyOf } from "./render.ts";
import { anchorAt, topFor, Unfold } from "./unfold.ts";

const text = (lines: FaceLine[]) => lines.map((l) => l.spans.map((s) => s.text).join(""));

const model = (...items: Item[]) => {
  const m = new FaceModel();
  m.apply({ op: "begin", items: items.length, turn: "idle" });
  for (const item of items) m.apply({ op: "item", item });
  m.apply({ op: "end" });
  return m;
};

const call = (id: string, name: string, input: object, output: string, status = "done"): Item => ({
  id,
  kind: "tool",
  status: status as Item["status"],
  name,
  text: JSON.stringify(input),
  output,
});

const lines = (n: number) => Array.from({ length: n }, (_, i) => `line ${i + 1}`).join("\n");

const run = model(
  call("r", "Read", { file_path: "/a" }, "1\tx"),
  call("b", "Bash", { command: "make" }, "no rule to make", "error"),
  { id: "t", kind: "text", status: "settled", text: "done" },
  call("o", "Probe", {}, lines(8)),
);

// Brief 56: what pi's screen and the terminal program open by ctrl+o, the
// face opens by the shell's key: a run call by call with its failure, and an
// output whole, with nothing left to count.
test("everything opened shows each call of a run and every output whole", () => {
  const view = new Unfold();
  expect(text(bodyOf(run, 40, Infinity, view).rows)).toEqual([
    "  Read 1 file, ran 1 shell command, 1",
    "  failed",
    "",
    "● done",
    "",
    "● Probe",
    "  ⎿  line 1",
    "     line 2",
    "     line 3",
    "     … +5 lines",
  ]);
  // The failed calls are counted in red, as the terminal program counts them.
  expect(bodyOf(run, 80, Infinity, view).rows[0]?.spans.slice(-2)).toEqual([
    { text: "1", tone: "error", bold: true },
    { text: " failed", tone: "error" },
  ]);
  view.every(true);
  expect(text(bodyOf(run, 40, Infinity, view).rows)).toEqual([
    "  Read 1 file, ran 1 shell command, 1",
    "  failed",
    "",
    "● Read(/a)",
    "  ⎿  1  x",
    "",
    "● Bash(make)",
    "  ⎿  Error: no rule to make",
    "",
    "● done",
    "",
    "● Probe",
    ...lines(8)
      .split("\n")
      .map((l, i) => (i === 0 ? `  ⎿  ${l}` : `     ${l}`)),
  ]);
});

// A click opens one part and counts what it opened, so a second click on
// the same row closes it; with everything open a click closes one.
test("a click opens or closes one part against what the key opened", () => {
  const view = new Unfold();
  view.toggle("more o");
  const opened = text(bodyOf(run, 40, Infinity, view).rows);
  expect(opened.slice(-2)).toEqual(["     line 8", "     … −5 lines"]);
  expect(bodyOf(run, 40, Infinity, view).rows.at(-1)?.part).toBe("more o");
  view.toggle("fold r");
  expect(text(bodyOf(run, 40, Infinity, view).rows)[3]).toBe("● Read(/a)");
  view.every(true);
  view.toggle("more o");
  expect(text(bodyOf(run, 40, Infinity, view).rows).slice(-2)).toEqual([
    "     line 3",
    "     … +5 lines",
  ]);
  // The key again forgets the clicks.
  view.every(false);
  expect(view.open("more o")).toBe(false);
});

// The row clicked keeps its row in the view, whatever opened above or
// under it.
test("the row a click opens stays where it stood in the view", () => {
  const view = new Unfold();
  const before = bodyOf(run, 40, Infinity, view).rows;
  const row = before.findIndex((l) => l.part === "more o");
  const anchor = anchorAt(before, row, row - 2);
  expect(anchor).toEqual({ key: "more o", k: 0, y: 2 });
  // The run above opens too, and the output grows under the row's place.
  view.toggle("fold r");
  view.toggle("more o");
  const after = bodyOf(run, 40, Infinity, view).rows;
  const top = anchor ? topFor(after, anchor) : undefined;
  expect(text(after.slice(top, (top ?? 0) + 3))).toEqual([
    "     line 7",
    "     line 8",
    "     … −5 lines",
  ]);
  // A part gone leaves the view where it was.
  expect(topFor(after, { key: "more gone", k: 0, y: 0 })).toBeUndefined();
});
