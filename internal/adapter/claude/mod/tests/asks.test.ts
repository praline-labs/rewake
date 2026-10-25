import { expect, test } from "claude-code/testing";
import { unanswered } from "../hooks/asks.ts";
import type { Reply } from "./world.ts";
import { start, world } from "./world.ts";

const opened: Reply = { status: 200, text: '{"id":"ab12","lifetimeMs":120000}' };
const pending: Reply = { status: 200, text: '{"state":"pending"}' };

// answered is main's answer to the dialog's one question, read from what the
// mod opened.
function answered(answer: string, reason?: string): Reply {
  return {
    status: 200,
    text: JSON.stringify({
      state: "answered",
      answers: [{ question: "?", answer }],
      ...(reason ? { reason } : {}),
    }),
  };
}

// engine stands for what is below the mod: the verdict a check comes to, the
// session's directory, and the question dialog, which the person answers
// with local after waitMs, or never.
function engine(on: any, run: ReturnType<typeof world>, local?: string, waitMs = 10 ** 9) {
  on("tool.check", () => ({ decision: "ask" }));
  on("session.cwd", () => ({ value: "/work" }));
  on("classic.SessionStart", () => ({}));
  on("classic.PermissionRequest", () => ({}));
  on("classic.PostToolUse", () => ({}));
  on("classic.Notification", () => ({}));
  on("tool.call", async (_: any, e: any) => {
    await run.clock.sleep(waitMs);
    return { result: { questions: e.questions, answers: { [e.questions[0].question]: local } } };
  });
}

// The poll answers carry the question the mod opened, as the wrapper's do.
function keyed(run: ReturnType<typeof world>, reply: Reply): Reply {
  return async () => {
    if (typeof reply !== "object") return reply;
    const question = run.sent.filter((one) => one.path === "/v1/ask/open").at(-1)?.body
      .questions[0].question;
    return { status: reply.status, text: reply.text.replaceAll('"?"', JSON.stringify(question)) };
  };
}

// worker is a worker's load in the given mode; its polls are answered by
// replies['/v1/ask/poll'] as it stands when each poll comes, so a test may set
// it once it knows the run.
async function worker(
  $: any,
  on: any,
  replies: Record<string, Reply[]>,
  local?: string,
  waitMs?: number,
  mode = "default",
) {
  const queue =
    (path: string): Reply =>
    async (body: any) => {
      const list = replies[path] ?? [{ status: 501, text: '{"error":"not served"}' }];
      const reply = list.length > 1 ? list.shift()! : list[0];
      return typeof reply === "function" ? reply(body) : reply;
    };
  const run = world(on, {
    replies: { "/v1/ask/open": [queue("/v1/ask/open")], "/v1/ask/poll": [queue("/v1/ask/poll")] },
  });
  engine(on, run, local, waitMs);
  await start($);
  await $.classic.SessionStart({ source: "startup", permission_mode: mode });
  return run;
}

const settled = (run: ReturnType<typeof world>) =>
  run.sent.filter((one) => one.path === "/v1/ask/settle").map((one) => one.body.winner);

test("a permission main allows once lets the call run, and main is told of the exact call", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened] };
  const run = await worker($, on, replies);
  replies["/v1/ask/poll"] = [keyed(run, answered("Allow once"))];
  const verdict = await $.tool.check({
    tool: "Bash",
    input: { command: "make test", timeout: 5 },
    tool_use_id: "t1",
  } as any);
  expect(verdict).toMatchObject({ decision: "allow" });
  const open = run.sent.find((one) => one.path === "/v1/ask/open")!.body;
  expect(open).toMatchObject({
    kind: "permission",
    tool: "Bash",
    input: '{"command":"make test","timeout":5}',
    cwd: "/work",
  });
  expect(open.questions[0].options.map((one: any) => one.label)).toEqual([
    "Allow once",
    "Allow for this session",
    "Refuse",
  ]);
  expect(settled(run)).toEqual(["main"]);
});

test("a call allowed for the session passes again without asking, and another call is asked", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened] };
  const run = await worker($, on, replies);
  replies["/v1/ask/poll"] = [keyed(run, answered("Allow for this session"))];
  const call = { tool: "Bash", input: { command: "make" }, tool_use_id: "t1" };
  expect(await $.tool.check(call as any)).toMatchObject({ decision: "allow" });
  expect(
    await $.tool.check({ ...call, input: { command: "make" }, tool_use_id: "t2" } as any),
  ).toMatchObject({ decision: "allow" });
  expect(run.sent.filter((one) => one.path === "/v1/ask/open").length).toBe(1);
  replies["/v1/ask/poll"] = [keyed(run, answered("Refuse", "not on this branch"))];
  const refused = await $.tool.check({
    ...call,
    input: { command: "make clean" },
    tool_use_id: "t3",
  } as any);
  expect(refused).toMatchObject({ decision: "deny" });
  expect((refused as any).reason).toContain("not on this branch");
});

