import { expect, test } from "bun:test";
import type { Session, Verdict } from "./room.ts";
import { cacheOf, markOf, stripParts } from "./strip.tsx";

const base: Session = {
  name: "w",
  cwd: "/p",
  model: "Model",
  effort: "low",
  window: 100_000,
  used: 23_000,
  percent: 23,
};
const now = 1_000_000_000_000;
const at = (ms: number): Session => ({ ...base, cache: { hit: 90, expiresAt: now + ms } });

test("the cache reads as rewake list has it, counted on the given clock", () => {
  expect(cacheOf(at(42 * 60_000 - 10_000), now)?.text).toBe("90% 42m");
  expect(cacheOf(at(20_000), now)?.text).toBe("90% <1m");
  expect(cacheOf(at(-1), now)?.text).toBe("90% —");
  expect(cacheOf({ ...base, cache: { hit: 90 } }, now)?.text).toBe("90%");
  expect(cacheOf(base, now)).toBeUndefined();
});

test("a long line drops the model, then the cache, then the branch", () => {
  const state = { session: "w", branch: "wt-1", status: at(60 * 60_000) };
  const texts = (width: number) => stripParts(state, width, now).map((p) => p.text);
  const full = texts(200);
  expect(full.at(-1)).toBe("90% 60m");
  expect(full).toContain("Model low");
  const noModel = texts(full.join(" · ").length - 1);
  expect(noModel).not.toContain("Model low");
  expect(noModel).toContain("90% 60m");
  const noCache = texts(noModel.join(" · ").length - 1);
  expect(noCache).not.toContain("90% 60m");
  expect(noCache).toContain("⎇ wt-1");
  const noBranch = texts(noCache.join(" · ").length - 1);
  expect(noBranch).not.toContain("⎇ wt-1");
});

test("thinking is named after the effort only while it is on", () => {
  const model = (thinking?: boolean) =>
    stripParts({ session: "w", status: { ...base, thinking } }, 200, now).map((p) => p.text);
  expect(model(true)).toContain("Model low thinking");
  expect(model(false)).toContain("Model low");
  expect(model(undefined)).toContain("Model low");
});

test("the strip shows the verdict's label and the button its mark, deriving neither", () => {
  const verdict = (v: Partial<Verdict>): Session => ({
    ...base,
    verdict: {
      activity: "working",
      label: "working",
      blocked: "",
      stale: false,
      mark: "steady",
      ...v,
    },
  });
  const activity = (s: Session) => stripParts({ session: "w", status: s }, 200, now)[1]?.text;
  expect(activity(verdict({ label: "working (stale)", stale: true, mark: "blink" }))).toBe(
    "working (stale)",
  );
  expect(activity(verdict({ activity: "unknown", label: "unknown (stale)" }))).not.toContain(
    "unknown",
  );
  expect(markOf(verdict({ mark: "blink" }))).toBe("blink");
  expect(markOf(base)).toBe("off");
  expect(markOf(undefined)).toBe("off");
});
