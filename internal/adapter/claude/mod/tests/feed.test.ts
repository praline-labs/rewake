// The mirror's feed through its interface: events in, batches, lost and
// resync markers and cut texts out.
import { expect, test } from "claude-code/testing";
import {
  bounds,
  type Carrier,
  fit,
  jsonSize,
  type Marker,
  type MirrorEvent,
  Outbox,
  pieces,
  settles,
  textBudget,
  utf8Size,
} from "../hooks/feed.ts";

const wire = (value: unknown) => new TextEncoder().encode(JSON.stringify(value)).length;

// A text of every kind of character the size counts apart: escaped, a
// control character, two and four UTF-8 bytes.
const mixed = `HEAD${'\u0001"é😀'.repeat(20000)}TAIL`;

test("sizes are the UTF-8 bytes JSON.stringify writes", () => {
  for (const text of ["", "plain", mixed, "\ud800 lone", "\n\t\\"])
    expect(jsonSize(text)).toBe(wire(text) - 2);
  const event = { kind: "delta", text: mixed };
  expect(utf8Size(JSON.stringify(event))).toBe(wire(event));
});

test("a text within its budget is kept whole, past it its head ½ and tail ¼ around the mark", () => {
  expect(fit("short", 100)).toBe("short");
  const budget = 4096;
  const kept = fit(mixed, budget);
  const [head, tail] = kept.split("\n… cut …\n");
  expect(head.startsWith("HEAD") && tail.endsWith("TAIL")).toBe(true);
  // Each part is the longest within its share, never splitting a character.
  expect(jsonSize(head)).toBeLessThanOrEqual(budget / 2);
  expect(jsonSize(head)).toBeGreaterThan(budget / 2 - 6);
  expect(jsonSize(tail)).toBeLessThanOrEqual(budget / 4);
  expect(jsonSize(tail)).toBeGreaterThan(budget / 4 - 6);
  expect(mixed.startsWith(head) && mixed.endsWith(tail)).toBe(true);
  expect(jsonSize(kept)).toBeLessThanOrEqual(budget);
});

test("a text splits into pieces within the budget, whole when joined, never inside a character", () => {
  expect(pieces("", 10)).toEqual([""]);
  const parts = pieces(mixed, 1000);
  expect(parts.join("")).toBe(mixed);
  expect(parts.every((part) => jsonSize(part) <= 1000)).toBe(true);
  expect(parts.every((part) => !/^[\udc00-\udfff]|[\ud800-\udbff]$/.test(part))).toBe(true);
});

test("a block settles in numbered parts, each within an event, its text cut past the block's bound", () => {
  const long = "x".repeat(bounds.block * 2);
  const parts = settles("b", "t", "text", long);
  expect(parts.map((p) => [p.part, p.of])).toEqual(parts.map((_, i) => [i, parts.length]));
  expect(parts.every((p) => p.id === "b" && p.turn === "t" && p.block === "text")).toBe(true);
  expect(parts.every((p) => wire(p) <= bounds.event)).toBe(true);
  expect(parts.map((p) => p.text).join("")).toBe(fit(long, bounds.block));
  expect(settles("b", undefined, "thinking", "")).toEqual([
    { kind: "settle", id: "b", block: "thinking", part: 0, of: 1, text: "" },
  ]);
  expect(textBudget).toBeLessThan(bounds.event);
});

// link is a carrier the test answers and times by hand.
function link(ready = true) {
  const posted: (MirrorEvent | Marker)[][] = [];
  const answers: ((ok: boolean) => void)[] = [];
  const timers: { ms: number; run: () => void }[] = [];
  const state = { ready };
  const carrier: Carrier = {
    ready: () => state.ready,
    post: (events) => {
      posted.push(events);
      return new Promise((resolve) => answers.push(resolve));
    },
    after: (ms, run) => timers.push({ ms, run }),
  };
  // fire runs the timers of one delay, a kick's 50 ms or a batch's 5 s.
  const fire = (ms = 50) => {
    const due = timers.filter((t) => t.ms === ms);
    timers.splice(0, timers.length, ...timers.filter((t) => t.ms !== ms));
    for (const t of due) t.run();
  };
  // drain answers every batch with ok until nothing more is posted.
  const drain = async () => {
    for (let i = 0; i < 100 && (answers.length > 0 || timers.some((t) => t.ms === 50)); i++) {
      fire();
      for (const answer of answers.splice(0)) answer(true);
      await Promise.resolve();
      await Promise.resolve();
    }
  };
  return { carrier, posted, answers, fire, drain, state };
}

const settled = () => new Promise((resolve) => setTimeout(resolve, 0));