test("the person at the screen answering first decides, and the wrapper hears it", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened] };
  const run = await worker($, on, replies, "Refuse", 1000);
  replies["/v1/ask/poll"] = [
    async () => {
      // The wrapper holds the poll until the settling wakes it.
      while (!settled(run).includes("local")) await run.clock.sleep(100);
      return { status: 200, text: '{"state":"settled"}' };
    },
  ];
  const checking = $.tool.check({
    tool: "Write",
    input: { file_path: "/work/a", content: "x" },
    tool_use_id: "t1",
  } as any);
  await run.clock.advance(2000);
  expect(await checking).toMatchObject({ decision: "deny" });
  expect(settled(run)).toEqual(["local"]);
});

test("a permission nobody answers by the deadline is refused, telling the model not to retry blindly", async ($, on) => {
  const replies: Record<string, Reply[]> = {
    "/v1/ask/open": [opened],
    "/v1/ask/poll": [pending, { status: 200, text: '{"state":"expired"}' }],
  };
  const run = await worker($, on, replies);
  const verdict = await $.tool.check({
    tool: "Bash",
    input: { command: "deploy" },
    tool_use_id: "t1",
  } as any);
  expect(verdict).toEqual({ decision: "deny", reason: unanswered });
  expect(run.sent.filter((one) => one.path === "/v1/ask/poll").length).toBe(2);
  expect(settled(run)).toEqual(["expired"]);
});

test("the model's own question keeps its choices, and main's answer is its result", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened] };
  const run = await worker($, on, replies);
  replies["/v1/ask/poll"] = [keyed(run, answered("Beta, Gamma"))];
  const questions = [
    {
      question: "Which probe option?",
      header: "Probe",
      multiSelect: true,
      options: [{ label: "Alpha", description: "first" }, { label: "Beta" }, { label: "Gamma" }],
    },
  ];
  const result = await $.tool.call({ tool: "AskUserQuestion", questions } as any);
  expect(result).toEqual({
    result: { questions, answers: { "Which probe option?": "Beta, Gamma" } },
  });
  const open = run.sent.find((one) => one.path === "/v1/ask/open")!.body;
  expect(open).toMatchObject({
    kind: "question",
    questions: [{ question: "Which probe option?", header: "Probe", multiSelect: true }],
  });
  expect(open.questions[0].options[0]).toEqual({ label: "Alpha", description: "first" });
  expect(settled(run)).toEqual(["main"]);
});

test("main's own asks stay with the engine", async ($, on) => {
  const run = world(on, {
    replies: { "/v1/hello": [{ status: 200, text: '{"role":"main","generation":1}' }] },
  });
  engine(on, run);
  await start($);
  await $.classic.SessionStart({ source: "startup", permission_mode: "default" });
  expect(
    await $.tool.check({ tool: "Bash", input: { command: "make" }, tool_use_id: "t1" } as any),
  ).toEqual({ decision: "ask" });
  expect(run.sent.some((one) => one.path === "/v1/ask/open")).toBe(false);
});

test("a worker's ask in a mode the person is not asked in stays with the engine", async ($, on) => {
  const run = await worker($, on, {}, undefined, undefined, "auto");
  expect(
    await $.tool.check({ tool: "Bash", input: { command: "make" }, tool_use_id: "t1" } as any),
  ).toEqual({ decision: "ask" });
  expect(run.sent.some((one) => one.path === "/v1/ask/open")).toBe(false);
});

// dialogs is a worker in auto mode, Claude Code's default, whose check left
// an ask of t1 to the engine, which then draws its own dialog.
async function dialogs($: any, on: any, replies: Record<string, Reply[]>) {
  const run = await worker($, on, replies, undefined, undefined, "auto");
  expect(await $.tool.check({ ...read, tool_use_id: "t1" } as any)).toEqual({ decision: "ask" });
  return run;
}

const read = { tool: "Read", input: { file_path: "/main/brief.md" } };
const drawn = { tool_name: "Read", tool_input: read.input, permission_mode: "auto" };

test("in auto mode the engine's own permission dialog is carried to main, and main's answer decides it", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened] };
  const run = await dialogs($, on, replies);
  replies["/v1/ask/poll"] = [keyed(run, answered("Allow once"))];
  expect(await $.classic.PermissionRequest(drawn as any)).toMatchObject({
    decision: { behavior: "allow" },
  });
  expect(run.sent.find((one) => one.path === "/v1/ask/open")!.body).toMatchObject({
    kind: "permission",
    call: "t1",
    tool: "Read",
    input: '{"file_path":"/main/brief.md"}',
    cwd: "/work",
  });
  expect(settled(run)).toEqual(["main"]);
  replies["/v1/ask/poll"] = [keyed(run, answered("Refuse", "read your own checkout"))];
  const refused = await $.classic.PermissionRequest(drawn as any);
  expect(refused).toMatchObject({ decision: { behavior: "deny" } });
  expect((refused as any).decision.message).toContain("read your own checkout");
});

