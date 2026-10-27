import { expect, test } from "bun:test";
import {
  type Answer,
  authorOf,
  backText,
  detailOf,
  foldText,
  headOf,
  infoOf,
  listOf,
  navOf,
  ordered,
  pickOf,
  redraws,
  rowsOf,
  type Summary,
  stepOf,
  stripOfBriefs,
  timeOf,
  unreadableLine,
} from "./briefs.ts";
import type { RoomView } from "./room.ts";

const brief: Summary = {
  id: 3,
  title: "Auth refactor",
  closed: false,
  edition: 2,
  done: 2,
  total: 5,
  commentCount: 4,
  bound: [
    { name: "write-claude", run: "running", kind: "work" },
    { name: "review-claude", run: "ended", kind: "work" },
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

test("a reviewer is marked in the entry, and every comment names its author and capacity", () => {
  const reviewed = {
    ...brief,
    bound: [
      { name: "write-claude", run: "running" as const, kind: "work" as const },
      { name: "review-pi", run: "running" as const, kind: "review" as const },
      { name: "review-claude", run: "ended" as const, kind: "review" as const },
    ],
  };
  expect(infoOf(reviewed)).toBe(
    "2/5 · 4 comments · write-claude, review-pi (review), review-claude (review, ended)",
  );
  const comment = { n: 1, from: "review-pi", at: "2026-10-27T06:00:00Z", text: "ok" };
  expect(authorOf({ ...comment, capacity: "review" })).toBe("review-pi · review");
  expect(authorOf(comment)).toBe("review-pi");
});

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

const view = (fields: Partial<RoomView>): RoomView => ({
  room: "r",
  asMain: true,
  spaces: [],
  sessions: [],
  ...fields,
});

test("a list keeps the records that did not read, to show them", () => {
  const listed = listOf(
    view({ briefs: [brief], unreadableBriefs: [{ path: "briefs/9", why: "bad json" }] }),
  );
  if (!listed.ok) throw new Error(listed.line);
  expect(listed.value.briefs).toEqual([brief]);
  expect(listed.value.unreadable.map(unreadableLine)).toEqual(["unreadable: briefs/9 — bad json"]);
  expect(listOf(view({}))).toEqual({ ok: true, value: { briefs: [], unreadable: [] } });
});

test("a view of a caller not proved main's has no briefs, and says so", () => {
  expect(listOf(view({ asMain: false }))).toEqual({
    ok: false,
    line: "main's run is not known yet; asking again",
  });
  expect(detailOf(view({ asMain: false }), 3).ok).toBe(false);
});

test("the opened brief is the one asked for, with its text", () => {
  const opened = { ...brief, text: "# T", comments: [] };
  expect(detailOf(view({ brief: opened }), 3)).toEqual({ ok: true, value: opened });
  // One that is not there, or of another id, or without its text, is a line.
  for (const bad of [undefined, { ...opened, id: 4 }, brief]) {
    expect(detailOf(view({ brief: bad }), 3)).toEqual({ ok: false, line: "brief 3 did not read" });
  }
});

test("a list that failed to read shows its reason, not an empty list", () => {
  const failed = listOf(view({ briefsProblem: "index.json: invalid character" }));
  expect(failed).toEqual({
    ok: false,
    line: "the briefs did not read: index.json: invalid character",
  });
});

test("an answer is drawn when it differs from what shows, whatever was shown before", () => {
  const good: Answer<number> = { ok: true, value: 1 };
  const bad: Answer<number> = { ok: false, line: "did not read" };
  const nothing = { same: () => false };
  const shows = (n: number, problem?: string) => ({ problem, same: (v: number) => v === n });
  // The same value is not drawn again; another is.
  expect(redraws(good, shows(1))).toBe(false);
  expect(redraws({ ok: true, value: 2 }, shows(1))).toBe(true);
  // Read brief A, open B, return to A: the cleared view draws A's unchanged answer.
  expect(redraws(good, nothing)).toBe(true);
  // A line shown is replaced by the next good answer, though its value is the one shown.
  expect(redraws(good, shows(1, "did not read"))).toBe(true);
  // The same line is not drawn twice; another is.
  expect(redraws(bad, shows(1, "did not read"))).toBe(false);
  expect(redraws(bad, shows(1))).toBe(true);
});

test("a time that does not parse is shown as it came", () => {
  expect(timeOf("soon")).toBe("soon");
  expect(timeOf("2026-10-26T14:05:00Z")).toMatch(/^2026-10-2[67] \d\d:\d\d$/);
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

test("the list shows the open briefs and a fold row, the closed ones only unfolded", () => {
  const list = ordered([b(1, "a"), b(2, "b", true), b(3, "c"), b(4, "d", true)]);
  const picks = (unfolded: boolean) => rowsOf(list, unfolded).map(pickOf);
  expect(picks(false)).toEqual([1, 3, "fold"]);
  expect(picks(true)).toEqual([1, 3, "fold", 2, 4]);
  expect(rowsOf(list, false)[2]).toEqual({ fold: 2 });
  expect(foldText(2)).toBe("closed · 2");
  // No closed brief: no row; only closed ones: the row alone.
  expect(rowsOf([b(1, "a")], true).map(pickOf)).toEqual([1]);
  expect(rowsOf([b(2, "b", true)], false).map(pickOf)).toEqual(["fold"]);
  expect(rowsOf([], false)).toEqual([]);
});
