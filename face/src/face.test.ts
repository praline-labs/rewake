// The face's glue in process (face.ts): what the shell sends and the mirror
// says, turned into the frames the face posts, on a clock the test moves.
// The face's own modules are tested apart; this is how face.ts ties them.
import { expect, test } from "bun:test";
import { epoch, faced, replies } from "./facekit.ts";

const fifty = replies(Array.from({ length: 50 }, (_, i) => `line ${i + 1}`));

test("a face draws the mirror's snapshot at its tick, every row filled to the pane", () => {
  const f = faced({ items: [{ id: "p", kind: "prompt", status: "settled", text: "say hi" }] });
  // A change only marks the face; the tick paints it.
  expect(f.posts).toEqual([]);
  f.tick();
  expect(f.frame().lines).toHaveLength(24);
  expect(f.shown().filter(Boolean)).toEqual(["❯ say hi"]);
  expect(f.frame()).toMatchObject({ link: "live", turn: "idle", prompts: true });
  // A tick that draws the same posts nothing.
  const posted = f.posts.length;
  f.tick();
  expect(f.posts).toHaveLength(posted);
});

// Seen live: the first notch from the bottom was counted against the tail
// the face had laid out, not the whole conversation, and jumped rows.
test("a notch of the wheel moves the face one row, from the bottom and on", () => {
  const f = faced({ items: fifty });
  f.tick();
  // Each reply is a row and a blank one, the last with its blank.
  expect(f.shown().at(-2)).toBe("● line 50");
  expect(f.shown().slice(0, 2)).toEqual(["● line 39", ""]);
  const notch = (up: boolean, at: number) => f.take({ kind: "wheel", up, at });
  notch(true, epoch);
  expect(f.shown()[0]).toBe("");
  notch(true, epoch + 1000);
  expect(f.shown()[0]).toBe("● line 38");
  // Down: the first notch back is held, as natively, the next one moves.
  notch(false, epoch + 2000);
  expect(f.shown()[0]).toBe("● line 38");
  notch(false, epoch + 3000);
  expect(f.shown()[0]).toBe("");
  f.take({ kind: "scroll", by: -100 });
  expect(f.shown()[0]).toBe("● line 39");
});

// Seen in review: a change that came after a tick was laid out and painted
// by a notch before the next tick, ahead of the face's one clock.
test("a notch cuts the rows the last tick painted; a change after it waits for its tick", () => {
  const f = faced({ items: fifty });
  f.tick();
  f.send({ op: "append", id: "r48", text: " NEW" });
  f.take({ kind: "wheel", up: true, at: epoch });
  expect(f.shown()[0]).toBe("");
  expect(f.shown()).toContain("● line 49");
  expect(f.posts.flatMap((p) => (p.kind === "lines" ? p.lines : []))).not.toContainEqual(
    expect.objectContaining({ spans: expect.arrayContaining([{ text: " NEW" }]) }),
  );
  f.tick();
  expect(f.shown()).toContain("● line 49 NEW");
});

// The owner, October 31, 2026: the turn's line is pinned to the pane's
// bottom, over the blank row above the input, under a short conversation too.
test("a face keeps the turn's line at its bottom under a short conversation", () => {
  const prompt = { id: "p", kind: "prompt", status: "settled", text: "say hi" };
  const f = faced({ items: [prompt], begin: { took: 31_000, at: epoch } });
  f.tick();
  const text = f.shown();
  expect(text[0]).toBe("❯ say hi");
  expect(text.slice(1, 22).every((l) => l === "")).toBe(true);
  expect(text[22]?.trim()).toStartWith("Done in 31s");
  expect(text[23]).toBe("");
});

// Review of fceb120: a face that connects mid-turn counted the turn in its
// snapshot as the one before, and said Waiting… with no tokens to the end.
test("a face that connects mid-turn shows the phase and the tokens of the turn it joined", () => {
  const reply = { id: "r", turn: "t1", kind: "text", status: "streaming", text: "x".repeat(400) };
  const f = faced({ items: [reply], begin: { turn: "working", since: epoch - 20_000 } });
  f.tick();
  const line = f.shown().find((l) => l.includes("Writing…")) ?? "";
  expect(line).toContain("tokens");
  // A working turn goes on ticking with no change.
  const posted = f.posts.length;
  f.hand.advance(1000);
  expect(f.posts.length).toBeGreaterThan(posted);
});

