// What the person does in rewake's face, handed to pi (docs/face.md, "The
// face"): a prompt typed there, Esc and Alt+T. on is the hello's answer:
// rewake's face draws this run. The face hears pi's thinking level from the
// session's state, which the wrapper reads (native.go).
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";

// facePrompt hands a prompt typed in the face to pi as the person's: a turn
// while pi is idle, steered into the run going on while it works, as pi's
// own editor queues one. pi takes it in or shows its own error; the wrapper
// knows it as the face's when pi hears it as its input.
export function facePrompt(
  pi: ExtensionAPI,
  on: boolean,
  idle: boolean,
  compacting: boolean,
  text: unknown,
): { ok?: boolean; steered?: boolean; error?: string } {
  if (!on) return { error: "rewake's face does not draw this session" };
  if (typeof text !== "string" || text.trim() === "") return { error: "a prompt needs a text" };
  if (compacting) return { error: "pi is compacting its context; send it again once that is done" };
  try {
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
  on: boolean,
  ctx: { isIdle: () => boolean; abort: () => void },
  compacting: boolean,
): { ok?: boolean; error?: string } {
  if (!on) return { error: "rewake's face does not draw this session" };
  if (compacting) return { error: "pi is compacting its context" };
  if (ctx.isIdle()) return { error: "pi is not working" };
  try {
    ctx.abort();
    return { ok: true };
  } catch {
    return { error: "pi did not take the interrupt" };
  }
}

// faceThinking moves pi's thinking level on at the person's Alt+T in the
// face, as pi's own app.thinking.cycle does: the next of the levels the
// wrapper read off the model (native.go, levelsOf) after the one pi runs at,
// for this session alone. pi's screen and rewake's telemetry hear it as any
// change of the level.
export function faceThinking(
  pi: Pick<ExtensionAPI, "getThinkingLevel" | "setThinkingLevel">,
  on: boolean,
  levels: string[] | undefined,
): { ok?: boolean; error?: string } {
  if (!on) return { error: "rewake's face does not draw this session" };
  if (!levels?.length) return { error: "a thinking cycle needs its levels" };
  try {
    const next = levels[(levels.indexOf(pi.getThinkingLevel()) + 1) % levels.length];
    if (next) pi.setThinkingLevel(next as never);
    return { ok: true };
  } catch {
    return { error: "pi did not change the thinking level" };
  }
}
