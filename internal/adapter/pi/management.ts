import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import type { Frame, Link, Turn } from "./extension.ts";

// State is what the session read of itself, unconverted: the wrapper names
// and converts it (telemetry.go), so a change of what the Статус shows is the
// wrapper's alone. lastAssistant is the last assistant message's usage and
// whose it was, as pi reported it.
export type State = {
  sequence: number;
  idle: boolean;
  compacting: boolean;
  native: {
    contextUsage: unknown;
    model: unknown;
    thinkingLevel: unknown;
    lastAssistant: unknown;
  };
};
type Pending = {
  frame: Frame;
  link: Link;
  resolve: (answer: Partial<Frame>) => void;
  timer: ReturnType<typeof setTimeout>;
};

// thrown is what a native call threw, as its message.
export function thrown(error: unknown): { error: string } {
  return { error: error instanceof Error ? error.message : String(error) };
}

// modelOf is the model as pi holds it, less what may carry credentials: its
// headers and its address.
export function modelOf(model: object | undefined): unknown {
  if (!model) return null;
  const { headers: _headers, baseUrl: _baseUrl, ...rest } = model as Record<string, unknown>;
  return rest;
}

export function management(
  pi: ExtensionAPI,
  context: () => ExtensionContext | undefined,
  current: () => Link | undefined,
  turn: () => Turn | undefined,
  emit: (link: Link, fields: Omit<Frame, "id">) => void,
) {
  let sequence = 0;
  let compactions = 0;
  let compacting = false;
  let lastAssistant: unknown = null;
  let compact: Pending | undefined;
  // interrupting is the turn an interrupt stopped, in flight until it settles.
  let interrupting: Turn | undefined;

  function sample(): State {
    const ctx = context();
    return {
      sequence: ++sequence,
      idle: ctx?.isIdle() === true,
      compacting: compacting || !!compact,
      native: {
        contextUsage: ctx?.getContextUsage() ?? null,
        model: modelOf(ctx?.model),
        thinkingLevel: ctx?.thinkingLevel ?? null,
        lastAssistant,
      },
    };
  }

  function changed() {
    const link = current();
    if (link?.ready) emit(link, { op: "state", thread: link.thread, state: sample() });
  }

  function answer(pending: Pending, fields: Partial<Frame>) {
    clearTimeout(pending.timer);
    pending.resolve(fields);
  }

  function finished(pending: Pending, fields: Partial<Frame>) {
    if (compact !== pending) return;
    compact = undefined;
    compacting = false;
    if (current() === pending.link) {
      emit(pending.link, {
        op: "compaction-result",
        thread: pending.link.thread,
        request: pending.frame.request,
        from: pending.frame.from,
        ...fields,
      });
      changed();
    }
    answer(pending, fields);
  }

  // A command is carried out natively after the check made just before its
  // call, and answered as it resolved: the wrapper words the reply, waits for
  // an interrupted turn to settle and names who stopped it (control.go).
  function startCompact(link: Link, frame: Frame, ctx: ExtensionContext): Promise<Partial<Frame>> {
    return new Promise((resolve) => {
      const pending: Pending = {
        frame,
        link,
        resolve,
        timer: setTimeout(() => resolve({ outcome: "requested" }), 1000),
      };
      compact = pending;
      changed();
      try {
        // Native compact aborts first. The idle check in control protects a
        // running turn, and holding compact prevents a wake during that await.
        ctx.compact({
          customInstructions: frame.focus || undefined,
          // The result as pi gave it, less the summary it carries.
          onComplete: (result) =>
            finished(pending, {
              outcome: "done",
              native: {
                tokensBefore: result.tokensBefore,
                estimatedTokensAfter: result.estimatedTokensAfter,
              },
            }),
          onError: (error) => finished(pending, { outcome: "failed", native: thrown(error) }),
        });
      } catch (error) {
        finished(pending, { outcome: "failed", native: thrown(error) });
      }
    });
  }

  async function control(link: Link, frame: Frame): Promise<Partial<Frame>> {
    const ctx = context();
    if (!ctx || current() !== link) return { outcome: "refused", code: "conversation" };
    if (compacting || compact || interrupting) return { outcome: "refused", code: "busy" };
    if (frame.action === "compact") {
      if (!ctx.isIdle()) return { outcome: "refused", code: "turn" };
      return startCompact(link, frame, ctx);
    }
    if (frame.action === "interrupt") {
      const running = turn();
      if (!running || running.link !== link || ctx.isIdle())
        return { outcome: "refused", code: "idle" };
      if (frame.turn !== undefined && frame.turn !== running.id)
        return { outcome: "refused", code: "turn" };
      interrupting = running;
      try {
        ctx.abort();
      } catch (error) {
        interrupting = undefined;
        return { outcome: "failed", native: thrown(error) };
      }
      return { outcome: "requested" };
    }
    return { outcome: "refused" };
  }

  // trace puts an interrupt's actor into pi's history without starting a
  // turn, once the wrapper heard the interrupted turn's end.
  function trace(text: string): Partial<Frame> {
    try {
      pi.sendMessage(
        { customType: "rewake_interrupt", content: text, display: true },
        { triggerTurn: false, deliverAs: "steer" },
      );
      return { ok: true };
    } catch (error) {
      return { error: String(error) };
    }
  }

  // A turn's settling frees an interrupt of it; the wrapper matches the end
  // it hears to the interrupt.
  function settled(ended: Turn) {
    if (interrupting === ended) interrupting = undefined;
  }

  function reset() {
    if (compact) answer(compact, { outcome: "failed", code: "closed" });
    compact = undefined;
    interrupting = undefined;
    compacting = false;
    lastAssistant = null;
  }

  pi.on("session_before_compact", async () => {
    compacting = true;
    if (compact) answer(compact, { outcome: "started" });
    changed();
  });
  pi.on("session_compact", async () => {
    const link = current();
    if (link?.ready)
      emit(link, {
        op: "compacted",
        thread: link.thread,
        sequence: ++compactions,
        request: compact?.frame.request,
        from: compact?.frame.from,
      });
    compacting = false;
    changed();
  });
  pi.on("session_compact_failed", async () => {
    compacting = false;
    changed();
  });
  // Each assistant message's usage is sampled at its end, a placeholder's
  // and a failed one's too: the wrapper tells which is the cache.
  pi.on("message_end", async (event) => {
    const message = event.message;
    if (message.role !== "assistant") return;
    const { usage, provider, model, timestamp } = message;
    lastAssistant = { usage, provider, model, timestamp };
    changed();
  });
  pi.on("model_select", async () => changed());
  pi.on("thinking_level_select", async () => changed());
  return { sample, changed, control, trace, settled, reset, busy: () => compacting || !!compact };
}
