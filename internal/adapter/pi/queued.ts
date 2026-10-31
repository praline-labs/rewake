// The prompts pi holds while it works (docs/face.md, "The look"): one
// steered into a working run waits in pi's queue until pi takes it after the
// run's current tool batch, its user message beginning then, or until pi
// gives it back — Esc puts the queue back into pi's editor, alt+up takes it
// there without stopping the run — or an input handler took it whole. The
// face shows it waiting, as pi's own screen does under its chat, until the
// take settles it where pi took it.

import type { MirrorEvent } from "./outbox.ts";

// A run queues few prompts; past this many the oldest is no longer followed
// and, left queued, ends a gap with the run.
const followed = 32;

type Waiting = { id: string; text: string; missed: boolean };

// textOf is a user message's text as pi matches it against its queue: its
// text parts joined with nothing between.
export function textOf(content: unknown): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  return content
    .filter((part) => part?.type === "text" && typeof part.text === "string")
    .map((part) => part.text as string)
    .join("");
}

export class Queue {
  private waiting: Waiting[] = [];

  constructor(private push: (event: MirrorEvent) => void) {}

  get size() {
    return this.waiting.length;
  }

  // add follows a prompt pi was given to queue.
  add(id: string, text: string) {
    this.waiting.push({ id, text, missed: false });
    if (this.waiting.length > followed) this.waiting.shift();
  }

  // took is a user message pi began while prompts wait: pi took the one of
  // its text, the first of them where two are the same; when none is, an
  // input handler or a template changed the text on its way to the queue,
  // and pi takes its queue in order, so it is the oldest.
  took(text: string) {
    if (this.waiting.length === 0) return;
    const at = Math.max(
      this.waiting.findIndex((w) => w.text === text),
      0,
    );
    const [taken] = this.waiting.splice(at, 1);
    if (taken) this.push({ kind: "taken", id: taken.id });
  }

  // check holds what waits against whether pi's queue holds anything: one
  // found missing twice running was given back or taken whole by an input
  // handler. Twice, since a prompt is followed from pi's input event, a
  // moment before pi puts it in its queue.
  check(pending: boolean) {
    if (pending) {
      for (const w of this.waiting) w.missed = false;
      return;
    }
    const back = this.waiting.filter((w) => w.missed);
    this.waiting = this.waiting.filter((w) => !w.missed);
    for (const w of this.waiting) w.missed = true;
    for (const w of back) this.push({ kind: "taken", id: w.id, status: "refused" });
  }

  // settled gives back what is left as the run ends, before its end: pi
  // takes its whole queue within a run, and Esc puts it back in its editor.
  settled() {
    for (const w of this.waiting) this.push({ kind: "taken", id: w.id, status: "refused" });
    this.waiting = [];
  }
}