test("a dialog the person answers at the screen first is left to them, and the wrapper hears it", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened] };
  const run = await dialogs($, on, replies);
  replies["/v1/ask/poll"] = [
    async () => {
      while (!settled(run).includes("local")) await run.clock.sleep(100);
      return { status: 200, text: '{"state":"settled"}' };
    },
  ];
  const asking = $.classic.PermissionRequest(drawn as any);
  await run.clock.advance(500);
  await $.classic.PostToolUse({ ...drawn, tool_use_id: "t1", tool_response: {} } as any);
  await run.clock.advance(500);
  expect((await asking) as any).not.toHaveProperty("decision");
  expect(settled(run)).toEqual(["local"]);
});

const dialogsWaited = (run: ReturnType<typeof world>) =>
  run.sent.filter((one) => one.path === "/v1/mod/state").map((one) => one.body.dialog);

test("a dialog no request carries is named in the state until the next tool result, and a carried one is not", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened] };
  const run = await dialogs($, on, replies);
  replies["/v1/ask/poll"] = [
    async () => {
      await run.clock.sleep(1000);
      return keyed(run, answered("Allow once"))({});
    },
  ];
  const asking = $.classic.PermissionRequest(drawn as any);
  await run.clock.advance(500);
  const shown = { notification_type: "permission_prompt", message: "Claude needs your permission" };
  await $.classic.Notification(shown as any);
  await run.clock.advance(1000);
  expect(await asking).toMatchObject({ decision: { behavior: "allow" } });
  await run.clock.settle();
  expect(dialogsWaited(run).filter(Boolean)).toEqual([]);
  await $.classic.Notification(shown as any);
  await run.clock.settle();
  expect(dialogsWaited(run).at(-1)).toBe("Claude needs your permission");
  await $.classic.PostToolUse({ ...drawn, tool_use_id: "t2", tool_response: {} } as any);
  await run.clock.settle();
  expect(dialogsWaited(run).at(-1)).toBeUndefined();
});

// The engine's informational notifications of 2.1.302 name no dialog: a
// plugin's $.ui.notify, a background agent's end or need, computer use's
// end, the prompt's idleness. An MCP server's input dialog does.
test("only a notification of a drawn dialog names one", async ($, on) => {
  const run = await dialogs($, on, {});
  for (const notification_type of [
    "plugin_notification",
    "agent_completed",
    "agent_needs_input",
    "computer_use_exit",
    "idle_prompt",
    "auth_success",
  ]) {
    await $.classic.Notification({ notification_type, message: "not a dialog" } as any);
  }
  await run.clock.settle();
  expect(dialogsWaited(run).filter(Boolean)).toEqual([]);
  const input = { notification_type: "elicitation_dialog", message: "An MCP server needs input" };
  await $.classic.Notification(input as any);
  await run.clock.settle();
  expect(dialogsWaited(run).at(-1)).toBe("An MCP server needs input");
});

const waited = (run: ReturnType<typeof world>) =>
  run.sent.filter((one) => one.path === "/v1/mod/state").map((one) => one.body.waiting);

// held answers a poll after a second with main's answer.
const held =
  (run: ReturnType<typeof world>, answer: string): Reply =>
  async () => {
    await run.clock.sleep(1000);
    return keyed(run, answered(answer))({});
  };

test("a dialog a request carries names what it asks in the state while it is held", async ($, on) => {
  const replies: Record<string, Reply[]> = { "/v1/ask/open": [opened] };
  const run = await dialogs($, on, replies);
  replies["/v1/ask/poll"] = [held(run, "Allow once")];
  const asking = $.classic.PermissionRequest(drawn as any);
  await run.clock.advance(500);
  expect(waited(run).at(-1)).toBe("permission");
  await run.clock.advance(1000);
  expect(await asking).toMatchObject({ decision: { behavior: "allow" } });
  await run.clock.settle();
  expect(waited(run).at(-1)).toBeUndefined();
  replies["/v1/ask/poll"] = [held(run, "Beta")];
  const questions = [{ question: "Which?", options: [{ label: "Alpha" }, { label: "Beta" }] }];
  const asked = $.tool.call({ tool: "AskUserQuestion", questions } as any);
  await run.clock.advance(500);
  expect(waited(run).at(-1)).toBe("question");
  await run.clock.advance(1000);
  await asked;
  await run.clock.settle();
  expect(waited(run).at(-1)).toBeUndefined();
});
