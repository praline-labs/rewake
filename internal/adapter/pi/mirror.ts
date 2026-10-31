// The conversation of a pi session rewake's own face draws (docs/face.md),
// passed to its wrapper in memory only, once the hello's answer said mirror:
// the prompts, the reply and thinking as they stream, each tool call with its
// input as it streams, its output and its end, and the turn. An observer: each
// handler only pushes to the outbox and returns, never awaits, never blocks a
// call or changes an event, and a failure of its own is dropped.
import { randomUUID } from "node:crypto";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { bounds, fit, type MirrorEvent, pieces, textBudget } from "./outbox.ts";
import { Queue, textOf } from "./queued.ts";
import { facedTool, resultText } from "./toolmap.ts";

type Sink = { push: (event: MirrorEvent) => void };

// An assistant message's end as pi gives it; only what is read here.
type Ended = {
  role?: string;
  stopReason?: string;
  errorMessage?: string;
  content?: { type?: string; text?: string; thinking?: string }[];
};

// A streamed piece of a reply as pi-ai gives it; only what is read here.
type Streamed = {
  type: string;
  contentIndex?: number;
  delta?: string;
  content?: string;
  toolCall?: { id?: string; name?: string; arguments?: unknown };
  partial?: { content?: { type?: string; id?: string; name?: string; arguments?: unknown }[] };
};

// How often a call's input is sent while it streams: each send stringifies
// the input so far, and a long write streams thousands of deltas.
const inputEvery = 100;

// The face's prompts sent and not yet heard back as pi's input.
const facedKept = 16;

const origins: Record<string, string> = {
  interactive: "person",
  extension: "extension",
  rpc: "rpc",
};

export class Mirror {
  // on is the hello's answer: rewake's face draws this run.
  on = false;
  // run names the agent run going on, the face's turn; pass counts pi's
  // loops in it, a retry or a continuation starting its turns again from 0,
  // and step is pi's turn in that loop.
  private run?: string;
  private pass = 0;
  private step = 0;
  // reached are the calls this extension's tool_call hook saw: pi's own
  // checks passed, so one that ends without running was refused by a hook.
  private reached = new Set<string>();
  private faced: string[] = [];
  private sentAt = new Map<string, number>();
  // thought are the thinking blocks of the run that streamed text: only they
  // reach the face before their end.
  private thought = new Set<string>();
  // open are the blocks streaming, by their index in the message, and
  // waiting the calls whose input streamed and that have not run: an
  // aborted or failed message ends them as pi's own screen does.
  private open = new Map<string, number>();
  private waiting = new Set<string>();
  // failed is the error the latest message ended with; retries, the runs pi
  // started again since a message did not fail, which it tells no extension.
  private failed?: string;
  private retries = 0;
  // queue follows the prompts pi holds until it takes them.
  readonly queue = new Queue((event) => this.push(event));

  constructor(
    private sink: Sink,
    private now = () => Date.now(),
  ) {}

  private push(event: MirrorEvent) {
    if (this.on) this.sink.push(event);
  }

  // sent is a prompt of the face handed to pi, heard back as its input.
  sent(text: string) {
    this.faced.push(text);
    if (this.faced.length > facedKept) this.faced.shift();
  }

  input(text: string, source: string, steering: boolean) {
    let origin = origins[source] ?? source;
    if (source === "extension" && this.faced[0] === text) {
      this.faced.shift();
      origin = "face";
    }
    // A prompt steered into the run going on is that run's, and waits in
    // pi's queue until pi takes it.
    const turn = steering ? this.run : undefined;
    const id = randomUUID();
    const status = steering ? "queued" : undefined;
    this.push({ kind: "prompt", id, turn, origin, status, text: fit(text, textBudget) });
    if (steering) this.queue.add(id, text);
  }

  // began is a message pi began: a user message while prompts wait is one
  // pi took from its queue; every message holds the rest against it.
  began(message: { role?: string; content?: unknown }, pending: boolean) {
    if (message.role === "user") this.queue.took(textOf(message.content));
    this.queue.check(pending);
  }

  started() {
    this.step = 0;
    if (this.run) {
      if (this.failed !== undefined) this.retries++;
      this.failed = undefined;
      this.pass++;
      return;
    }
    this.run = randomUUID();
    this.pass = 0;
    this.push({ kind: "turn", turn: this.run, state: "working" });
  }

