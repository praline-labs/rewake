import { expect, test } from "bun:test";
import { fitHints, hintsFor } from "./hints.ts";

const worker = { focus: "worker", name: "w", prefix: false, zoomed: false, workers: 1 } as const;

test("the hints of a narrow column are left out whole, the first kept", () => {
  const all = hintsFor(worker);
  const line = (width: number) =>
    fitHints(all, width)
      .map((h) => `${h.keys} ${h.does}`)
      .join(" · ");
  const full = line(500);
  expect(fitHints(all, full.length)).toHaveLength(all.length);
  expect(fitHints(all, full.length - 1)).toHaveLength(all.length - 1);
  expect(line(30)).toBe("keys go to w · Ctrl+\\ ←/m main");
  expect(fitHints(all, 3)).toEqual([{ keys: "keys", does: "go to w" }]);
});
