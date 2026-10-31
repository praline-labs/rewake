// The conversation of a pi session rewake's own face draws (docs/face.md),
// passed to its wrapper in memory only, once the hello's answer said mirror:
// the prompts, the reply and thinking as they stream, each tool call with its
// input as it streams, its output and its end, and the turn. An observer: each
// handler only pushes to the outbox and returns, never awaits, never blocks a
// call or changes an event, and a failure of its own is dropped.
import { randomUUID } from "node:crypto";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { bounds, fit, type MirrorEvent, pieces, textBudget } from "./outbox.ts";
import { facedTool, resultText } from "./toolmap.ts";

type Sink = { push: (event: MirrorEvent) => void };

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
  // run names the agent run going on, the face's turn; step is pi's turn in it.
  private run?: string;
  private step = 0;
  // reached are the calls this extension's tool_call hook saw: pi's own
  // checks passed, so one that ends without running was refused by a hook.
  private reached = new Set<string>();
  private faced: string[] = [];
  private sentAt = new Map<string, number>();

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
    // A prompt steered into the run going on is that run's.
    const turn = steering ? this.run : undefined;
    this.push({ kind: "prompt", id: randomUUID(), turn, origin, text: fit(text, textBudget) });
  }

  started() {
    if (this.run) return;
    this.run = randomUUID();
    this.step = 0;
    this.push({ kind: "turn", turn: this.run, state: "working" });
  }

  stepped(index: number) {
    this.step = index;
  }

  settled() {
    const run = this.run;
    this.run = undefined;
    this.reached.clear();
    this.sentAt.clear();
    if (run) this.push({ kind: "turn", turn: run, state: "idle" });
  }

  private blockId(index: number | undefined) {
    return `${this.run ?? "run"}.${this.step}.${index ?? 0}`;
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
    this.push({ kind: "input", id, text: fit(JSON.stringify(faced.input), bounds.field) });
  }

  streamed(e: Streamed) {
    const turn = this.run;
    switch (e.type) {
      case "thinking_start":
        // Most providers stream no thinking text: the block shows all the same.
        this.push({
          kind: "delta",
          id: this.blockId(e.contentIndex),
          turn,
          block: "thinking",
          text: "",
        });
        return;
      case "text_delta":
      case "thinking_delta": {
        const block = e.type === "text_delta" ? "text" : "thinking";
        for (const text of pieces(e.delta ?? "", textBudget))
          if (text !== "")
            this.push({ kind: "delta", id: this.blockId(e.contentIndex), turn, block, text });
        return;
      }
      case "text_end":
      case "thinking_end":
        this.settle(
          this.blockId(e.contentIndex),
          e.type === "text_end" ? "text" : "thinking",
          e.content ?? "",
        );
        return;
      case "toolcall_start":
      case "toolcall_delta": {
        const call = e.partial?.content?.[e.contentIndex ?? -1];
        if (call?.type !== "toolCall" || !call.id || !call.name) return;
        if (e.type === "toolcall_start") {
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

  // ended takes an assistant message's end: a failure pi shows in its own
  // words is a notice of the face.
  ended(message: { role?: string; stopReason?: string; errorMessage?: string }) {
    if (message.role !== "assistant" || message.stopReason !== "error" || !message.errorMessage)
      return;
    this.push({
      kind: "notice",
      id: randomUUID(),
      turn: this.run,
      text: fit(message.errorMessage, textBudget),
    });
  }

  toolStarted(id: string, name: string, args: unknown) {
    this.push({ kind: "tool", id, turn: this.run, name: facedTool(name, {}).name });
    this.toolInput(id, name, args);
  }

  toolCalled(id: string) {
    this.reached.add(id);
  }

  toolEnded(id: string, result: unknown, isError: boolean, ran: boolean, aborted: boolean) {
    const text = fit(resultText(result), bounds.field);
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

// observing runs a handler as an observer: whatever it throws is dropped, and
// it returns nothing, so pi's event goes on as it came.
const observing =
  <E>(handle: (event: E, ctx: { signal?: AbortSignal; isIdle: () => boolean }) => void) =>
  (event: E, ctx: { signal?: AbortSignal; isIdle: () => boolean }) => {
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
    observing((e: { turnIndex: number }) => mirror.stepped(e.turnIndex)),
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
    observing(() => mirror.settled()),
  );
}

// facePrompt hands a prompt typed in the face to pi as the person's: a turn
// while pi is idle, steered into the run going on while it works, as pi's
// own editor queues one. pi takes it in or shows its own error.
export function facePrompt(
  pi: ExtensionAPI,
  mirror: Mirror,
  idle: boolean,
  compacting: boolean,
  text: unknown,
): { ok?: boolean; steered?: boolean; error?: string } {
  if (!mirror.on) return { error: "rewake's face does not draw this session" };
  if (typeof text !== "string" || text.trim() === "") return { error: "a prompt needs a text" };
  if (compacting) return { error: "pi is compacting its context; send it again once that is done" };
  try {
    mirror.sent(text);
    pi.sendUserMessage(text, idle ? undefined : { deliverAs: "steer" });
    return { ok: true, steered: !idle };
  } catch {
    return { error: "pi did not take the prompt" };
  }
}
