import { expect, test } from "bun:test";
import type { FaceLine } from "./messages.ts";
import { FaceModel } from "./model.ts";
import { render } from "./render.ts";
import { Ticker, tickEvery } from "./ticker.ts";

const text = (lines: FaceLine[]) => lines.map((l) => l.spans.map((s) => s.text).join(""));

// Seen live: a delta painted 33 ms after it came and the timer on a clock of
// its own, so one moved without the other. Now a change waits for the tick,
// and the tick paints the text and the time together.
test("a delta and the timer that change together are painted once, at the tick", () => {
  let now = 10_000;
  let run: (() => void) | undefined;
  let stopped = 0;
  const m = new FaceModel();
  m.apply({ op: "begin", items: 0, turn: "working" } as never);
  m.apply({ op: "end" } as never);
  m.apply({ op: "add", item: { id: "b", kind: "text", status: "streaming", text: "a" } } as never);
  const since = 0;
  const paints: string[][] = [];
  const ticker = new Ticker(
    (at) => paints.push(text(render(m, 40, 4, 0, { now: at, since }))),
    () => m.turn === "working",
    {
      every: (tick, ms) => {
        expect(ms).toBe(tickEvery);
        run = tick;
        return 1;
      },
      stop: () => stopped++,
      now: () => now,
    },
  );

  // The delta lands and the second turns before the tick: nothing paints.
  m.apply({ op: "append", id: "b", text: " b" } as never);
  ticker.change();
  // Past the quiet start, when the time shows with no tokens yet.
  now = 17_000;
  m.apply({ op: "append", id: "b", text: " c" } as never);
  ticker.change();
  expect(paints).toEqual([]);

  run?.();
  expect(paints).toHaveLength(1);
  const [reply, , turn] = paints[0] ?? [];
  expect(reply).toBe("● a b c");
  expect(turn).toMatch(/^. \p{L}+… \(17s\)$/u);

  // A working turn goes on ticking with no change; an idle one stops.
  run?.();
  expect(paints).toHaveLength(2);
  m.apply({ op: "turn", turn: "idle" } as never);
  run?.();
  expect(paints).toHaveLength(2);
  expect(stopped).toBe(1);
});