  stepped(index: number) {
    this.step = index;
  }

  // settled ends the run; an aborted one (the person's Esc) says so in the
  // turn's end, a fact the face draws, not a notice it would have to guess.
  settled(aborted = false) {
    const run = this.run;
    this.queue.settled();
    // Retries that all failed end as pi's screen ends them.
    if (run && !aborted && this.retries > 0 && this.failed !== undefined)
      this.notice(`Error: Retry failed after ${this.retries} attempts: ${this.failed}`);
    this.failed = undefined;
    this.retries = 0;
    this.run = undefined;
    this.reached.clear();
    this.sentAt.clear();
    this.thought.clear();
    this.open.clear();
    this.waiting.clear();
    if (run)
      this.push({
        kind: "turn",
        turn: run,
        state: "idle",
        ...(aborted ? { status: "error" } : {}),
      });
  }

  private blockId(index: number | undefined) {
    return `${this.run ?? "run"}.${this.pass}.${this.step}.${index ?? 0}`;
  }

  private settle(id: string, block: "text" | "thinking", text: string) {
    const parts = pieces(fit(text, bounds.block), textBudget);
    for (const [i, part] of parts.entries())
      this.push({
        kind: "settle",
        id,
        turn: this.run,
        block,
        part: i,
        of: parts.length,
        text: part,
      });
  }

  private toolInput(id: string, name: string, args: unknown) {
    const faced = facedTool(name, args);
    this.push({ kind: "input", id, text: fit(JSON.stringify(faced.input), textBudget) });
  }

  streamed(e: Streamed) {
    const turn = this.run;
    switch (e.type) {
      case "text_delta":
      case "thinking_delta": {
        const block = e.type === "text_delta" ? "text" : "thinking";
        const id = this.blockId(e.contentIndex);
        for (const text of pieces(e.delta ?? "", textBudget))
          if (text !== "") {
            if (block === "thinking") this.thought.add(id);
            this.open.set(id, e.contentIndex ?? 0);
            this.push({ kind: "delta", id, turn, block, text });
          }
        return;
      }
      case "text_end": {
        const id = this.blockId(e.contentIndex);
        this.open.delete(id);
        this.settle(id, "text", e.content ?? "");
        return;
      }
      case "thinking_end": {
        // A block with nothing in it, a signature only as most providers
        // send, shows nothing, as the Claude Code worker's face shows none:
        // a thinking row is drawn only for something to show.
        const id = this.blockId(e.contentIndex);
        this.open.delete(id);
        const content = e.content ?? "";
        const streamed = this.thought.delete(id);
        if (streamed || content.trim() !== "") this.settle(id, "thinking", content);
        return;
      }
      case "toolcall_start":
      case "toolcall_delta": {
        const call = e.partial?.content?.[e.contentIndex ?? -1];
        if (call?.type !== "toolCall" || !call.id || !call.name) return;
        if (e.type === "toolcall_start") {
          this.waiting.add(call.id);
          this.push({ kind: "tool", id: call.id, turn, name: facedTool(call.name, {}).name });
          return;
        }
        const at = this.sentAt.get(call.id) ?? 0;
        if (this.now() - at < inputEvery) return;
        this.sentAt.set(call.id, this.now());
        this.toolInput(call.id, call.name, call.arguments);
        return;
      }
      case "toolcall_end":
        if (e.toolCall?.id && e.toolCall.name)
          this.toolInput(e.toolCall.id, e.toolCall.name, e.toolCall.arguments);
        return;
    }
  }

  // ended takes an assistant message's end. One pi stopped or that failed ends
  // as pi's screen ends it: what streamed stays, each call it asked for ends
  // with the reason, and a message with no call says it in red.
  ended(message: Ended) {
    if (message.role !== "assistant") return;
    const stop = message.stopReason;
    const retries = this.retries;
    this.failed = stop === "error" ? message.errorMessage || "Unknown error" : undefined;
    if (stop !== "error") this.retries = 0;
    if (stop !== "aborted" && stop !== "error") return;
    const content = message.content ?? [];
    for (const [id, index] of this.open) {
      const block = content[index];
      if (block?.type === "text") this.settle(id, "text", block.text ?? "");
      else if (block?.type === "thinking") this.settle(id, "thinking", block.thinking ?? "");
    }
    this.open.clear();
    this.thought.clear();
    const reason =
      stop === "error"
        ? message.errorMessage || "Error"
        : retries > 0
          ? `Aborted after ${retries} retry attempt${retries > 1 ? "s" : ""}`
          : "Operation aborted";
    for (const id of this.waiting)
      this.push({ kind: "result", id, status: "error", text: fit(reason, textBudget) });
    this.waiting.clear();
    if (content.some((block) => block.type === "toolCall")) return;
    this.notice(stop === "aborted" ? reason : `Error: ${message.errorMessage || "Unknown error"}`);
  }

