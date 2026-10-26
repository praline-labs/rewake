import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";
import type { Frame, Link, Turn } from "./extension.ts";

export type State = {
  sequence: number;
  activity: "idle" | "working";
  model: string | null;
  modelName: string | null;
  effort: string | null;
  // thinking is whether the model thinks: its thinking level is not off; null
  // for a model without reasoning, whose level means nothing.
  thinking: boolean | null;
  tokens: number | null;
  window: number | null;
  percent: number | null;
  compacting: boolean;
  cache: Cache | null;
};
// ttl is the lifetime in seconds of the tier the request was sent with and
// expiresAt when it ends (epoch milliseconds), both absent where the model names
// no lifetime for that tier.
type Cache = {
  read: number;
  write: number;
  input: number;
  ttl?: number | null;
  expiresAt?: number | null;
};
type Pending = {
  frame: Frame;
  link: Link;
  resolve: (answer: Partial<Frame>) => void;
  timer: ReturnType<typeof setTimeout>;
};

// What the provider reported for an assistant message, as pi normalises it:
// input is the part neither read from nor written to the cache. Whatever the
// stop reason: a response cut short or failed after its input usage arrived
// still sent that request, and a placeholder with no input counts has none to
// show, so the caller keeps the last observation.
export function cacheOf(message: {
  role: string;
  usage?: { input?: number; cacheRead?: number; cacheWrite?: number };
}): Cache | null {
  if (message.role !== "assistant" || !message.usage) return null;
  const { input = 0, cacheRead = 0, cacheWrite = 0 } = message.usage;
  if (!(input >= 0 && cacheRead >= 0 && cacheWrite >= 0) || input + cacheRead + cacheWrite <= 0)
    return null;
  return { read: cacheRead, write: cacheWrite, input };
}

// lifeOf is how long the cache the message's request read or wrote lives. pi's
// own requests name no retention, so the tier is "long" when
// PI_CACHE_RETENTION=long and "short" otherwise, as pi's cache warmer reads it;
// the model's promptCache gives the tier's best-effort lifetime in seconds,
// unset where the provider's is not known. The provider counts it from the
// request, the message's timestamp, not from the end of the answer. A model
// other than the one that answered says nothing of this cache.
export function lifeOf(
  message: { provider?: string; model?: string; timestamp?: number },
  model:
    | { provider: string; id: string; promptCache?: { short?: number; long?: number } }
    | undefined,
  retention: string | undefined,
): Pick<Cache, "ttl" | "expiresAt"> {
  const ttl = model?.promptCache?.[retention === "long" ? "long" : "short"];
  const at = message.timestamp;
  if (!model || model.provider !== message.provider || model.id !== message.model) return {};
  if (!(typeof ttl === "number" && ttl > 0 && typeof at === "number" && at > 0)) return {};
  return { ttl, expiresAt: at + ttl * 1000 };
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
  let cache: State["cache"] = null;
  let compact: Pending | undefined;
  let interrupt: (Pending & { turn: Turn }) | undefined;

  function sample(): State {
    const ctx = context();
    const usage = ctx?.getContextUsage();
    return {
      sequence: ++sequence,
      activity: ctx?.isIdle() ? "idle" : "working",
      model: ctx?.model?.id ?? null,
      modelName: ctx?.model?.name || null,
      effort: ctx?.thinkingLevel ?? null,
      thinking: ctx?.model?.reasoning && ctx.thinkingLevel ? ctx.thinkingLevel !== "off" : null,
      tokens: usage?.tokens ?? null,
      window: usage?.contextWindow ?? ctx?.model?.contextWindow ?? null,
      percent: usage?.percent ?? null,
      compacting: compacting || !!compact,
      cache,
    };
  }

  function usageSeen(message: Parameters<typeof cacheOf>[0] & Parameters<typeof lifeOf>[0]) {
    const seen = cacheOf(message);
    if (!seen) return;
    cache = { ...seen, ...lifeOf(message, context()?.model, process.env.PI_CACHE_RETENTION) };
    changed();
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
          onComplete: (result) =>
            finished(pending, {
              outcome: "done",
              tokensBefore: result.tokensBefore,
              tokensAfter: result.estimatedTokensAfter,
            }),
          onError: (error) => {
            const empty = /already compacted|nothing to compact|session too small/i.test(
              error.message,
            );
            finished(pending, {
              outcome: empty ? "refused" : "failed",
              reason: empty ? "nothing to compact" : "cut short",
              detail: empty ? undefined : "native compaction failed or was interrupted",
            });
          },
        });
      } catch {
        finished(pending, {
          outcome: "failed",
          detail: "native compaction could not be requested",
        });
      }
    });
  }

  function startInterrupt(
    link: Link,
    frame: Frame,
    ctx: ExtensionContext,
    running: Turn,
  ): Promise<Partial<Frame>> {
    return new Promise((resolve) => {
      const pending = {
        frame,
        link,
        resolve,
        turn: running,
        timer: setTimeout(
          () =>
            resolve({
              outcome: "failed",
              open: true,
              reason: "cut short",
              detail: "the turn has not settled after interrupt",
            }),
          3000,
        ),
      };
      interrupt = pending;
      running.interrupted = `${frame.from} interrupted this turn with rewake interrupt`;
      try {
        ctx.abort();
      } catch {
        interrupt = undefined;
        running.interrupted = undefined;
        answer(pending, { outcome: "failed", detail: "native interrupt could not be requested" });
      }
    });
  }

  async function control(link: Link, frame: Frame): Promise<Partial<Frame>> {
    const ctx = context();
    if (!ctx || current() !== link) return { outcome: "refused", reason: "not answering" };
    if (!frame.request || !frame.from)
      return { outcome: "refused", detail: "control needs a request and sender" };
    if (compacting || compact || interrupt)
      return { outcome: "refused", reason: "another request in flight" };
    if (frame.action === "compact") {
      if (!ctx.isIdle()) return { outcome: "refused", reason: "in a turn" };
      return startCompact(link, frame, ctx);
    }
    if (frame.action === "interrupt") {
      const running = turn();
      if (!running || running.link !== link || ctx.isIdle())
        return { outcome: "refused", reason: "no turn running" };
      return startInterrupt(link, frame, ctx, running);
    }
    return { outcome: "refused", detail: "unknown control action" };
  }

  function settled(ended: Turn, aborted: boolean) {
    const pending = interrupt;
    if (pending?.turn !== ended) return;
    interrupt = undefined;
    if (aborted) {
      ended.text = ended.interrupted!;
      // Keep the actor in native history without starting another turn.
      try {
        pi.sendMessage(
          { customType: "rewake_interrupt", content: ended.text, display: true },
          { triggerTurn: false, deliverAs: "steer" },
        );
        answer(pending, { outcome: "done" });
      } catch {
        answer(pending, {
          outcome: "failed",
          detail: "the turn stopped, but its interruption trace could not be recorded",
        });
      }
    } else {
      answer(pending, { outcome: "refused", reason: "no turn running" });
    }
  }

  function reset() {
    for (const pending of [compact, interrupt]) {
      if (pending)
        answer(pending, {
          outcome: "failed",
          open: true,
          detail: "the conversation closed before its control outcome",
        });
    }
    compact = undefined;
    interrupt = undefined;
    compacting = false;
    cache = null;
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
  pi.on("message_end", async (event) => usageSeen(event.message));
  pi.on("model_select", async () => changed());
  pi.on("thinking_level_select", async () => changed());
  return { sample, changed, control, settled, reset, busy: () => compacting || !!compact };
}
