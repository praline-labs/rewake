import { expect, test } from "bun:test";
import {
  type Answer,
  backText,
  headOf,
  infoOf,
  listOf,
  navOf,
  ordered,
  poll,
  type Summary,
  stepOf,
  stripOfBriefs,
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

test("an entry is its title, then progress, comments and who is bound, an ended run marked", () => {
  expect(headOf(brief)).toBe("3 · Auth refactor");
  expect(infoOf(brief)).toBe("2/5 · 4 comments · write-claude, review-claude (ended)");
  expect(infoOf({ ...brief, commentCount: 1, bound: [] })).toBe("2/5 · 1 comment");
  expect(infoOf({ ...brief, closed: true })).toBe(
    "2/5 · 4 comments · closed · write-claude, review-claude",
  );
});

const b = (id: number, title: string, closed = false): Summary => ({ ...brief, id, title, closed });

test("the shell lists the open briefs oldest first, then the closed ones", () => {
  const list = [b(5, "e"), b(2, "b", true), b(4, "d"), b(1, "a", true), b(3, "c")];
  expect(ordered(list).map((x) => x.id)).toEqual([3, 4, 5, 1, 2]);
});

test("the navigation row holds the open briefs and fits its width whichever shows", () => {
  const list = ordered([b(1, "Auth refactor"), b(2, "Shell bar"), b(3, "Old", true)]);
  const texts = (shown: number, width: number) => navOf(list, shown, width).map((i) => i.text);
  expect(texts(1, 80)).toEqual([backText, " 1 Auth refactor ", " 2 Shell bar "]);
  // The closed brief shows only while it is the one shown, at the end.
  expect(texts(3, 80)).toEqual([backText, " 1 Auth refactor ", " 2 Shell bar ", " 3 Old "]);
  const width = (items: string[]) => items.reduce((n, t) => n + t.length + 1, 0) - 1;
  const narrow = texts(1, 30);
  expect(width(narrow)).toBeLessThanOrEqual(30);
  expect(narrow).toEqual(texts(2, 30));
  expect(narrow[1]).toEndWith("… ");
  // Too narrow for any title: the numbers alone.
  expect(texts(1, 18)).toEqual([backText, " 1 ", " 2 "]);
});

test("← and → step through the row and stop at its ends", () => {
  const items = navOf(ordered([b(1, "a"), b(2, "b"), b(4, "d")]), 2, 80);
  expect(stepOf(items, 2, -1)).toBe(1);
  expect(stepOf(items, 2, 1)).toBe(4);
  expect(stepOf(items, 4, 1)).toBeUndefined();
  expect(stepOf(items, 1, -1)).toBeUndefined();
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

test("the strip under the briefs says what shows, cut at the width", () => {
  const closed = { ...brief, id: 1, closed: true };
  expect(stripOfBriefs({ list: [brief, closed] }, 80)).toEqual(["briefs", "1 open", "1 closed"]);
  expect(stripOfBriefs({ list: [brief] }, 80)).toEqual(["briefs", "1 open"]);
  expect(stripOfBriefs({ id: 3, detail: brief }, 80)).toEqual(["brief 3", "2/5", "4 comments"]);
  // A detail of the brief shown before is not this one's.
  expect(stripOfBriefs({ id: 4, detail: brief }, 80)).toEqual(["brief 4"]);
  expect(stripOfBriefs({ id: 3, detail: brief }, 12)).toEqual(["brief 3", "2…"]);
});
