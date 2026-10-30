import { expect, test } from "bun:test";
import { testRender } from "@opentui/solid";
import { Bar } from "./bar.tsx";
import { themeOf } from "./theme.ts";

const theme = themeOf(undefined);
const tabs = ["alpha", "beta", "gamma"].map((label) => ({ id: label, label }));

async function barAt(width: number, zoomed = false) {
  const { renderOnce, captureCharFrame } = await testRender(
    () => (
      <Bar
        tabs={tabs}
        focused="alpha"
        zoomed={zoomed}
        theme={theme}
        pick={() => {}}
        briefs="briefs"
        mark={() => "off"}
      />
    ),
    { width, height: 1 },
  );
  await renderOnce();
  return captureCharFrame().split("\n")[0] ?? "";
}

// The owner (October 30, 2026): the briefs button goes to the bar's right
// edge, the workers' stay on the left.
test("the briefs button stands at the bar's right edge, the workers' at its left", async () => {
  expect(await barAt(60)).toBe(`  alpha   beta   gamma${" ".repeat(30)}briefs  `);
  expect(await barAt(60, true)).toBe(`  alpha   beta   gamma${" ".repeat(22)}zoomed  briefs  `);
});

// Seen while it was written: a button the cut reached wrapped its name off
// the row and showed nothing of it.
test("on a narrow bar the workers' buttons are cut a cell before the briefs, never under it", async () => {
  expect(await barAt(30)).toBe("  alpha   beta   gam  briefs  ");
  expect(await barAt(17)).toBe("  alph   briefs  ");
  expect(await barAt(12)).toBe("    briefs  ");
});
