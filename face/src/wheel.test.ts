import { expect, test } from "bun:test";
import { Wheel } from "./wheel.ts";

// rows is how far a stream of notches moves the view: count notches every
// gap ms the given way, from a moment.
function rows(wheel: Wheel, way: 1 | -1, count: number, gap: number, from: number) {
  let moved = 0;
  for (let i = 0; i < count; i++) moved += wheel.rows(way, from + i * gap);
  return moved;
}

// Measured live on a native pane over one long reply (October 30, 2026,
// Claude Code 2.1.310): the face moved 3 rows a notch whatever the speed,
// the native view one a notch alone, 19 for ten notches 10 ms apart and 67
// for twenty 4 ms apart.
test("a notch moves a native pane's rows: one alone, ramping while they come fast", () => {
  const wheel = new Wheel();
  expect(rows(wheel, 1, 4, 300, 1_000)).toBe(4);
  expect(rows(wheel, 1, 10, 10, 10_000)).toBe(19);
  expect(rows(wheel, 1, 20, 4, 20_000)).toBe(67);
  // The ramp stops at 6 a notch, and a pause starts it again from one.
  expect(rows(wheel, 1, 40, 4, 30_000) - rows(new Wheel(), 1, 20, 4, 30_000)).toBe(20 * 6);
  expect(wheel.rows(1, 40_000)).toBe(1);
});

test("the first notch against the way the view moved is held, and the next one turns it", () => {
  const wheel = new Wheel();
  expect(rows(wheel, 1, 3, 300, 1_000)).toBe(3);
  expect(wheel.rows(-1, 2_000)).toBe(0);
  expect(wheel.rows(-1, 2_300)).toBe(1);
  expect(wheel.rows(-1, 2_600)).toBe(1);
});

// A wheel that sends a stray notch back while it turns: the notch back the
// old way within 200 ms marks a wheel's, whose notches ramp faster.
test("a bounce makes the stream a wheel's, ramping faster until it rests", () => {
  const wheel = new Wheel();
  wheel.rows(1, 1_000);
  expect(wheel.rows(-1, 1_050)).toBe(0);
  expect(wheel.rows(1, 1_100)).toBeGreaterThan(1);
  expect(rows(wheel, 1, 5, 30, 1_130)).toBeGreaterThan(rows(new Wheel(), 1, 5, 30, 1_130));
  // After a rest of 1.5 s it is a notch alone again.
  expect(wheel.rows(1, 5_000)).toBe(1);
});
