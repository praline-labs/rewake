import { expect, test } from "bun:test";
import type { FaceLine } from "./messages.ts";
import type { Item } from "./mirror.gen.ts";
import { FaceModel } from "./model.ts";
import { bodyOf, fill, pinned, render, statusOf } from "./render.ts";

const text = (lines: FaceLine[]) =>
  lines.map((l) =>
    l.spans
      .map((s) => s.text)
      .join("")
      .trimEnd(),
  );

const call = (id: string, command: string): Item => ({
  id,
  kind: "tool",
  status: "done",
  name: "Bash",
  turn: "t",
  text: JSON.stringify({ command }),
  output: "ok",
});

// Brief 57: a prompt sent while the session works waits in its queue; pi's
// screen shows it under the chat as `Steering: …` until it takes it, and the
// face does the same, then draws it where the session took it. While it
// waits it ends no run of calls.
test("a queued prompt waits under the conversation until it is taken", () => {
  const m = new FaceModel();
  m.apply({ op: "begin", items: 0, turn: "working", since: 0 });
  m.apply({ op: "end" });
  m.apply({ op: "add", item: call("a", "make") });
  m.apply({
    op: "add",
    item: { id: "q", kind: "prompt", status: "queued", turn: "t", text: "also\n  run the tests" },
  });
  m.apply({ op: "add", item: call("b", "make test") });
  const clock = { now: 1000, since: 0 };
  expect(text(render(m, 40, 100, 0, clock))).toEqual([
    "● Running 2 shell commands… (ctrl+o to",
    "  expand)",
    "  ⎿  $ make test",
    "",
    "Steering: also run the tests",
    "",
    "⠏ Working…",
    "",
  ]);
  // Cut to the pane with …, as pi cuts its own.
  expect(text(render(m, 20, 100, 0, clock)).at(-4)).toBe("Steering: also run …");
  m.apply({ op: "drop", id: "q", dropped: 0 });
  m.apply({
    op: "add",
    item: { id: "q", kind: "prompt", status: "settled", turn: "t", text: "also" },
  });
  expect(text(render(m, 50, 100, 0, clock))).toEqual([
    "  Ran 2 shell commands (ctrl+o to expand)",
    "",
    "❯ also",
    "",
    "⠏ Working…",
    "",
  ]);
});

// The owner, October 31, 2026: a short conversation starts at the pane's
// top and grows down, as the terminal programs start one.
test("a conversation shorter than the pane is drawn from its top", () => {
  const rows = fill([{ spans: [{ text: "❯ hi" }] }], 6, 3);
  expect(text(rows)).toEqual(["❯ hi", "", ""]);
  expect(rows.every((r) => r.spans.map((s) => s.text).join("").length === 6)).toBe(true);
});

// The owner, October 31, 2026: the turn's line is pinned to the bottom, as
// Claude Code keeps its own: on a short conversation it stands just above the
// blank row over the input, the gap above it empty; a long one leaves none.
test("the turn's line stays at the pane's bottom under a short conversation", () => {
  const m = new FaceModel();
  m.apply({ op: "begin", items: 0, turn: "idle", since: 0, took: 31000, at: 0 });
  m.apply({ op: "end" });
  m.apply({ op: "add", item: { id: "p", kind: "prompt", status: "done", turn: "t", text: "hi" } });
  const clock = { now: 0, worked: m.worked };
  const frame = (height: number) =>
    text(pinned(bodyOf(m, 40).rows, statusOf(m, clock, 40), height));
  const short = frame(8);
  expect(short).toHaveLength(8);
  expect(short[0]).toBe("❯ hi");
  expect(short.slice(1, 6)).toEqual(["", "", "", "", ""]);
  expect(short[6]).toStartWith("  Done in 31s");
  expect(short[7]).toBe("");
  const tall = frame(2);
  expect(tall).toHaveLength(4);
  expect(tall.slice(0, 2)).toEqual(["❯ hi", ""]);
  expect(tall[2]).toStartWith("  Done in 31s");
});
