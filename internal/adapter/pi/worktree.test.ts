// The worktree_status tool's contract with pi, run by go test (worktree_tool_test.go).
import { expect, test } from "bun:test";
import { landTimeoutMs, listArgs, statusArgs, statusTimeoutMs, worktreeTools } from "./worktree.ts";

const status = '{"session":"api-pi","checkout":null,"close":{"outcome":"none"}}';

// stage registers the tool on a pi that records each exec and answers it
// with ran, and returns the tool and the calls.
function stage(
  ran: () => Promise<any>,
  executable: string | undefined = "/opt/rewake/bin/rewake",
  role: string | undefined = undefined,
) {
  const tools: any[] = [];
  const execs: any[] = [];
  const pi: any = {
    registerTool: (tool: any) => tools.push(tool),
    exec: (command: string, args: string[], options: any) => {
      execs.push({ command, args, options });
      return ran();
    },
  };
  worktreeTools(pi, executable, role);
  return { tools, execs };
}

const call = (tool: any) =>
  tool.execute("call-1", {}, undefined, undefined, { cwd: "/work/tree" } as any);

test("the tool runs rewake's status by argv in the session's directory and answers its JSON", async () => {
  const seen = stage(async () => ({ stdout: status + "\n", stderr: "", code: 0, killed: false }));
  expect(seen.tools.map((tool) => tool.name)).toEqual(["worktree_status"]);
  const result = await call(seen.tools[0]);
  expect(result.isError).toBe(false);
  expect(result.content).toEqual([{ type: "text", text: status }]);
  expect(seen.execs).toEqual([
    {
      command: "/opt/rewake/bin/rewake",
      args: statusArgs,
      options: { cwd: "/work/tree", signal: undefined, timeout: statusTimeoutMs },
    },
  ]);
  expect(statusArgs).toEqual(["worktree", "status", "--json"]);
});

test("a refusal, a child killed at the bound and an exec that throws are errors", async () => {
  const results = [
    async () => ({
      stdout: "",
      stderr: "rewake: status answers for the session it runs in",
      code: 2,
      killed: false,
    }),
    // pi reports a child it killed with code 0.
    async () => ({ stdout: "", stderr: "", code: 0, killed: true }),
    async () => {
      throw new Error("spawn ENOENT");
    },
  ];
  const seen = stage(() => results.shift()!());
  const refused = await call(seen.tools[0]);
  expect(refused.isError).toBe(true);
  expect(refused.content[0].text).toContain("exit 2");
  const killed = await call(seen.tools[0]);
  expect(killed.isError).toBe(true);
  expect(killed.content[0].text).toContain("did not run to its end");
  const thrown = await call(seen.tools[0]);
  expect(thrown.isError).toBe(true);
  expect(thrown.content[0].text).toContain("spawn ENOENT");
});

test("a pi whose rewake sets no executable registers no tool", () => {
  expect(stage(async () => ({}), "").tools).toEqual([]);
});

const asMain = (ran: () => Promise<any>) => stage(ran, "/opt/rewake/bin/rewake", "main");
const byName = (seen: { tools: any[] }, name: string) =>
  seen.tools.find((tool) => tool.name === name);
const ok = async () => ({ stdout: "{}", stderr: "", code: 0, killed: false });

test("main is offered list and land beside status, anyone else status alone", () => {
  expect(asMain(ok).tools.map((tool) => tool.name)).toEqual([
    "worktree_status",
    "worktree_list",
    "worktree_land",
  ]);
  expect(stage(ok, "/opt/rewake/bin/rewake", "write").tools.map((tool) => tool.name)).toEqual([
    "worktree_status",
  ]);
});

test("list asks for this room's checkouts, land passes the name and the branch as single arguments with a longer bound", async () => {
  const seen = asMain(ok);
  await call(byName(seen, "worktree_list"));
  await seen.tools[2].execute("c", { name: "fix-login", into: "release" }, undefined, undefined, {
    cwd: "/work/tree",
  } as any);
  await seen.tools[2].execute("c", { name: "fix-login" }, undefined, undefined, {
    cwd: "/work/tree",
  } as any);
  expect(seen.execs.map((one) => one.args)).toEqual([
    listArgs,
    ["worktree", "land", "fix-login", "--into=release", "--json"],
    ["worktree", "land", "fix-login", "--json"],
  ]);
  expect(seen.execs.map((one) => one.options.timeout)).toEqual([
    statusTimeoutMs,
    landTimeoutMs,
    landTimeoutMs,
  ]);
  expect(landTimeoutMs).toBeGreaterThan(60_000);
});

test("land refuses a name or a branch that would be read as a flag, and shows a refusal as an error", async () => {
  const seen = asMain(async () => ({
    stdout: "",
    stderr: "main moved on",
    code: 1,
    killed: false,
  }));
  const land = byName(seen, "worktree_land");
  for (const input of [
    {},
    { name: "" },
    { name: "--force" },
    { name: "x", into: "-f" },
    { name: "x", into: "" },
    { name: 3 },
  ]) {
    const result = await land.execute("c", input, undefined, undefined, { cwd: "/w" } as any);
    expect(result.isError).toBe(true);
  }
  expect(seen.execs).toEqual([]);
  const refused = await land.execute("c", { name: "x" }, undefined, undefined, {
    cwd: "/w",
  } as any);
  expect(refused.isError).toBe(true);
  expect(refused.content[0].text).toContain("rewake worktree land failed (exit 1): main moved on");
});
