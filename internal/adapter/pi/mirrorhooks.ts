// pi's events the face's mirror hears (docs/face.md), each handled as an
// observer: whatever a handler throws is dropped, and it returns nothing, so
// pi's event goes on as it came and no compaction is cancelled.
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { Mirror } from "./mirror.ts";

type Observed = {
  signal?: AbortSignal;
  isIdle: () => boolean;
  hasPendingMessages?: () => boolean;
};

// observing runs a handler as an observer.
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
    observing((e: { assistantMessageEvent: Parameters<Mirror["streamed"]>[0] }) =>
      mirror.streamed(e.assistantMessageEvent),
    ),
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
    "session_before_compact",
    observing(() => mirror.compacting()),
  );
  pi.on(
    "session_compact",
    observing(() => mirror.compacted()),
  );
  pi.on(
    "session_compact_failed",
    observing(() => mirror.compacted()),
  );
  pi.on(
    "agent_settled",
    observing((event: { aborted?: boolean }) => mirror.settled(event.aborted === true)),
  );
}
