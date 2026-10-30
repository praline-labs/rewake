import { expect, test } from "claude-code/testing";
import { noting, Permits, reasonOf, type Verdict } from "../hooks/permits.ts";
import { answered, keyed, opened, worker } from "./asking.ts";
import type { Reply } from "./world.ts";
import { start, world } from "./world.ts";

// Each decision path of a call runs with the permit recorded: a run the face
// draws, whose permits the mod keeps. A record that fails must leave every
// verdict as it was; the kit runs the mod in a module of its own and freezes
// the realm's built-ins, so the failure is put into permits.ts's methods by
// hand (docs/face.md, "Checked") while these run.
const face: Record<string, Reply[]> = {
  "/v1/hello": [{ status: 200, text: '{"role":"write","generation":1,"mirror":true}' }],
  "/v1/mirror/events": [{ status: 200, text: "{}" }],
};
test("with the permit recorded, main's allow once is an allow", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened], ...face };
  const run = await worker($, on, replies);
  replies["/v1/ask/poll"] = [keyed(run, answered("Allow once"))];
  const verdict = await $.tool.check({
    tool: "Bash",
    input: { command: "make" },
    tool_use_id: "t1",
  } as any);
  expect(verdict).toMatchObject({ decision: "allow" });
});

test("with the permit recorded, main's refusal is a refusal with its reason", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened], ...face };
  const run = await worker($, on, replies);
  replies["/v1/ask/poll"] = [keyed(run, answered("Refuse", "not on this branch"))];
  const verdict = await $.tool.check({
    tool: "Bash",
    input: { command: "make" },
    tool_use_id: "t1",
  } as any);
  expect(verdict).toMatchObject({ decision: "deny" });
  expect((verdict as any).reason).toContain("not on this branch");
});

test("with the permit recorded, a call allowed for the session is allowed", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened], ...face };
  const run = await worker($, on, replies);
  replies["/v1/ask/poll"] = [keyed(run, answered("Allow for this session"))];
  const call = { tool: "Bash", input: { command: "make" } };
  expect(await $.tool.check({ ...call, tool_use_id: "t1" } as any)).toMatchObject({
    decision: "allow",
  });
  expect(await $.tool.check({ ...call, tool_use_id: "t2" } as any)).toMatchObject({
    decision: "allow",
  });
});

test("with the permit recorded, main's answer at the engine's dialog is the answer", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened], ...face };
  const run = await worker($, on, replies, undefined, undefined, "auto");
  const read = { tool: "Read", input: { file_path: "/main/brief.md" } };
  const drawn = { tool_name: "Read", tool_input: read.input, permission_mode: "auto" };
  expect(await $.tool.check({ ...read, tool_use_id: "t1" } as any)).toEqual({ decision: "ask" });
  replies["/v1/ask/poll"] = [keyed(run, answered("Allow once"))];
  expect(await $.classic.PermissionRequest(drawn as any)).toMatchObject({
    decision: { behavior: "allow" },
  });
  replies["/v1/ask/poll"] = [keyed(run, answered("Refuse", "read your own checkout"))];
  expect(await $.classic.PermissionRequest(drawn as any)).toMatchObject({
    decision: { behavior: "deny" },
  });
});

test("with the permit recorded, an engine's verdict and its call's result stand", async ($, on) => {
  const run = world(on, { replies: face });
  on("tool.check", (_: any, e: any) => ({ decision: e.tool === "Glob" ? "deny" : "allow" }));
  on("classic.SessionStart", () => ({}));
  on("classic.PermissionDenied", () => ({}));
  on("tool.call", () => ({ result: "ok", text: "ok" }));
  await start($);
  await run.clock.settle();
  await $.classic.SessionStart({ source: "startup", permission_mode: "auto" } as any);
  expect(await $.tool.check({ tool: "Glob", input: {}, tool_use_id: "g" } as any)).toMatchObject({
    decision: "deny",
  });
  expect(await $.tool.check({ tool: "Read", input: {}, tool_use_id: "r" } as any)).toMatchObject({
    decision: "allow",
  });
  await $.classic.PermissionDenied({ tool_name: "Read", tool_use_id: "r", reason: "x" } as any);
  expect(
    await $.tool.call({ tool: "Read", file_path: "/work/a", tool_use_id: "r" } as any),
  ).toMatchObject({ result: "ok" });
  await run.clock.advance(100);
  const results = run.sent
    .filter((one) => one.path === "/v1/mirror/events")
    .flatMap((one) => one.body.events)
    .filter((e: any) => e.kind === "result")
    .map(({ permit: _, ...result }: any) => result);
  expect(results).toEqual([{ kind: "result", id: "r", status: "done", text: "ok" }]);
});

