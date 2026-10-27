import { expect, test } from "bun:test";
import type { TerminalColors } from "@opentui/core";
import { contrast, themeOf } from "./theme.ts";

// A terminal that answered every query, and one that answered none: in the
// second the faint colour and the buttons' grey are the same fallback.
const answered = {
  palette: [
    "#2e3436",
    "#cc0000",
    "#4e9a06",
    "#c4a000",
    "#3465a4",
    "#75507b",
    "#06989a",
    "#d3d7cf",
    "#555753",
    "#ef2929",
    "#8ae234",
    "#fce94f",
    "#729fcf",
    "#ad7fa8",
    "#34e2e2",
    "#eeeeec",
  ],
  defaultForeground: "#d3d7cf",
  defaultBackground: "#300a24",
} as unknown as TerminalColors;

test("faint words on a button's grey read with and without the terminal's colours", () => {
  for (const colors of [answered, undefined]) {
    const theme = themeOf(colors);
    expect(contrast(theme.faintOnIdle, theme.idle)).toBeGreaterThanOrEqual(3);
  }
  expect(themeOf(undefined).faint).toBe(themeOf(undefined).idle);
});
