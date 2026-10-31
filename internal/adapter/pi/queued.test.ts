import { expect, test } from "bun:test";
import { Mirror, serveMirror } from "./mirror.ts";
import type { MirrorEvent } from "./outbox.ts";

// running is a pi at work whose queue holds what pending says, as pi's
// events reach an extension.
function running() {
  const events: MirrorEvent[] = [];
  const mirror = new Mirror({ push: (e) => events.push(e) });
  mirror.on = true;
  const handlers = new Map<string, ((e: unknown, ctx: unknown) => unknown)[]>();
  const pi = {
    on: (name: string, handle: (e: unknown, ctx: unknown) => unknown) =>
      handlers.set(name, [...(handlers.get(name) ?? []), handle]),
  };
  serveMirror(pi as never, mirror);
  let pending = false;
  const emit = (name: string, e: unknown) =>
    (handlers.get(name) ?? []).map((handle) =>
      handle(e, { isIdle: () => false, hasPendingMessages: () => pending }),
    );
  emit("agent_start", {});
  const steer = (text: string) => {
    emit("input", { text, source: "extension", streamingBehavior: "steer" });
    pending = true;
  };
  const user = (text: string) =>
    emit("message_start", { message: { role: "user", content: [{ type: "text", text }] } });
  const queue = (holds: boolean) => {
    pending = holds;
  };
  const taken = () =>
    events.filter((e) => e.kind === "taken").map((e) => [idOf(e.id), e.status ?? "taken"]);
  const idOf = (id?: string) => events.find((e) => e.kind === "prompt" && e.id === id)?.text;
  return { events, emit, steer, user, queue, taken };
}

// pi takes a steered prompt after the tool batch, its user message beginning
// then: the face hears it taken, the same texts one at a time.
test("a steered prompt waits queued until pi takes it", () => {
  const pi = running();
  pi.steer("a");
  pi.steer("a");
  const prompts = pi.events.filter((e) => e.kind === "prompt");
  expect(prompts.map((e) => e.status)).toEqual(["queued", "queued"]);
  pi.user("a");
  expect(pi.taken()).toEqual([["a", "taken"]]);
  pi.user("a");
  expect(pi.taken()).toEqual([
    ["a", "taken"],
    ["a", "taken"],
  ]);
});

// Review of 8e94ba1: an input handler consumed one prompt and transformed the
// next, and the transformed message was said to be the consumed prompt, the
// oldest. A message of no queued text is none of them: both are given back
// once pi's queue is found empty, and neither is said taken.
test("a message of no queued text is said to be no queued prompt", () => {
  const pi = running();
  pi.steer("handled");
  pi.steer("/tpl x");
  pi.user("the template, expanded");
  expect(pi.taken()).toEqual([]);
  pi.queue(false);
  pi.emit("message_start", { message: { role: "assistant" } });
  pi.emit("message_start", { message: { role: "assistant" } });
  expect(pi.taken()).toEqual([
    ["handled", "refused"],
    ["/tpl x", "refused"],
  ]);
});

// Alt+up in pi's own screen takes the queue back into its editor while the
// run goes on: the next two of pi's events find the queue empty, and the
// prompt is given back. Esc's end of the run gives back what is left before
// the turn's end, so the model does not leave it a gap.
test("a prompt pi gave back, by alt+up or Esc, is refused", () => {
  const pi = running();
  pi.steer("back");
  pi.queue(false);
  pi.emit("turn_start", { turnIndex: 1 });
  expect(pi.taken()).toEqual([]);
  pi.emit("message_start", { message: { role: "assistant" } });
  expect(pi.taken()).toEqual([["back", "refused"]]);
  pi.steer("left");
  pi.emit("agent_settled", { aborted: true });
  expect(pi.taken()).toEqual([
    ["back", "refused"],
    ["left", "refused"],
  ]);
  const last = pi.events.slice(-2).map((e) => e.kind);
  expect(last).toEqual(["taken", "turn"]);
});