test("a permit tells who let a call run or refused it, as the terminal program tells it", () => {
  const permits = new Permits();
  const check = (tool: string, input: object, id: string, verdict: Verdict, cwd?: string) =>
    permits.checked({ tool, input, tool_use_id: id } as any, verdict, cwd);
  const ask = { decision: "ask" };
  check("Read", { file_path: "/work/a" }, "r", { decision: "allow" });
  check("Glob", { pattern: "*" }, "g", { decision: "deny", rule: "Glob" });
  check("Bash", { command: "pre" }, "h", { decision: "allow", hook: "pre" });
  check("Bash", { command: "make" }, "b", ask, "/work");
  check("Edit", { file_path: "/work/a" }, "e", ask, "/work");
  check("Edit", { file_path: "/workshop/a" }, "o", ask, "/work");
  check("WebFetch", { url: "u" }, "w", ask, "/work");
  check("WebFetch", { url: "v" }, "d", ask, "/work");
  check("Bash", { command: "rm" }, "p", ask, "/work");
  check("Bash", { command: "rm -r" }, "q", ask, "/work");
  check("Bash", { command: "ls" }, "x", ask, "/work");
  check("Bash", { command: "ls" }, "y", ask, "/work");
  permits.denied("d", "out of scope");
  permits.dialog({ tool_name: "Bash", tool_input: { command: "rm" } } as any, "person");
  permits.answered("x", "rewake", true);
  permits.answered("y", "rewake", false);
  const refusal =
    "Permission for this action was denied by the Claude Code auto mode classifier. Reason: [Scope]. If you have other tasks, go on.";
  const take = (id: string, ended: object = {}, mode = "auto") => permits.take(id, ended, mode);
  expect(take("r")).toEqual({ by: "mode", allowed: true });
  expect(take("g", { refused: "no" })).toEqual({ by: "rule", allowed: false });
  expect(take("h")).toEqual({ by: "hook", allowed: true });
  expect(take("b")).toEqual({ by: "classifier", allowed: true });
  expect(take("e")).toEqual({ by: "mode", allowed: true });
  expect(take("o")).toEqual({ by: "classifier", allowed: true });
  expect(take("w", { failed: refusal })).toEqual({
    by: "classifier",
    allowed: false,
    reason: "[Scope] ∙ see https://code.claude.com/docs/s/claude-code-auto-mode",
  });
  expect(take("d", { refused: "denied" })).toEqual({
    by: "classifier",
    allowed: false,
    reason: "out of scope ∙ see https://code.claude.com/docs/s/claude-code-auto-mode",
  });
  expect(take("p")).toEqual({ by: "person", allowed: true });
  expect(
    take("q", {
      failed: "The user doesn't want to proceed with this tool use. The tool use was rejected.",
    }),
  ).toEqual({
    by: "person",
    allowed: false,
  });
  expect(take("x")).toEqual({ by: "rewake", allowed: true });
  expect(take("y", { refused: "refused" })).toEqual({ by: "rewake", allowed: false });
  expect(take("r")).toBeUndefined();
  check("Bash", { command: "make" }, "m", ask, "/work");
  expect(take("m", {}, "default")).toEqual({ by: "mode", allowed: true });
  permits.checked({ tool: "Bash", input: {}, tool_use_id: "s", agentId: "a1" } as any, ask);
  expect(take("s")).toBeUndefined();
  // A long reason is cut so the link survives the face's short field.
  check("WebFetch", { url: "l" }, "l", ask, "/work");
  permits.denied("l", "x".repeat(400));
  const long = take("l", { refused: "denied" })?.reason ?? "";
  expect(new TextEncoder().encode(long).length <= 256).toBe(true);
  expect(long.endsWith("… ∙ see https://code.claude.com/docs/s/claude-code-auto-mode")).toBe(true);
  check("WebFetch", { url: "n" }, "n", ask, "/work");
  expect(
    take("n", { failed: "Permission for this action has been denied. Reason: x" })?.reason,
  ).toBe("see https://code.claude.com/docs/s/claude-code-auto-mode");
  expect(reasonOf("Permission for this action has been denied. Reason: x")).toEqual({});
  expect(reasonOf("ok")).toBeUndefined();
});

test("a record that fails is lost, and nothing is thrown", () => {
  expect(() =>
    noting(() => {
      throw new Error("the record failed");
    }),
  ).not.toThrow();
});
