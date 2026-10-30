// The asks tests' world: a worker whose permission requests main answers.
import type { Reply } from "./world.ts";
import { start, world } from "./world.ts";

export const opened: Reply = { status: 200, text: '{"id":"ab12"}' };
export const pending: Reply = { status: 200, text: '{"state":"pending"}' };

// answered is main's answer to the dialog's one question, read from what the
// mod opened.
export function answered(answer: string, reason?: string): Reply {
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
export function engine(on: any, run: ReturnType<typeof world>, local?: string, waitMs = 10 ** 9) {
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
export function keyed(run: ReturnType<typeof world>, reply: Reply): Reply {
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
export async function worker(
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
    replies: {
      ...replies,
      "/v1/ask/open": [queue("/v1/ask/open")],
      "/v1/ask/poll": [queue("/v1/ask/poll")],
    },
  });
  engine(on, run, local, waitMs);
  await start($);
  await $.classic.SessionStart({ source: "startup", permission_mode: mode });
  return run;
}

export const settled = (run: ReturnType<typeof world>) =>
  run.sent.filter((one) => one.path === "/v1/ask/settle").map((one) => one.body.winner);