// The face selects and picks parts; what it posts is the text alone.
test("a drag copies the selection's text, a click the part's source, lit for a moment", () => {
  const f = faced({ items: replies(["first **reply**", "second"]) });
  f.tick();
  const first = f.rowOf("● first");
  const second = f.rowOf("● second");
  const copies = () => f.posts.flatMap((p) => (p.kind === "copy" ? [p] : []));
  const selected = () => f.frame().lines[first]?.spans.filter((s) => s.selected) ?? [];

  f.mouse("down", 2, first);
  f.mouse("drag", 9, first);
  expect(selected().map((s) => s.text)).toEqual(["first ", "re"]);
  f.mouse("up", 9, first);
  expect(copies()).toEqual([{ kind: "copy", text: "first re", what: "selection" }]);

  // A click on the other reply copies it as written, and drops the selection.
  f.mouse("down", 4, second);
  f.mouse("up", 4, second);
  expect(copies()[1]).toEqual({ kind: "copy", text: "second", what: "the reply" });
  expect(selected()).toEqual([]);
  expect(f.frame().lines[second]?.lit).toBe(true);
  // The light goes out by itself a second later.
  f.hand.advance(1000);
  expect(f.frame().lines.some((l) => l.lit)).toBe(false);

  // The blank under a short conversation is no part: a click there copies
  // nothing, and a pointer moved with no button lights nothing.
  f.mouse("down", 4, 23);
  f.mouse("up", 4, 23);
  f.take({ kind: "mouse", type: "move" as never, x: 4, y: first, focused: true });
  expect(copies()).toHaveLength(2);
  expect(f.frame().lines.some((l) => l.lit)).toBe(false);
  // A click on an unfocused face only focuses it.
  f.take({ kind: "mouse", type: "down", x: 4, y: first, focused: false });
  f.take({ kind: "mouse", type: "up", x: 4, y: first, focused: false });
  expect(copies()).toHaveLength(2);
});

test("a drag past the top scrolls on the edge's clock, the selection's end with it", () => {
  const f = faced({ items: fifty });
  f.tick();
  const row = f.rowOf("● line 41");
  f.mouse("down", 4, row);
  f.mouse("drag", 4, -1);
  expect(f.shown()[0]).toBe("● line 39");
  f.hand.advance(50);
  expect(f.shown()[0]).toBe("● line 38");
  f.mouse("up", 4, -1);
  f.hand.advance(500);
  expect(f.shown()[0]).toBe("● line 38");
  const copy = f.posts.findLast((p) => p.kind === "copy");
  expect(copy?.kind === "copy" && copy.text).toStartWith("line 38");
});

const output = Array.from({ length: 12 }, (_, i) => `line ${i + 1}`).join("\n");
const probe = { id: "c", kind: "tool", status: "done", name: "Probe", text: "{}", output };

// Review of 288cd7f: a selection is held by the rows' places, so rows opened
// above it moved its light onto a call; whatever opens or folds drops it.
test("ctrl+o opens and folds everything, and drops the selection", () => {
  const read = { ...probe, id: "r", name: "Read", text: JSON.stringify({ file_path: "/a" }) };
  const reply = { id: "t", kind: "text", status: "settled", text: "SELECT me" };
  const f = faced({ items: [probe, read, reply] }, 40, 30);
  f.tick();
  expect(f.shown()).toContain("     … +9 lines (ctrl+o to expand)");
  const selected = () => f.frame().lines.some((l) => l.spans.some((s) => s.selected));
  const row = f.rowOf("● SELECT me");
  f.mouse("down", 2, row);
  f.mouse("drag", 8, row);
  f.mouse("up", 8, row);
  expect(selected()).toBe(true);
  f.take({ kind: "unfold", all: true });
  f.tick();
  expect(f.shown()).toContain("     line 12");
  expect(f.shown()).toContain("● Read(/a)");
  expect(selected()).toBe(false);
  f.take({ kind: "unfold", all: false });
  f.tick();
  expect(f.shown()).toContain("     … +9 lines (ctrl+o to expand)");
});

// Seen live: ctrl+o in a view scrolled up moved what the person read.
test("ctrl+o in a view scrolled up keeps the first part it shows on its row", () => {
  const f = faced({ items: [probe, ...replies(["a", "b", "c", "d", "e", "f"]), probe] }, 40, 8);
  f.tick();
  f.take({ kind: "scroll", by: 6 });
  const before = f.shown();
  const y = before.findIndex((l) => l.startsWith("● "));
  f.take({ kind: "unfold", all: true });
  f.tick();
  expect(y).toBeGreaterThanOrEqual(0);
  // Nine rows opened above it; the view did not move.
  expect(f.shown()).toEqual(before);
  f.take({ kind: "scroll", by: -100 });
  expect(f.shown()).toContain("     line 12");
});
