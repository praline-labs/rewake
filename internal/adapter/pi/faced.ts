// What the person does in rewake's face, handed to pi (docs/face.md, "The
// face"): a prompt typed there, and Esc.
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import type { Mirror } from "./mirror.ts";

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

// faceInterrupt stops pi's run on the person's Esc in the face. In pi's own
// screen ctx.abort() is its Esc while it works: the run stops and a message
// queued to steer it goes back to pi's editor. Nothing else ever calls it from
// the face, and an idle pi is left alone.
export function faceInterrupt(
  mirror: Mirror,
  ctx: { isIdle: () => boolean; abort: () => void },
  compacting: boolean,
): { ok?: boolean; error?: string } {
  if (!mirror.on) return { error: "rewake's face does not draw this session" };
  if (compacting) return { error: "pi is compacting its context" };
  if (ctx.isIdle()) return { error: "pi is not working" };
  try {
    ctx.abort();
    return { ok: true };
  } catch {
    return { error: "pi did not take the interrupt" };
  }
}