test("one batch is in flight at a time, each within its cap, and a later input replaces a queued one", async () => {
  const { carrier, posted, answers, fire, drain } = link();
  const box = new Outbox(carrier);
  box.push({ kind: "delta", id: "b", text: "a" });
  box.push({ kind: "input", id: "c", text: "{}" });
  box.push({ kind: "input", id: "c", text: '{"a":1}' });
  fire();
  expect(posted.map((batch) => batch.map((e) => (e as MirrorEvent).text))).toEqual([
    ["a", '{"a":1}'],
  ]);
  box.push({ kind: "delta", id: "b", text: "b" });
  fire();
  expect(posted.length).toBe(1);
  answers[0](true);
  await settled();
  fire();
  expect(posted[1]).toEqual([{ kind: "delta", id: "b", text: "b" }]);
  answers[1](true);
  await settled();
  const text = "y".repeat(textBudget);
  for (let i = 0; i < 6; i++) box.push({ kind: "result", id: `r${i}`, status: "done", text });
  await drain();
  expect(posted.length).toBeGreaterThan(3);
  for (const batch of posted) expect(wire(batch)).toBeLessThanOrEqual(bounds.batch);
  expect(posted.flat().filter((e) => e.kind === "result").length).toBe(6);
});

test("a batch refused or unanswered in 5 s is never sent again and stands as lost behind a resync", async () => {
  const refused = link();
  const box = new Outbox(refused.carrier);
  box.push({ kind: "delta", id: "b", text: "a" });
  box.push({ kind: "tool", id: "c", name: "Bash" });
  refused.fire();
  box.push({ kind: "delta", id: "b", text: "b" });
  refused.answers[0](false);
  await settled();
  refused.fire();
  expect(refused.posted[1]).toEqual([
    { kind: "delta", id: "b", text: "b" },
    { kind: "resync", lost: 2 },
  ]);

  const stalled = link();
  const late = new Outbox(stalled.carrier);
  late.push({ kind: "turn", turn: "a", state: "working" });
  stalled.fire();
  late.push({ kind: "turn", turn: "b", state: "working" });
  stalled.fire(5000);
  stalled.fire();
  expect(stalled.posted[1]).toEqual([
    { kind: "turn", turn: "b", state: "working" },
    { kind: "resync", lost: 1 },
  ]);
  // The first batch's answer, come late, changes nothing.
  stalled.answers[0](true);
  await settled();
  expect(stalled.posted.length).toBe(2);
});

test("a link that is down keeps the events until a kick finds it up", async () => {
  const { carrier, posted, fire, state } = link(false);
  const box = new Outbox(carrier);
  box.push({ kind: "prompt", id: "p", text: "go" });
  fire();
  expect(posted).toEqual([]);
  state.ready = true;
  box.kick();
  fire();
  expect(posted).toEqual([[{ kind: "prompt", id: "p", text: "go" }]]);
});

test("an event past its bound is never sent: its item stands as lost", async () => {
  const { carrier, posted, drain } = link();
  const box = new Outbox(carrier);
  box.push({ kind: "result", id: "big", status: "done", text: "y".repeat(bounds.batch) });
  await drain();
  expect(posted.flat()).toEqual([{ kind: "lost", id: "big" }]);
});

// sent is what an outbox held, read by bringing its link up and sending it all.
async function sent(
  box: Outbox,
  state: { ready: boolean },
  drain: () => Promise<void>,
  posted: (MirrorEvent | Marker)[][],
) {
  state.ready = true;
  box.kick();
  await drain();
  return posted.flat() as MirrorEvent[];
}

test("past its bound the outbox drops deltas first, then whole items behind a lost marker", async () => {
  const { carrier, state, drain, posted } = link(false);
  const box = new Outbox(carrier);
  const text = "d".repeat(30000);
  box.push({ kind: "mode", mode: "plan" });
  for (let i = 0; i < 20; i++) box.push({ kind: "delta", id: `b${i}`, text });
  expect(box.bytes).toBeLessThanOrEqual(bounds.outbox);
  for (let i = 0; i < 20; i++) {
    for (const part of [0, 1])
      box.push({ kind: "settle", id: `s${i}`, part, of: 2, text: text.slice(0, 12000) });
    expect(box.bytes).toBeLessThanOrEqual(bounds.outbox);
  }
  const kept = await sent(box, state, drain, posted);
  expect(kept.some((e) => e.kind === "delta")).toBe(false);
  // An event naming no item goes alone, leaving no marker.
  expect(kept.some((e) => e.kind === "mode")).toBe(false);
  expect(kept.filter((e) => e.kind === "lost").map((e) => e.id)).toContain("s0");
  // A dropped settle takes its other parts with it.
  expect(kept.some((e) => e.kind === "settle" && e.id === "s0")).toBe(false);
  expect(kept.filter((e) => e.kind === "settle" && e.id === "s19").length).toBe(2);
});

test("markers past their reserve collapse into one resync counting what went", async () => {
  const { carrier, state, drain, posted } = link(false);
  const box = new Outbox(carrier);
  for (let i = 0; i < 1000; i++)
    box.push({ kind: "turn", turn: `t${i}`, state: i % 2 ? "idle" : "working" });
  expect(box.bytes).toBeLessThanOrEqual(bounds.markers);
  const kept = await sent(box, state, drain, posted);
  expect(kept[0].kind).toBe("resync");
  expect(kept.filter((e) => e.kind === "resync").length).toBe(1);
  expect((kept[0].lost ?? 0) + kept.length - 1).toBe(1000);
});
