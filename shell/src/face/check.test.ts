// The shell takes from a face only what the contract shapes, within bounds
// (check.ts): a face from another checkout cannot make its thread throw.
import { expect, test } from "bun:test";
import { faultOf } from "./check.ts";

const frame = (spans: object[], more: object = {}) => ({
  kind: "lines",
  lines: [{ spans, band: true, part: "r1" }],
  turn: "working",
  link: "live",
  prompts: true,
  mode: "auto",
  ...more,
});

test("the messages a face posts pass as the contract shapes them", () => {
  const span = { text: "● ", tone: "done", bold: true, glow: 0.5, gutter: true, cell: 2, code: 0 };
  expect(faultOf(frame([span, { text: "hi", mark: "added", join: " " }]))).toBeUndefined();
  expect(faultOf({ kind: "error", text: "the face failed" })).toBeUndefined();
  expect(faultOf({ kind: "copy", text: "x".repeat(1 << 20), what: "selection" })).toBeUndefined();
  expect(faultOf({ kind: "answer", id: "p1", ok: false, detail: "refused" })).toBeUndefined();
  expect(faultOf({ kind: "pong", n: 3 })).toBeUndefined();
});

test("anything else names its fault", () => {
  const cases: [unknown, string][] = [
    [null, "a message that is no object"],
    [{ kind: "draw" }, "a message of no kind the shell knows"],
    [frame([{ text: "x", tone: "pink" }]), "a span's tone"],
    [frame([{ text: "x".repeat(20_000) }]), "a span's text"],
    [frame([{ text: "x", glow: 2 }]), "a span's glow"],
    [frame([{ text: "x", bold: "yes" }]), "a span's bold"],
    [frame([{ text: "x", cell: -1 }]), "a span's cell or code"],
    [frame([], { link: "gone" }), "its link"],
    [frame([], { mode: "turbo" }), "its mode"],
    [{ ...frame([]), lines: Array(3000).fill({ spans: [] }) }, "its rows"],
    [{ kind: "copy", text: "x".repeat(5 << 20), what: "selection" }, "a copy"],
    [{ kind: "answer", id: "p1" }, "an answer"],
    [{ kind: "pong", n: 1.5 }, "a pong"],
  ];
  for (const [message, fault] of cases) expect(faultOf(message)).toBe(fault);
});
