// The shell takes from a face only what the contract shapes, within bounds
// (check.ts, from the shapes of face/src/messages.ts): a face from another
// checkout cannot make its thread throw, nor put anything of its own in the
// shell's tree.
import { expect, test } from "bun:test";
import { faultOf, taken } from "./check.ts";

const frame = (spans: unknown[], more: object = {}) => ({
  kind: "lines",
  lines: [{ spans, band: true, part: "r1" }],
  turn: "working",
  link: "live",
  prompts: true,
  mode: "auto",
  level: "high",
  ...more,
});

test("the messages a face posts pass as the contract shapes them", () => {
  const span = { text: "● ", tone: "done", bold: true, gutter: true, cell: 2, code: 0 };
  const lines = frame([span, { text: "hi", mark: "added", join: " " }]);
  expect(taken(lines) as unknown).toEqual({ message: lines });
  for (const message of [
    { kind: "error", text: "the face failed" },
    { kind: "copy", text: "x".repeat(1 << 20), what: "selection" },
    { kind: "answer", id: "p1", ok: false, detail: "refused" },
    { kind: "answer", id: "p1", ok: true },
    { kind: "pong", n: 3 },
  ])
    expect(taken(message) as unknown).toEqual({ message });
});

// A structured clone keeps an array's own properties and an object's fields
// the contract does not name: the copy has none of them, at any depth.
test("what reaches the shell is its own copy of the named fields alone", () => {
  const spans = Object.assign([{ text: "x", glow: 0.5, extra: 1 }], { slice: 0 });
  const lines = Object.assign([{ spans, extra: 2 }], { slice: 0 });
  const sent = { ...frame([]), lines, cycles: true, extra: 3 };
  const got = taken(sent);
  if (!("message" in got) || got.message.kind !== "lines") throw new Error("not taken");
  const message = got.message;
  expect(message as unknown).toEqual({ ...frame([]), lines: [{ spans: [{ text: "x" }] }] });
  expect(Object.keys(message.lines)).toEqual(["0"]);
  expect(Object.keys(message.lines[0]?.spans ?? {})).toEqual(["0"]);
  expect(message.lines).not.toBe(lines);
  expect(message.lines[0]?.spans).not.toBe(spans);
});

test("anything else names its fault", () => {
  const cases: [unknown, string][] = [
    [null, "a message that is no object"],
    [[], "a message that is no object"],
    [{ kind: "draw" }, "a message of no kind the shell knows"],
    [{ kind: "toString" }, "a message of no kind the shell knows"],
    [frame([{ text: "x", tone: "pink" }]), "a span's tone"],
    [frame([{ text: "x".repeat(20_000) }]), "a span's text"],
    [frame([{ tone: "done" }]), "a span's text"],
    [frame([{ text: "x", bold: "yes" }]), "a span's bold"],
    [frame([{ text: "x", cell: -1 }]), "a span's cell"],
    [frame([{ text: "x", code: 1.5 }]), "a span's code"],
    [frame(["x"]), "a span that is no object"],
    [{ ...frame([]), lines: [{ spans: {} }] }, "a row's spans"],
    [{ ...frame([]), lines: [{ spans: [], band: 1 }] }, "a row's band"],
    [frame([], { link: "gone" }), "the lines message's link"],
    [frame([], { mode: "turbo" }), "the lines message's mode"],
    [frame([], { level: "extreme" }), "the lines message's level"],
    [frame([], { prompts: undefined }), "the lines message's prompts"],
    [{ ...frame([]), lines: Array(3000).fill({ spans: [] }) }, "the lines message's lines"],
    [{ kind: "copy", text: "x".repeat(5 << 20), what: "selection" }, "the copy message's text"],
    [{ kind: "answer", id: "p1" }, "the answer message's ok"],
    [{ kind: "pong", n: 1.5 }, "the pong message's n"],
  ];
  for (const [message, fault] of cases)
    expect([message, faultOf(message)]).toEqual([message, fault]);
});
