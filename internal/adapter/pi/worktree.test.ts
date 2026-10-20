// The worktree_status tool's contract with pi, run by go test (worktree_tool_test.go).
import { expect, test } from "bun:test";
import { statusArgs, statusTimeoutMs, worktreeStatus } from "./worktree.ts";

const status = '{"session":"api-pi","checkout":null,"close":{"outcome":"none"}}';

// stage registers the tool on a pi that records each exec and answers it
// with ran, and returns the tool and the calls.
function stage(ran: () => Promise<any>, executable: string | undefined = "/opt/rewake/bin/rewake") {
  const tools: any[] = [];
  const execs: any[] = [];
  const pi: any = {
    registerTool: (tool: any) => tools.push(tool),
    exec: (command: string, args: string[], options: any) => {
      execs.push({ command, args, options });
      return ran();
    },
  };
  worktreeStatus(pi, executable);
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
