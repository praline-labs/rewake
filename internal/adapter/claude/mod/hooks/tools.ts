// rewake's tools, as data (docs/worktree-tools.md): the wrapper sends the
// table the run is offered in its hello's answer, and again in a poll's when
// the load holds another, and the mod registers what it lists. A call is
// planned by the wrapper, which checks the arguments and answers the argv, the
// run's variables and the bound; the mod runs it natively, so Esc stops it as
// any call of the session's, and posts the raw end, which the wrapper words.
// worktree_enter's result names a checkout, which the mod moves the session
// into with Claude Code's own EnterWorktree, then posts what came of it.
// What the model reads of a tool is the wrapper's: a change reaches a running
// session with the wrapper's restart.
import type { Register } from "claude-code";
import type {
  NativeCall,
  ToolAnswer,
  ToolEntry,
  ToolNativeRequest,
  ToolPlan,
  ToolPlanRequest,
  ToolRan,
  ToolResultRequest,
} from "../protocol.gen.ts";
import { bounds, paths } from "../protocol.gen.ts";
import type { Host } from "./outbox.ts";
import { utf8Length } from "./outbox.ts";

// prefix is what the harness puts before a plugin's tool's name.
export const prefix = "mcp__rewake__";

// Registration is what $.tool.register takes.
export type Registration = Pick<ToolEntry, "name" | "description" | "inputSchema">;

// Tools is the table the load registered from.
export class Tools {
  digest: string | undefined;
  private entries = new Map<string, ToolEntry>();
  // registered is each name's schema as registered, which a new table
  // registers again only when it changed.
  private registered = new Map<string, string>();

  // take registers what a table adds or reshapes and says whether a
  // description of a tool already registered changed, for the describe hook
  // to be asked again. A tool the table drops stays registered: the
  // wrapper's plan refuses its calls.
  async take(
    tools: ToolEntry[] | undefined,
    digest: string | undefined,
    register: (tool: Registration) => Promise<unknown>,
  ): Promise<boolean> {
    if (tools === undefined || digest === undefined || digest === this.digest) return false;
    let described = false;
    for (const tool of tools) {
      const schema = JSON.stringify(tool.inputSchema);
      const before = this.entries.get(tool.name);
      if (this.registered.get(tool.name) !== schema) {
        await register({
          name: tool.name,
          description: tool.description,
          inputSchema: tool.inputSchema,
        }).catch(() => undefined);
        this.registered.set(tool.name, schema);
      } else if (before !== undefined && before.description !== tool.description) {
        described = true;
      }
      this.entries.set(tool.name, tool);
    }
    this.digest = digest;
    return described;
  }

  // description is what the model reads of a tool, by its name as the
  // model sees it.
  description(tool: string): string | undefined {
    return tool.startsWith(prefix)
      ? this.entries.get(tool.slice(prefix.length))?.description
      : undefined;
  }
}

// unanswered is the one result the mod words itself: the wrapper did not
// answer, so nothing else can be said of the call.
export const unanswered = (why: string) => `rewake could not answer this call: ${why}`;

// Waits between attempts of a request the wrapper did not take, in ms: a
// switch's exec leaves the socket without a server for a moment, and a hook
// pays its own waits.
const retries = [250, 500, 1000, 2000];

// ask posts a request until the wrapper answers it, sending one refused in a
// switch's freeze (503) or met with no server again; any other refusal is
// final and thrown with its text.
export async function ask<T>(host: Host, path: string, request: unknown): Promise<T> {
  let why = "";
  for (let attempt = 0; attempt <= retries.length; attempt++) {
    if (attempt > 0) await new Promise<void>((done) => host.after(retries[attempt - 1], done));
    try {
      const answer = await host.post(path, request);
      if (answer.ok) return JSON.parse(answer.text) as T;
      why = `${answer.status} ${answer.text.trim()}`;
      if (answer.status !== 503) break;
    } catch (error) {
      why = error instanceof Error ? error.message : String(error);
    }
  }
  throw new Error(why);
}

