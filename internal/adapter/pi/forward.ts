// pi's events the face's mirror is told of (docs/face.md; translate.go
// translates them), each forwarded as pi gave it, less what is only pi's own:
// a streamed piece without the partial message it grows, a message's end
// without the blocks a finished one streamed already. Each handler is an
// observer: it pushes to the stream and returns nothing, never awaits, and
// whatever it throws is dropped, so pi's event goes on as it came and no
// compaction is cancelled.
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { bounds, type Forwarded, type Stream } from "./stream.ts";

type Observed = {
  signal?: AbortSignal;
  hasPendingMessages?: () => boolean;
};

type Content = { type?: string; text?: unknown; thinking?: unknown }[] | string | undefined;

// texts are a user message's text parts, as pi matches its queue against them.
function texts(content: Content) {
  if (!Array.isArray(content)) return content;
  return content.filter((part) => part?.type === "text").map(({ type, text }) => ({ type, text }));
}

// blocks are an assistant message's blocks by index: the text of each text
// and thinking block, the kind of every other.
function blocks(content: Content) {
  if (!Array.isArray(content)) return [];
  return content.map(({ type, text, thinking }) => ({ type, text, thinking }));
}

// A streamed piece of a reply as pi-ai gives it; only what is read here.
type Streamed = {
  type: string;
  contentIndex?: number;
  delta?: string;
  content?: string;
  toolCall?: { type?: string; id?: string; name?: string; arguments?: unknown };
  partial?: { content?: { type?: string; id?: string; name?: string; arguments?: unknown }[] };
};

// parts are a tool result's content parts: their kind, and a text part's text.
function parts(result: unknown) {
  const content = (result as { content?: unknown })?.content;
  if (!Array.isArray(content)) return content;
  return content.map((part: { type?: string; text?: unknown }) => ({
    type: part?.type,
    text: part?.text,
  }));
}

const call = (block?: { type?: string; id?: string; name?: string; arguments?: unknown }) =>
  block && { type: block.type, id: block.id, name: block.name, arguments: block.arguments };

// A tool's input and output each go within one line, the face's bound for
// them being smaller still.
const toolBound = bounds.part;

export function forward(pi: ExtensionAPI, stream: Stream) {
  const on = <E>(
    name: string,
    read: (event: E, ctx: Observed) => Forwarded | undefined,
    how: (forwarded: Forwarded) => { delta?: boolean; max?: number } = () => ({}),
  ) =>
    pi.on(
      name as never,
      ((event: E, ctx: Observed) => {
        try {
          const forwarded = read(event, ctx);
          if (!forwarded) return undefined;
          const { delta, max } = how(forwarded);
          stream.push(forwarded, delta, max);
        } catch {}
        return undefined;
      }) as never,
    );
  const pending = (ctx: Observed) => ctx.hasPendingMessages?.();

  on("input", (e: { text: string; source: string; streamingBehavior?: string }) => ({
    event: "input",
    text: e.text,
    source: e.source,
    streamingBehavior: e.streamingBehavior,
  }));
  on("agent_start", () => ({ event: "agent_start" }));
  on("turn_start", (e: { turnIndex: number }, ctx) => ({
    event: "turn_start",
    turnIndex: e.turnIndex,
    pending: pending(ctx),
  }));
  on("message_start", (e: { message: { role?: string; content?: Content } }, ctx) => ({
    event: "message_start",
    role: e.message.role,
    content: e.message.role === "user" ? texts(e.message.content) : undefined,
    pending: pending(ctx),
  }));
  on(
    "message_update",
    (e: { assistantMessageEvent: Streamed }) => {
      const u = e.assistantMessageEvent;
      return {
        event: "message_update",
        type: u.type,
        contentIndex: u.contentIndex,
        delta: u.delta,
        content: u.content,
        toolCall: call(u.toolCall),
        // A call's start: its native block as it began.
        block:
          u.type === "toolcall_start"
            ? call(u.partial?.content?.[u.contentIndex ?? -1])
            : undefined,
      };
    },
    (forwarded) => ({
      delta: String(forwarded.type).endsWith("_delta"),
      max: forwarded.type === "toolcall_end" ? toolBound : undefined,
    }),
  );
  on(
    "message_end",
    (e: {
      message: { role?: string; stopReason?: string; errorMessage?: string; content?: Content };
    }) => {
      const { role, stopReason, errorMessage, content } = e.message;
      // pi sends no update for an aborted or failed message's end: its
      // blocks are the only word of what streamed last.
      const last = stopReason === "aborted" || stopReason === "error";
      return {
        event: "message_end",
        role,
        stopReason,
        errorMessage,
        content: last ? blocks(content) : undefined,
      };
    },
  );
  on(
    "tool_execution_start",
    (e: { toolCallId: string; toolName: string; args: unknown; parentToolCallId?: string }) => ({
      event: "tool_execution_start",
      toolCallId: e.toolCallId,
      toolName: e.toolName,
      args: e.args,
      parentToolCallId: e.parentToolCallId,
    }),
    () => ({ max: toolBound }),
  );
  on("tool_call", (e: { toolCallId: string; parentToolCallId?: string }) => ({
    event: "tool_call",
    toolCallId: e.toolCallId,
    parentToolCallId: e.parentToolCallId,
  }));
  on(
    "tool_execution_end",
    (
      e: {
        toolCallId: string;
        result: unknown;
        isError: boolean;
        durationMs?: number;
        parentToolCallId?: string;
      },
      ctx,
    ) => ({
      event: "tool_execution_end",
      toolCallId: e.toolCallId,
      // The result's content, less an image's data; its details are pi's own.
      result: typeof e.result === "string" ? e.result : { content: parts(e.result) },
      isError: e.isError,
      durationMs: e.durationMs,
      parentToolCallId: e.parentToolCallId,
      aborted: ctx.signal?.aborted === true,
    }),
    () => ({ max: toolBound }),
  );
  for (const name of ["session_before_compact", "session_compact", "session_compact_failed"])
    on(name, () => ({ event: name }));
  on("agent_settled", (e: { aborted?: boolean }) => ({
    event: "agent_settled",
    aborted: e.aborted === true,
  }));
}
