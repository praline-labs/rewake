import { expect, test } from "bun:test";
import { faced } from "./facekit.ts";

test("work mode is distinct from permission mode and survives a fresh face snapshot", () => {
  const f = faced({
    begin: { workMode: "review", level: "high" },
    inputs: ["prompt", "thinking", "workmode"],
  });
  f.tick();
  expect(f.frame()).toMatchObject({ workMode: "review", level: "high" });
  expect(f.frame().mode).toBeUndefined();
  f.take({ kind: "workmode", id: "mode-1" });
  expect(f.heard).toContainEqual({ id: "mode-1", kind: "workmode", text: "" });
  f.send({ op: "stance", workMode: "advisor" });
  f.tick();
  expect(f.frame().workMode).toBe("advisor");
});

test("thinking visibility changes the drawing, not the thinking level or mode", () => {
  const f = faced({
    items: [{ id: "t", kind: "thinking", status: "settled", text: "a summary" }],
    begin: { workMode: "build", level: "low" },
  });
  f.tick();
  expect(f.shown().join("\n")).toContain("a summary");
  f.take({ kind: "thinkingVisible" });
  f.tick();
  expect(f.shown().join("\n")).not.toContain("a summary");
  expect(f.frame()).toMatchObject({ workMode: "build", level: "low" });
  f.take({ kind: "thinkingVisible" });
  f.tick();
  expect(f.shown().join("\n")).toContain("a summary");
});
