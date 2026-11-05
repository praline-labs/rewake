import type {
  ExtensionAPI,
  ExtensionContext,
  ThinkingLevel,
} from "@earendil-works/pi-coding-agent";
import { facePrompt, faceThinking } from "./faced.ts";
import type { Frame, Link } from "./protocol.ts";

type PromptTarget = () => { context: ExtensionContext; drawn: boolean } | undefined;
type PromptResult =
  | { kind: "reply"; reply: Partial<Frame> }
  | { kind: "conversation" }
  | { kind: "closed" };

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
  private current: ModePlan | undefined;
  private changing = false;
  private stopped = false;
  private waiting: (() => void)[] = [];

  constructor(
    private pi: ExtensionAPI,
    private context: () => ExtensionContext | undefined,
    private busy: () => boolean,
    private ask: (fields: Omit<Frame, "id">) => Promise<Frame>,
    private state: () => NonNullable<Frame["state"]>,
    private changed: () => void,
    private warn: (text: string) => void,
    private command: (link: Link, request: Frame) => Promise<Partial<Frame>>,
  ) {
    // Input precedes native preflight; custom-message wakes bypass input.
    // The run hook covers both, including an input already in preflight
    // when a switch closed admission. Throwing here would not stop pi.
    pi.on("input", async () => ({
      action: (await this.admit(() => {})) ? "continue" : "handled",
    }));
    pi.on("before_agent_start", async (event, ctx) => {
      if (
        !(await this.admit(() => {
          if (this.current)
            event.systemPromptOptions.sections.rewake_work_mode = this.current.instructions;
        }))
      )
        ctx.abort();
    });
    pi.on("session_before_compact", async () => {
      if (!(await this.admit(() => {}))) return { cancel: true };
    });
  }

  get name(): string | undefined {
    return this.current?.name;
  }

  async initialize(plan?: ModePlan): Promise<void> {
    if (!plan || this.current) return;
    const result = await this.apply(plan, true);
    if (result.error) this.warn(result.error);
  }

  async prompt(text: unknown, target: PromptTarget): Promise<PromptResult> {
    let result: PromptResult = { kind: "closed" };
    await this.admit(() => {
      const addressed = target();
      result = addressed
        ? {
            kind: "reply",
            reply: facePrompt(
              this.pi,
              addressed.drawn,
              addressed.context.isIdle(),
              this.busy(),
              text,
            ),
          }
        : { kind: "conversation" };
    });
    return result;
  }

  wake(notice: unknown): Partial<Frame> {
    if (typeof notice !== "string" || !notice) return { error: "a wake needs a notice" };
    if (this.blocked) return { pending: true };
    if (this.busy()) return { pending: true, error: "compaction in progress" };
    const steered = !this.context()?.isIdle();
    try {
      this.pi.sendMessage(
        { customType: "rewake_notice", content: notice, display: true },
        { triggerTurn: true, deliverAs: "steer" },
      );
      return { ok: true, steered };
    } catch {
      return { error: "the harness did not accept the notice" };
    }
  }

  control(link: Link, request: Frame): Promise<Partial<Frame>> {
    if (this.changing) return Promise.resolve({ outcome: "refused", code: "busy" });
    return this.command(link, request);
  }

  private get blocked(): boolean {
    return this.stopped || !this.current || this.changing;
  }

  // Start closed, and close synchronously before the first native mutation.
  // Execute the caller in the admission check's continuation, not after an
  // extra await that could allow another switch to overtake it.
  private async admit(run: () => void): Promise<boolean> {
    while (!this.stopped && this.blocked) {
      await new Promise<void>((resolve) => this.waiting.push(resolve));
    }
    if (this.stopped) return false;
    run();
    return true;
  }

  private release() {
    for (const resume of this.waiting.splice(0)) resume();
  }

  private stop(ctx: ExtensionContext | undefined) {
    if (this.stopped) return;
    this.stopped = true;
    this.release();
    ctx?.shutdown();
  }

  private async apply(plan: ModePlan, initial = false): Promise<Partial<Frame>> {
    const ctx = this.context();
    if (!ctx || this.stopped || this.changing || !ctx.isIdle() || this.busy()) {
      if (initial) this.stop(ctx);
      return { error: plan.busy };
    }
    this.changing = true;
    const previous = this.pi.getThinkingLevel();
    const previousModel = ctx.model;
    const previousMode = this.current;
    try {
      if (plan.model && plan.provider) {
        const model = ctx.modelRegistry.find(plan.provider, plan.model);
        if (!model || !(await this.pi.setModel(model))) throw new Error(plan.failed);
      }
      // setModel may select its own thinking default. An unset destination
      // effort preserves the previous level, subject to native clamping.
      this.pi.setThinkingLevel((plan.effort ?? previous) as ThinkingLevel);
      this.current = plan;
      if (ctx.hasUI) ctx.ui.setStatus("rewake_work_mode", plan.status);
      this.changed();
      return { ok: true };
    } catch {
      const committed = this.current !== previousMode;
      this.current = previousMode;
      // Native model selection can mutate before its awaited handlers end.
      // Keep admission closed through restoration too; never resume a turn
      // with a refused destination's model and the old instructions.
      try {
        const mutated = ctx.model !== previousModel || this.pi.getThinkingLevel() !== previous;
        if (
          ctx.model !== previousModel &&
          (!previousModel || !(await this.pi.setModel(previousModel)))
        )
          throw new Error(plan.failed);
        if (this.pi.getThinkingLevel() !== previous) this.pi.setThinkingLevel(previous);
        if (committed && previousMode && ctx.hasUI)
          ctx.ui.setStatus("rewake_work_mode", previousMode.status);
        if (mutated || committed) this.changed();
      } catch {
        this.stop(ctx);
      }
      return { error: plan.failed };
    } finally {
      this.changing = false;
      if (initial && !this.current) this.stop(ctx);
      this.release();
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

  async thinking(levels?: string[]): Promise<Partial<Frame>> {
    try {
      const reply: Partial<Frame> = levels
        ? { ok: levels.length > 0, levels }
        : await this.ask({ op: "thinking-cycle", state: this.state() });
      if (!reply.ok || !reply.levels?.length) {
        if (reply.error) this.warn(reply.error);
        return reply;
      }
      const result = this.changeThinking(reply.levels);
      if (result.error) this.warn(result.error);
      return result;
    } catch {
      if (this.current) this.warn(this.current.failed);
      return { ok: false };
    }
  }

  private changeThinking(levels: string[]): Partial<Frame> {
    const ctx = this.context();
    if (!ctx || !this.current || this.stopped || this.changing || this.busy())
      return { error: this.current?.busy };
    this.changing = true;
    const previous = this.pi.getThinkingLevel();
    try {
      if (!faceThinking(this.pi, true, levels).ok) throw new Error(this.current.failed);
      this.changed();
      return { ok: true };
    } catch {
      try {
        this.pi.setThinkingLevel(previous);
      } catch {
        this.stop(ctx);
      }
      return { error: this.current.failed };
    } finally {
      this.changing = false;
      this.release();
    }
  }
}