// fit keeps a result's body within the endpoint's bound: the error output
// first, then the output, cut at the last whole character that fits and
// marked cut, which the wrapper words as a cut answer.
export function fit(request: ToolResultRequest): ToolResultRequest {
  const bound = bounds.maxEndBodyBytes;
  const size = (r: ToolResultRequest) => utf8Length(JSON.stringify(r));
  if (size(request) <= bound) return request;
  let ran: ToolRan = {
    ...request.ran,
    stderr: Array.from(request.ran.stderr).slice(0, 4096).join(""),
  };
  const chars = Array.from(ran.stdout);
  const kept = (count: number): ToolResultRequest => ({
    ...request,
    ran: { ...ran, stdout: chars.slice(0, count).join(""), stdoutCut: true },
  });
  let low = 0;
  let high = chars.length;
  while (low < high) {
    const middle = Math.ceil((low + high) / 2);
    if (size(kept(middle)) <= bound) low = middle;
    else high = middle - 1;
  }
  ran = kept(low).ran;
  return { ...request, ran };
}

// within says whether dir is path or lies in it.
export function within(dir: string, path: string): boolean {
  const base = path.replace(/\/+$/, "");
  return dir === base || dir.startsWith(base + "/");
}

// The keys a call carries beside the tool's own arguments.
const reserved = new Set(["tool", "tool_use_id", "agentId", "requestMeta", "consent"]);

function shaped(answer: ToolAnswer) {
  return answer.isError ? { isError: true as const, result: answer.text } : { result: answer.text };
}

function message(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

// serveTools answers every call of rewake's tools, and their descriptions
// by the table. loaded is the admitted load's transport, undefined before
// the hello's answer.
export function serveTools(
  on: Parameters<Register>[0],
  tools: Tools,
  loaded: () => { host: Host; loadId: string } | undefined,
) {
  on("tool.describe", { tool: /^mcp__rewake__/ }, async ($, e, next) => {
    const described = await next(e);
    const description = tools.description(e.tool);
    return description === undefined ? described : { ...described, description };
  });

  on("tool.call", { tool: /^mcp__rewake__/ }, async ($, e) => {
    const load = loaded();
    if (load === undefined) {
      return {
        isError: true,
        result: unanswered("this session's load has no answer from rewake yet"),
      };
    }
    const name = e.tool.slice(prefix.length);
    const args = Object.fromEntries(
      Object.entries(e as Record<string, unknown>).filter(([key]) => !reserved.has(key)),
    );
    const planned: ToolPlanRequest = { loadId: load.loadId, tool: name, arguments: args };
    const plan = await ask<ToolPlan>(load.host, paths.toolPlan, planned);
    if (plan.refused !== undefined || plan.argv === undefined) {
      return { isError: true, result: plan.refused ?? unanswered("no command") };
    }
    let ran: ToolRan;
    try {
      const done = await $.process.run(plan.argv, {
        cwd: await $.session.cwd(),
        env: Object.fromEntries((plan.env ?? []).map((v) => [v.name, v.value])),
        timeoutMs: plan.timeoutMs,
      });
      ran = { stdout: done.stdout, stderr: done.stderr };
      if (done.exitCode !== null) ran.exitCode = done.exitCode;
      if (done.isStdoutTruncated) ran.stdoutCut = true;
    } catch (error) {
      ran = { stdout: "", stderr: "", error: message(error) };
    }
    const result = fit({ loadId: load.loadId, tool: name, ran });
    const answer = await ask<ToolAnswer>(load.host, paths.toolResult, result);
    if (answer.enterWorktree === undefined) return shaped(answer);

    // The move: the checkout is the run's from here, whatever comes of it.
    const path = answer.enterWorktree;
    const inside = within(await $.session.root(), path);
    let moved: NativeCall | undefined;
    if (!inside) {
      try {
        const call = await $.tool.call({ tool: "EnterWorktree", path });
        moved = {};
        if (call.deny !== undefined) moved.deny = call.deny;
        if (call.isError) moved.isError = true;
        if (call.text !== undefined) moved.text = call.text;
      } catch (error) {
        moved = { error: message(error) };
      }
    }
    const native: ToolNativeRequest = {
      loadId: load.loadId,
      tool: name,
      made: answer.text,
      path,
      inside,
      moved,
    };
    return shaped(await ask<ToolAnswer>(load.host, paths.toolNative, native));
  }).catch((_$, _e, next) => ({ isError: true, result: unanswered(next.error.message) }));
}
