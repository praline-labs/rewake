// rewake's tools, as data (docs/worktree-tools.md): the wrapper sends the
// table the run is offered in the hello's answer, and again after an
// adoption, and the extension registers what it lists; registering a name
// again replaces its tool, and a name the table drops is registered again
// hidden. A call is planned by the wrapper, which checks the arguments and
// answers the argv and the bound; the extension runs it through pi's exec
// with the call's abort, so Esc stops it as any call of the session's, and
// sends the raw end, which the wrapper words. pi's exec takes no environment:
// the command inherits pi's, which carries the run's variables. What the
// model reads of a tool is the wrapper's: a change reaches a running session
// with the wrapper's restart.
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

export type ToolEntry = {
  name: string;
  label: string;
  description: string;
  inputSchema: unknown;
};

export type ToolRan = {
  exitCode?: number;
  stdout: string;
  stderr: string;
  killed?: boolean;
  stdoutCut?: boolean;
  error?: string;
};

// ToolFields are a frame's fields of the tools: the table, in hello's answer
// and a tools frame; a call's plan and its raw end, and their answers.
export type ToolFields = {
  tools?: ToolEntry[];
  tool?: string;
  arguments?: unknown;
  argv?: string[];
  timeoutMs?: number;
  refused?: string;
  ran?: ToolRan;
  isError?: boolean;
};

type Answer = {
  argv?: string[];
  timeoutMs?: number;
  refused?: string;
  text?: string;
  isError?: boolean;
  error?: string;
};

// Ask sends a request of the extension's to the wrapper and answers its reply.
export type Ask = (fields: {
  op: string;
  tool: string;
  arguments?: unknown;
  ran?: ToolRan;
}) => Promise<Answer>;

// unanswered is the one result the extension words itself: the wrapper did
// not answer, so nothing else can be said of the call.
export const unanswered = (why: string) => `rewake could not answer this call: ${why}`;

// maxOutput bounds the output a result carries, within the frame's 4 MiB; a
// longer one is cut and marked cut, which the wrapper words as a cut answer.
export const maxOutput = 1 << 20;

// fit cuts a raw end to what a frame carries: the error output to 4096
// characters, the output at the last whole character within maxOutput bytes.
export function fit(ran: ToolRan): ToolRan {
  const stderr = Array.from(ran.stderr).slice(0, 4096).join("");
  const bytes = Buffer.from(ran.stdout, "utf8");
  if (bytes.length <= maxOutput) return stderr === ran.stderr ? ran : { ...ran, stderr };
  let end = maxOutput;
  // A continuation byte is no character's start: step back to one.
  while (end > 0 && (bytes[end] & 0xc0) === 0x80) end--;
  return { ...ran, stderr, stdout: bytes.subarray(0, end).toString("utf8"), stdoutCut: true };
}

function message(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

// Tools is the table the extension registered from.
export class Tools {
  // held is each name's entry as registered, and whether it was hidden.
  private held = new Map<string, { shape: string; entry: ToolEntry; hidden: boolean }>();

  constructor(
    private readonly pi: ExtensionAPI,
    private readonly ask: Ask,
  ) {}

  // take registers what a table adds or changes, and hides what it drops.
  take(entries: ToolEntry[] | undefined) {
    if (!Array.isArray(entries)) return;
    const named = new Set<string>();
    for (const entry of entries) {
      named.add(entry.name);
      const shape = JSON.stringify(entry);
      const held = this.held.get(entry.name);
      if (held !== undefined && held.shape === shape && !held.hidden) continue;
      this.register(entry, false);
      this.held.set(entry.name, { shape, entry, hidden: false });
    }
    for (const [name, held] of this.held) {
      if (named.has(name) || held.hidden) continue;
      this.register(held.entry, true);
      held.hidden = true;
    }
  }

  private register(entry: ToolEntry, hidden: boolean) {
    this.pi.registerTool({
      name: entry.name,
      label: entry.label,
      description: entry.description,
      parameters: entry.inputSchema as never,
      ...(hidden ? { exposure: "hidden" as const } : {}),
      execute: async (_id, params, signal, _onUpdate, ctx) => {
        let result: { text: string; isError: boolean };
        try {
          result = await this.call(entry.name, params, signal, ctx.cwd);
        } catch (error) {
          result = { isError: true, text: unanswered(message(error)) };
        }
        return {
          content: [{ type: "text", text: result.text }],
          details: undefined,
          isError: result.isError,
        };
      },
    });
  }

  // call plans a call, runs it unless its abort came first, and has its end
  // worded: an aborted call runs nothing, so a cancelled tick writes nothing.
  private async call(name: string, params: unknown, signal: AbortSignal | undefined, cwd: string) {
    const plan = await this.ask({ op: "tool-plan", tool: name, arguments: params ?? {} });
    if (plan.error !== undefined) throw new Error(plan.error);
    if (plan.refused !== undefined || !plan.argv?.length) {
      return { isError: true, text: plan.refused ?? unanswered("no command") };
    }
    let ran: ToolRan;
    if (signal?.aborted) {
      ran = { stdout: "", stderr: "", killed: true };
    } else {
      try {
        const done = await this.pi.exec(plan.argv[0], plan.argv.slice(1), {
          cwd,
          signal,
          timeout: plan.timeoutMs,
        });
        ran = { exitCode: done.code, stdout: done.stdout, stderr: done.stderr };
        if (done.killed) ran.killed = true;
      } catch (error) {
        ran = { stdout: "", stderr: "", error: message(error) };
      }
    }
    const answer = await this.ask({ op: "tool-result", tool: name, ran: fit(ran) });
    if (answer.error !== undefined) throw new Error(answer.error);
    return { isError: answer.isError === true, text: answer.text ?? "" };
  }
}
