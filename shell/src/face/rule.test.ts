// The input's top rule (rule.ts): the run's work at its right, and at its
// left what the face heard from the session's mirror, the mode or else the
// thinking level shift+tab moves, given as one.
import { expect, test } from "bun:test";
import { type Left, ruleOf, ruleParts } from "./rule.ts";

// The run's room work sits at the right end of the input's top rule, where
// Claude Code's hint line ends with it, and fits any pane; none is a bare rule.
test("the input's rule carries the run's work, cut to a narrow pane", () => {
  expect(ruleOf(20, "brief 57 3/6")).toBe(`${"─".repeat(5)} brief 57 3/6 ─`);
  expect(ruleOf(12, "brief 57 3/6 · ⧗ 2")).toBe("─── brief… ─");
  expect(ruleOf(30)).toBe("─".repeat(30));
  for (let width = 0; width <= 30; width++) {
    const rule = ruleOf(width, "名前 brief 57 3/6");
    expect(Bun.stringWidth(rule)).toBe(width);
    expect(rule.endsWith(" ─") || rule === "─".repeat(width)).toBe(true);
  }
});

// A Claude Code session says its mode, as the terminal program names it under
// its input; a pi session the thinking level its shift+tab moves, in pi's
// words with the key (the owner, October 31, 2026). A narrow pane drops the
// hint, then the mode or the level, then cuts the work.
test("the rule's left is the session's mode, else its thinking level, from one input", () => {
  const cases: [Left, number, string | undefined, string][] = [
    [{ mode: "auto" }, 30, "⧗ 3", `── ⏵⏵ auto mode ${"─".repeat(8)} ⧗ 3 ─`],
    [{ mode: "auto" }, 20, "⧗ 3", `${"─".repeat(14)} ⧗ 3 ─`],
    [{ mode: "auto" }, 30, undefined, `── ⏵⏵ auto mode ${"─".repeat(14)}`],
    // The default mode says nothing.
    [{ mode: "default" }, 40, "⧗ 3", `${"─".repeat(34)} ⧗ 3 ─`],
    [
      { level: "high" },
      60,
      "brief 61 1/3",
      `── ◆ high (shift+tab to cycle) ${"─".repeat(14)} brief 61 1/3 ─`,
    ],
    [{ level: "off" }, 40, undefined, "── ◆ thinking off (shift+tab to cycle) ─"],
    [{ level: "high" }, 30, "brief 61 1/3", `── ◆ high ${"─".repeat(5)} brief 61 1/3 ─`],
    [{ level: "high" }, 20, "brief 61 1/3", `${"─".repeat(5)} brief 61 1/3 ─`],
    [{ level: "high" }, 10, "brief 61 1/3", "── brie… ─"],
    // The mode wins where a session says both.
    [{ mode: "auto", level: "high" }, 30, undefined, `── ⏵⏵ auto mode ${"─".repeat(14)}`],
    [{}, 20, undefined, "─".repeat(20)],
  ];
  for (const [said, width, work, rule] of cases)
    expect([said, width, ruleOf(width, work, said)]).toEqual([said, width, rule]);
  // The mode and the level are each a part of their own, drawn in their colour.
  expect(ruleParts(60, "⧗ 2", { level: "xhigh" }).find((p) => p.level)).toEqual({
    text: "◆ xhigh",
    level: "xhigh",
  });
  expect(ruleParts(60, "⧗ 2", { mode: "plan" }).find((p) => p.mode)).toEqual({
    text: "⏸ plan mode",
    mode: "plan",
  });
  for (const said of [{ mode: "bypass" }, { level: "minimal" }] as Left[])
    for (let width = 0; width <= 60; width++)
      expect(Bun.stringWidth(ruleOf(width, "brief 名前 1/2", said))).toBe(width);
});
