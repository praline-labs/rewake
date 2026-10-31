import { expect, test } from "bun:test";
import { testRender } from "@opentui/solid";
import type { Session, Verdict } from "./room.ts";
import { cacheOf, markOf, Strip, type StripState, stripParts } from "./strip.tsx";
import { themeOf } from "./theme.ts";

const base: Session = {
  name: "w",
  cwd: "/p",
  path: "/p",
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

// The owner, October 31, 2026: the ttl and the 5h/7d limits matter, and a
// narrow pane loses what is least needed first; the limits are main's alone,
// and the context stands at the strip's right edge.
const limits = [
  { window: "5h", percent: 37 },
  { window: "7d", percent: 9 },
];
const shown = (line: ReturnType<typeof stripParts>) => [
  ...line.left.map((p) => p.text),
  ...(line.context ? [line.context.text] : []),
];

test("a long line drops the model, then the 7d limit, then the 5h, then the cache", () => {
  const status: Session = { ...at(60 * 60_000), own: true, limits };
  const state = { session: "w", path: "/p", branch: "wt-1", status };
  const texts = (width: number) => shown(stripParts(state, width, now));
  const full = texts(200);
  expect(full).toEqual(["w", "Model low", "90% 60m", "5h 37%", "7d 9%", "23k/100k"]);
  // The branch is on the pane's top row now, never on the strip.
  expect(full.join(" ")).not.toContain("wt-1");
  let left = full;
  for (const gone of ["Model low", "7d 9%", "5h 37%", "90% 60m"]) {
    left = texts(left.join(" · ").length - 1);
    expect(left).not.toContain(gone);
  }
  expect(left).toEqual(["w", "23k/100k"]);
});

test("the account's limits show on main's strip alone", () => {
  const texts = (own?: boolean) =>
    shown(stripParts({ session: "w", status: { ...base, own, limits } }, 200, now));
  expect(texts(true)).toContain("5h 37%");
  expect(texts(true)).toContain("7d 9%");
  expect(texts(undefined).join(" ")).not.toMatch(/5h|7d/);
});

test("the context stays at the right whole while the left is cut", () => {
  const status: Session = { ...base, name: "a-long-worker-name" };
  const line = (width: number) => stripParts({ session: "a-long-worker-name", status }, width, now);
  expect(line(200).context?.text).toBe("23k/100k");
  const narrow = line(20);
  expect(narrow.context?.text).toBe("23k/100k");
  expect(narrow.left.map((p) => p.text)).toEqual(["a-long-w…"]);
  // Left, separator's gap and context fill the width and no more.
  expect(narrow.left.map((p) => p.text).join("").length + 3 + "23k/100k".length).toBe(20);
  // Too narrow for the context itself, it is cut and the left is gone.
  expect(line(5)).toEqual({ left: [], context: expect.objectContaining({ text: "23k/…" }) });
});

test("thinking is named after the effort only while it is on", () => {
  const model = (thinking?: boolean) =>
    shown(stripParts({ session: "w", status: { ...base, thinking } }, 200, now));
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
  const activity = (s: Session) => stripParts({ session: "w", status: s }, 200, now).left[1]?.text;
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

// The face's input rule carries the run's work now (brief 60): every pane's
// strip names the session and its context, drawn by the face or not.
test("every pane's strip names the session and its context, under rewake's face too", () => {
  const status = { ...base, type: "code" };
  const texts = (face?: "rewake" | "native") =>
    shown(stripParts({ session: "w", status, face }, 200, now));
  for (const face of ["rewake", "native", undefined] as const) {
    expect(texts(face)[0]).toBe("w (code)");
    expect(texts(face)).toContain("23k/100k");
  }
  expect(texts("rewake")[1]).toBe("dev rewake");
  // No pane's strip says the percent of the window.
  expect(texts(undefined).join(" ")).not.toContain("23%");
});

async function stripAt(state: StripState, width: number) {
  const { renderOnce, captureCharFrame } = await testRender(
    () => (
      <Strip
        state={() => state}
        width={width}
        colour="#ffffff"
        theme={themeOf(undefined)}
        pick={() => {}}
      />
    ),
    { width, height: 1 },
  );
  await renderOnce();
  return captureCharFrame().split("\n")[0] ?? "";
}

test("the drawn strip puts the context against its right end", async () => {
  const state = { session: "w", status: { ...base, cache: { hit: 90 } } };
  expect(await stripAt(state, 40)).toBe(` w · Model low · 90%${" ".repeat(11)}23k/100k `);
  expect(await stripAt(state, 20)).toBe(" w · 90%   23k/100k ");
});
