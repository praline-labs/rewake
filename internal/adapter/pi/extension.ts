import { randomUUID } from "node:crypto";
import { createConnection, type Socket } from "node:net";
import type { ExtensionAPI, ExtensionContext } from "@earendil-works/pi-coding-agent";

import { management, type State } from "./management.ts";

export type Frame = {
  op: string;
  id: number;
  protocol?: number;
  thread?: string;
  turn?: string;
  outcome?: string;
  text?: string;
  notice?: string;
  ok?: boolean;
  error?: string;
  steered?: boolean;
  state?: State;
  sequence?: number;
  pending?: boolean;
  action?: string;
  request?: string;
  from?: string;
  focus?: string;
  reason?: string;
  detail?: string;
  open?: boolean;
  tokensBefore?: number;
  tokensAfter?: number;
};
type Waiter = { resolve: (frame: Frame) => void; reject: (error: Error) => void; timer: ReturnType<typeof setTimeout> };
export type Link = { socket: Socket; thread: string; ready: boolean; next: number; waiting: Map<number, Waiter> };
export type Turn = { id: string; link: Link; text: string; reason: string; interrupted?: string };

const maxFrame = 4 << 20;
const socketPath = process.env.REWAKE_PI_SOCKET;

export default function (pi: ExtensionAPI) {
  let context: ExtensionContext | undefined;
  let active = false;
  let current: Link | undefined;
  let opening: Promise<void> | undefined;
  let retry: ReturnType<typeof setTimeout> | undefined;
  let delay = 250;
  let turn: Turn | undefined;
  const managed = management(pi, () => context, () => current, () => turn,
    (link, fields) => { void ask(link, fields).catch(() => {
      if (fields.op !== "state") warn("compaction observation was not acknowledged; ask main to check its outcome");
    }); });

  function warn(message: string) {
    if (context?.hasUI) context.ui.notify(`rewake: ${message}`, "warning");
    else process.stderr.write(`rewake: ${message}\n`);
  }

  function send(link: Link, frame: Frame) {
    if (link.socket.destroyed) throw new Error("the extension connection closed");
    const line = JSON.stringify(frame);
    if (Buffer.byteLength(line) > maxFrame) throw new Error("extension frame exceeds its bound");
    link.socket.write(line + "\n");
  }

  function ask(link: Link, fields: Omit<Frame, "id">, bound = 5000): Promise<Frame> {
    return new Promise((resolve, reject) => {
      const id = ++link.next;
      const timer = setTimeout(() => {
        link.waiting.delete(id);
        reject(new Error(`no answer to ${fields.op}`));
      }, bound);
      link.waiting.set(id, { resolve, reject, timer });
      try { send(link, { ...fields, id }); }
      catch (error) {
        clearTimeout(timer);
        link.waiting.delete(id);
        reject(error);
      }
    });
  }

  function close(link: Link) {
    link.ready = false;
    link.socket.destroy();
    for (const waiter of link.waiting.values()) {
      clearTimeout(waiter.timer);
      waiter.reject(new Error("the extension connection closed"));
    }
    link.waiting.clear();
  }

  function incoming(link: Link, frame: Frame) {
    if (frame.op === "answer") {
      const waiter = link.waiting.get(frame.id);
      if (waiter) {
        clearTimeout(waiter.timer);
        link.waiting.delete(frame.id);
        // A wake may follow hello in this same data chunk, before promises resume.
        if (frame.id === 1 && frame.ok) link.ready = true;
        waiter.resolve(frame);
      }
      return;
    }
    const respond = (reply: Partial<Frame>) => {
      try { send(link, { op: "answer", id: frame.id, ...reply }); }
      catch { close(link); }
    };
    if (link !== current || !link.ready || !context || frame.thread !== context.sessionManager.getSessionId()) {
      respond({ outcome: "refused", reason: "not the selected conversation",
        error: "the delivery conversation is no longer current" });
      return;
    }
    if (frame.op === "state") { respond({ ok: true, state: managed.sample() }); return; }
    if (frame.op === "control") {
      void managed.control(link, frame).then(respond).catch(() => respond({ outcome: "failed", open: true,
        detail: "native control did not return an outcome" }));
      return;
    }
    let reply: Partial<Frame>;
    if (frame.op !== "wake") reply = { error: "unknown host operation" };
    else if (typeof frame.notice !== "string" || !frame.notice) reply = { error: "a wake needs a notice" };
    else if (managed.busy()) reply = { pending: true, error: "compaction in progress" };
    else {
      const steered = !context.isIdle();
      try {
        pi.sendMessage({ customType: "rewake_notice", content: frame.notice, display: true },
          { triggerTurn: true, deliverAs: "steer" });
        reply = { ok: true, steered };
      } catch { reply = { error: "the harness did not accept the notice" }; }
    }
    respond(reply);
  }

  function schedule() {
    if (!active || retry || !socketPath) return;
    retry = setTimeout(() => {
      retry = undefined;
      void connect();
    }, delay);
    delay = Math.min(delay * 2, 8000);
  }

  async function connect() {
    if (!active || !context || !socketPath) return;
    if (opening) return opening;
    const thread = context.sessionManager.getSessionId();
    if (current?.ready && current.thread === thread) return;
    if (current) close(current);
    const socket = createConnection(socketPath);
    const link: Link = { socket, thread, ready: false, next: 0, waiting: new Map() };
    current = link;
    let buffer = Buffer.alloc(0);
    socket.on("data", (part: Buffer) => {
      buffer = Buffer.concat([buffer, part]);
      for (;;) {
        const end = buffer.indexOf(10);
        if (end < 0) {
          if (buffer.length > maxFrame) close(link);
          return;
        }
        if (end > maxFrame) { close(link); return; }
        const line = buffer.subarray(0, end).toString("utf8");
        buffer = buffer.subarray(end + 1);
        try {
          const frame = JSON.parse(line) as Frame;
          if (!frame || typeof frame.op !== "string" || !Number.isSafeInteger(frame.id) || frame.id <= 0) {
            throw new Error("invalid frame");
          }
          incoming(link, frame);
        } catch { close(link); return; }
      }
    });
    socket.on("error", () => close(link));
    socket.on("close", () => {
      close(link);
      if (current === link) {
        current = undefined;
        schedule();
      }
    });
    opening = (async () => {
      try {
        const reply = await ask(link, { op: "hello", protocol: 2, thread, state: managed.sample() });
        if (!reply.ok) throw new Error("host refused the extension handshake");
        if (current !== link || !active) { close(link); return; }
        link.ready = true;
        delay = 250;
      } catch {
        close(link);
        warn("mail connection unavailable; reconnecting while this session runs");
      } finally { opening = undefined; }
    })();
    return opening;
  }

  pi.on("session_start", async (_event, ctx) => {
    context = ctx;
    active = true;
    await connect();
  });

  pi.on("session_shutdown", async () => {
    active = false;
    managed.reset();
    if (retry) clearTimeout(retry);
    retry = undefined;
    if (current) close(current);
    current = undefined;
    // A replacement conversation cannot finish the old conversation's work.
    turn = undefined;
    await opening;
  });

  pi.on("agent_start", async (_event, ctx) => {
    context = ctx;
    // Automatic retries have another agent_start, but only one settled end.
    if (turn) return;
    const link = current;
    if (!link?.ready) { warn("turn started without a mail connection; its end cannot be reported"); return; }
    const started: Turn = { id: randomUUID(), link, text: "", reason: "" };
    turn = started;
    managed.changed();
    try {
      const reply = await ask(link, { op: "turn-start", thread: link.thread, turn: started.id });
      if (!reply.ok) throw new Error("host refused the turn start");
    } catch {
      if (turn === started) turn = undefined;
      warn("turn start was not acknowledged; its end cannot be reported");
    }
  });

  pi.on("agent_end", async (event) => {
    if (!turn) return;
    const assistant = [...event.messages].reverse().find((message) => message.role === "assistant");
    turn.text = "";
    turn.reason = "";
    if (assistant?.role === "assistant") {
      turn.text = assistant.content.filter((block) => block.type === "text").map((block) => block.text).join("\n");
      turn.reason = assistant.stopReason;
    }
  });

  pi.on("agent_settled", async (event) => {
    const ended = turn;
    turn = undefined;
    managed.changed();
    if (!ended) return;
    const outcome = event.aborted || ended.reason === "aborted" ? "aborted"
      : ended.reason === "stop" ? "answered" : "failed";
    managed.settled(ended, outcome === "aborted");
    try {
      const reply = await ask(ended.link, { op: "turn-end", thread: ended.link.thread,
        turn: ended.id, outcome, text: ended.text }, 20000);
      if (!reply.ok) throw new Error("host refused the turn end");
    } catch { warn("turn end was not acknowledged; ask main to check the report"); }
  });
}
