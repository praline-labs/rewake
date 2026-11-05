import type {
  ExtensionAPI,
  ExtensionContext,
  ThinkingLevel,
} from "@earendil-works/pi-coding-agent";
import type { Frame } from "./protocol.ts";

// Every word and destination comes from Go. This bridge only calls native
// model/thinking APIs and holds the policy that was successfully applied.
export type ModePlan = {
  name: string;
  instructions: string;
  status: string;
  provider?: string;
  model?: string;
  effort?: string;
  busy: string;
  failed: string;
};

export class Modes {
  current: ModePlan | undefined;
  changing = false;

  constructor(
    private pi: ExtensionAPI,
    private context: () => ExtensionContext | undefined,
    private busy: () => boolean,
    private ask: (fields: Omit<Frame, "id">) => Promise<Frame>,
    private state: () => NonNullable<Frame["state"]>,
    private changed: () => void,
    private warn: (text: string) => void,
  ) {
    pi.on("before_agent_start", (event) => {
      if (this.current)
        event.systemPromptOptions.sections.rewake_work_mode = this.current.instructions;
    });
  }

  async apply(plan: ModePlan, initial = false): Promise<Partial<Frame>> {
    const ctx = this.context();
    if (!ctx || this.changing || !ctx.isIdle() || this.busy()) {
      return { error: plan.busy };
    }
    this.changing = true;
    try {
      const previous = this.pi.getThinkingLevel();
      if (plan.model && plan.provider) {
        const model = ctx.modelRegistry.find(plan.provider, plan.model);
        if (!model || !(await this.pi.setModel(model))) return { error: plan.failed };
      }
      // setModel may select its own thinking default. An unset destination
      // effort preserves the previous level, subject to native clamping.
      this.pi.setThinkingLevel((plan.effort ?? previous) as ThinkingLevel);
      this.current = plan;
      if (ctx.hasUI) ctx.ui.setStatus("rewake_work_mode", plan.status);
      this.changed();
      return { ok: true };
    } catch {
      return { error: plan.failed };
    } finally {
      this.changing = false;
      if (initial && !this.current) ctx.shutdown();
    }
  }

  async cycle(): Promise<Partial<Frame>> {
    try {
      const reply = await this.ask({ op: "mode-cycle", state: this.state() });
      const result = reply.ok && reply.mode ? await this.apply(reply.mode) : reply;
      if (result.error) this.warn(result.error);
      return result;
    } catch {
      if (this.current) this.warn(this.current.failed);
      return { ok: false };
    }
  }

  async thinking(): Promise<void> {
    try {
      const reply = await this.ask({ op: "thinking-cycle", state: this.state() });
      if (!reply.ok || !reply.levels?.length) {
        if (reply.error) this.warn(reply.error);
        return;
      }
      const levels = reply.levels as ThinkingLevel[];
      const at = levels.indexOf(this.pi.getThinkingLevel());
      const next = levels[(at + 1) % levels.length];
      if (next !== undefined) this.pi.setThinkingLevel(next);
      this.changed();
    } catch {
      if (this.current) this.warn(this.current.failed);
    }
  }
}
