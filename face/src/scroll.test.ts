import { expect, test } from "bun:test";
import type { FaceLine } from "./messages.ts";
import { FaceModel } from "./model.ts";
import { rowsOf } from "./render.ts";
import { Scroll } from "./scroll.ts";

const text = (lines: FaceLine[]) => lines.map((l) => l.spans.map((s) => s.text).join(""));
const clock = { now: 0 };

// The person scrolls up while a reply streams in: the rows they look at stay,
// and back at the bottom the view follows the stream again.
test("the face follows new rows at the bottom and holds them scrolled up", () => {
  const m = new FaceModel();
  m.apply({ op: "begin", items: 0, turn: "idle" } as never);
  m.apply({ op: "end" } as never);
  m.apply({ op: "add", item: { id: "b", kind: "text", status: "streaming", text: "1" } } as never);
  const more = (from: number, to: number) => {
    for (let i = from; i <= to; i++) m.apply({ op: "append", id: "b", text: `\n${i}` } as never);
  };
  more(2, 10);
  const scroll = new Scroll();
  const shown = () => text(scroll.view(rowsOf(m, 20, clock), 3));
  expect(shown()).toEqual(["  9", "  10", ""]);
  more(11, 12);
  expect(shown()).toEqual(["  11", "  12", ""]);

  scroll.by(4, rowsOf(m, 20, clock).length, 3);
  expect(shown()).toEqual(["  7", "  8", "  9"]);
  more(13, 20);
  expect(shown()).toEqual(["  7", "  8", "  9"]);

  // Down past the bottom follows again, and goes on following.
  scroll.by(-100, rowsOf(m, 20, clock).length, 3);
  expect(scroll.top).toBeUndefined();
  more(21, 21);
  expect(shown()).toEqual(["  20", "  21", ""]);
  // Up past the top stops at the first row.
  scroll.by(100, rowsOf(m, 20, clock).length, 3);
  expect(shown()).toEqual(["● 1", "  2", "  3"]);
});