  // notice says a failure in red under what it stopped.
  private notice(text: string) {
    const id = randomUUID();
    this.push({ kind: "notice", id, turn: this.run, status: "error", text: fit(text, textBudget) });
  }

  toolStarted(id: string, name: string, args: unknown) {
    this.waiting.delete(id);
    this.push({ kind: "tool", id, turn: this.run, name: facedTool(name, {}).name });
    this.toolInput(id, name, args);
  }

  toolCalled(id: string) {
    this.reached.add(id);
  }

  toolEnded(id: string, result: unknown, isError: boolean, ran: boolean, aborted: boolean) {
    const text = fit(resultText(result), textBudget);
    if (!isError) return this.push({ kind: "result", id, status: "done", text });
    // A call pi's checks let through that never ran was blocked by a hook.
    if (!ran && !aborted && this.reached.has(id))
      return this.push({
        kind: "result",
        id,
        status: "refused",
        text,
        permit: { by: "hook", allowed: false, reason: fit(text, 256) },
      });
    this.push({ kind: "result", id, status: "error", text });
  }
}

type Observed = {
  signal?: AbortSignal;
  isIdle: () => boolean;
  hasPendingMessages?: () => boolean;
};

// observing runs a handler as an observer: whatever it throws is dropped, and
// it returns nothing, so pi's event goes on as it came.
const observing =
  <E>(handle: (event: E, ctx: Observed) => void) =>
  (event: E, ctx: Observed) => {
    try {
      handle(event, ctx);
    } catch {}
    return undefined;
  };

// serveMirror hooks pi's events for the face; nested calls a tool made are
// left out, as the Claude Code mod leaves a subagent's.
export function serveMirror(pi: ExtensionAPI, mirror: Mirror) {
  pi.on(
    "input",
    observing((e: { text: string; source: string; streamingBehavior?: string }) =>
      mirror.input(e.text, e.source, e.streamingBehavior !== undefined),
    ),
  );
  pi.on(
    "agent_start",
    observing(() => mirror.started()),
  );
  pi.on(
    "turn_start",
    observing((e: { turnIndex: number }, ctx) => {
      mirror.stepped(e.turnIndex);
      mirror.queue.check(ctx.hasPendingMessages?.() ?? true);
    }),
  );
  pi.on(
    "message_start",
    observing((e: { message: { role?: string; content?: unknown } }, ctx) =>
      mirror.began(e.message, ctx.hasPendingMessages?.() ?? true),
    ),
  );
  pi.on(
    "message_update",
    observing((e: { assistantMessageEvent: Streamed }) => mirror.streamed(e.assistantMessageEvent)),
  );
  pi.on(
    "message_end",
    observing((e: { message: Parameters<Mirror["ended"]>[0] }) => mirror.ended(e.message)),
  );
  pi.on(
    "tool_execution_start",
    observing(
      (e: { toolCallId: string; toolName: string; args: unknown; parentToolCallId?: string }) => {
        if (!e.parentToolCallId) mirror.toolStarted(e.toolCallId, e.toolName, e.args);
      },
    ),
  );
  pi.on(
    "tool_call",
    observing((e: { toolCallId: string; parentToolCallId?: string }) => {
      if (!e.parentToolCallId) mirror.toolCalled(e.toolCallId);
    }),
  );
  pi.on(
    "tool_execution_end",
    observing(
      (
        e: {
          toolCallId: string;
          result: unknown;
          isError: boolean;
          durationMs?: number;
          parentToolCallId?: string;
        },
        ctx,
      ) => {
        if (!e.parentToolCallId)
          mirror.toolEnded(
            e.toolCallId,
            e.result,
            e.isError,
            e.durationMs !== undefined,
            !!ctx.signal?.aborted,
          );
      },
    ),
  );
  pi.on(
    "agent_settled",
    observing((event: { aborted?: boolean }) => mirror.settled(event.aborted === true)),
  );
}
