import { expect, test } from "bun:test";
import {
  type Answer,
  listOf,
  poll,
  rowOf,
  type Summary,
  timeOf,
  unreadableLine,
} from "./briefs.ts";

const brief: Summary = {
  id: 3,
  title: "Auth refactor",
  closed: false,
  edition: 2,
  done: 2,
  total: 5,
  commentCount: 4,
  bound: [
    { name: "write-claude", run: "running" },
    { name: "review-claude", run: "ended" },
  ],
};

const text = (b: Summary) =>
  rowOf(b)
    .map((p) => p.text)
    .join("");

test("a row says title, progress, comments and who is bound, an ended run marked", () => {
  expect(text(brief)).toBe(
    "3 · Auth refactor · 2/5 · 4 comments · write-claude, review-claude (ended)",
  );
  expect(rowOf(brief).map((p) => p.faint)).toEqual([false, false, false, false, true]);
  expect(text({ ...brief, commentCount: 1, bound: [] })).toBe(
    "3 · Auth refactor · 2/5 · 1 comment",
  );
});

test("a closed brief is faint whole and its bindings are not marked one by one", () => {
  const closed = { ...brief, closed: true };
  expect(text(closed)).toBe(
    "3 · Auth refactor · 2/5 · 4 comments · closed · write-claude, review-claude",
  );
  expect(rowOf(closed).every((p) => p.faint)).toBe(true);
});

test("a list keeps the records that did not read, to show them", () => {
  const listed = listOf({ briefs: [brief], unreadable: [{ path: "briefs/9", why: "bad json" }] });
  expect(listed.briefs).toEqual([brief]);
  expect(listed.unreadable.map(unreadableLine)).toEqual(["unreadable: briefs/9 — bad json"]);
  expect(listOf({})).toEqual({ briefs: [], unreadable: [] });
});

test("a time that does not parse is shown as it came", () => {
  expect(timeOf("soon")).toBe("soon");
  expect(timeOf("2026-10-26T14:05:00Z")).toMatch(/^2026-10-2[67] \d\d:\d\d$/);
});

test("a poll shows an answer only when it changed, and stops", async () => {
  const answers: Answer<number>[] = [
    { ok: true, value: 1 },
    { ok: true, value: 1 },
    { ok: false, line: "did not read" },
    { ok: true, value: 2 },
  ];
  let reads = 0;
  const shown: Answer<number>[] = [];
  const stop = poll(
    async () => answers[Math.min(reads++, answers.length - 1)]!,
    (a) => shown.push(a),
    5,
  );
  await new Promise((resolve) => setTimeout(resolve, 80));
  stop();
  const seen = reads;
  await new Promise((resolve) => setTimeout(resolve, 40));
  expect(reads).toBe(seen);
  expect(shown).toEqual([answers[0]!, answers[2]!, answers[3]!]);
});

test("a read that throws is a line, not a crash", async () => {
  const shown: Answer<number>[] = [];
  const stop = poll<number>(
    async () => {
      throw new Error("boom");
    },
    (a) => shown.push(a),
    5,
  );
  await new Promise((resolve) => setTimeout(resolve, 30));
  stop();
  expect(shown).toHaveLength(1);
  expect(shown[0]?.ok).toBe(false);
});
