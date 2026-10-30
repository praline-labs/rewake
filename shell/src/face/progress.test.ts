import { expect, test } from "bun:test";
import type { Item } from "./mirror.gen.ts";
import { count, phaseOf, Tracker } from "./progress.ts";
import { turnLine } from "./turn.ts";

const prompt: Item = { id: "p", kind: "prompt", status: "settled", text: "go" };
const said = (now: number, progress: Parameters<typeof turnLine>[1]["progress"], width = 80) =>
  (turnLine("working", { now, since: 0, progress }, width)?.spans ?? [])
    .map((s) => s.text)
    .join("");

// The arrow points up only while the request is out and nothing came back:
// after the prompt, and once every call of a step has ended.
test("the phase is requesting until something streams back, and again after the calls", () => {
  const call = (status: Item["status"], text = '{"command":"ls"}'): Item => ({
    id: "c",
    kind: "tool",
    name: "Bash",
    status,
    text,
  });
  expect(phaseOf([prompt])).toBe("requesting");
  expect(phaseOf([prompt, { id: "t", kind: "thinking", status: "streaming", text: "h" }])).toBe(
    "thinking",
  );
  expect(phaseOf([prompt, { id: "t", kind: "text", status: "streaming", text: "h" }])).toBe(
    "responding",
  );
  expect(phaseOf([prompt, call("running", "")])).toBe("tool-input");
  expect(phaseOf([prompt, call("running")])).toBe("tool-use");
  expect(phaseOf([prompt, call("done")])).toBe("requesting");
  expect(phaseOf([prompt, { id: "t", kind: "text", status: "settled", text: "h" }])).toBe(
    "tool-use",
  );
  // A new prompt starts the turn over.
  expect(phaseOf([prompt, call("done"), { ...prompt, id: "p2" }])).toBe("requesting");
});

// A prompt queued while the turn works is no new turn: the phase and the
// count stay the turn's, known by the turn its items name; a new turn starts
// both over.
test("a prompt queued during a turn leaves its phase and count, a new turn starts them over", () => {
  const t = new Tracker();
  const asked: Item = { ...prompt, turn: "t1" };
  const reply: Item = {
    id: "r",
    turn: "t1",
    kind: "text",
    status: "settled",
    text: "x".repeat(4000),
  };
  const call: Item = {
    id: "c",
    turn: "t1",
    kind: "tool",
    name: "Bash",
    status: "running",
    text: "{}",
  };
  const queued: Item = { id: "q", kind: "prompt", status: "settled", text: "and then" };
  t.reset(0, []);
  expect(t.at([asked, reply, call], 0)).toMatchObject({ phase: "tool-use", tokens: 1001 });
  expect(t.at([asked, reply, call, queued], 50)).toMatchObject({ phase: "tool-use", tokens: 1001 });
  const items = [asked, reply, { ...call, status: "done" as const }, queued];
  t.reset(100, items);
  expect(t.at(items, 100)).toMatchObject({ phase: "requesting", tokens: 0 });
  const next: Item = { id: "n", turn: "t2", kind: "text", status: "streaming", text: "yy" };
  expect(t.at([...items, next], 150)).toMatchObject({ phase: "responding" });
});

// The count moves towards what streamed back on the 50 ms clock, as the
// terminal program's does, and leaps a long way behind.
test("the estimate is a quarter of what streamed back, smoothed tick by tick", () => {
  const t = new Tracker();
  t.reset(0);
  const items = (n: number): Item[] => [
    prompt,
    { id: "t", kind: "text", status: "streaming", text: "x".repeat(n) },
  ];
  expect(t.at(items(100), 0).tokens).toBe(0);
  // 100 behind: max(8, ceil(15)) = 15 a tick.
  expect(t.at(items(100), 50).tokens).toBe(4);
  // Within 70 of the total it creeps by 3.
  expect(t.at(items(100), 2000).tokens).toBe(25);
  expect(t.at(items(5000), 2050).tokens).toBe(1250);
  expect(t.at(items(5000), 2050).phase).toBe("responding");
  expect(count(622)).toBe("622");
  expect(count(1234)).toBe("1.2k");
});

// The bracket shows once there are tokens or thinking, else after 16 s, and
// a narrow line drops the tokens before the time.
test("the working line shows its time, tokens and thinking as the width allows", () => {
  expect(said(10_000, { phase: "requesting", tokens: 0 })).toMatch(/^. \p{L}+…$/u);
  expect(said(17_000, { phase: "requesting", tokens: 0 })).toMatch(/… \(17s\)$/);
  expect(said(2_000, { phase: "responding", tokens: 38 })).toMatch(/… \(2s · ↓ 38 tokens\)$/);
  expect(said(2_000, { phase: "requesting", tokens: 38 })).toMatch(/\(2s · ↑ 38 tokens\)$/);
  expect(
    said(2_000, {
      phase: "thinking",
      tokens: 38,
      thinking: { kind: "thinking", text: "thinking" },
    }),
  ).toMatch(/\(2s · ↓ 38 tokens · thinking\)$/);
  expect(said(2_000, { phase: "responding", tokens: 38 }, 28)).toMatch(/… \(2s\)$/);
  expect(said(2_000, { phase: "responding", tokens: 38 }, 16)).toMatch(/^. \p{L}+…$/u);
});

// Thinking says so while it streams, then how long it took for two seconds
// once it has shown for two.
test("a thinking block's word turns into how long it took", () => {
  const t = new Tracker();
  t.reset(0);
  const thinking: Item = { id: "t", kind: "thinking", status: "streaming", text: "h" };
  const after: Item[] = [
    prompt,
    { ...thinking, status: "settled" },
    { id: "r", kind: "text", status: "streaming", text: "x" },
  ];
  expect(t.at([prompt, thinking], 0).thinking?.text).toBe("thinking");
  expect(t.at([prompt, thinking], 12_000).thinking?.text).toBe("still thinking");
  expect(t.at(after, 12_500).thinking?.text).toBe("thought for 13s");
  expect(t.at(after, 14_600).thinking).toBeUndefined();
});
