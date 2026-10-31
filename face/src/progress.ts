// What the working line tells of a turn's progress (docs/face.md, "The
// look"), as the terminal program's own spinner does: the phase its arrow shows,
// an estimate of what streamed back and the call that runs. All of it is
// derived from the model on the face's clock; the session sends none of it.

import type { Item } from "./mirror.gen.ts";
import { nameOf } from "./tools.ts";

// Phase is where the turn's request stands: out with nothing back yet, or a
// reply, a thinking block, a call's input or a call coming back.
export type Phase = "requesting" | "responding" | "thinking" | "tool-input" | "tool-use";

// Progress is the turn's phase, what streamed back, and the name of the call
// that runs, as the face draws its head.
export type Progress = { phase: Phase; tokens: number; tool?: string };

// own is an item a turn streamed back: no prompt, no notice.
const own = (item: Item) => item.kind !== "prompt" && item.kind !== "notice";

// turnOf is what the turn going on streamed back, known by the turn its
// items name, so a prompt queued while it runs leaves it whole; before is the
// turn that was latest as this one began, none of it this turn's. Items that
// name no turn count from the last prompt.
function turnOf(items: Item[], before?: string): Item[] {
  const latest = items.findLast(own);
  if (latest?.turn !== undefined)
    return latest.turn === before
      ? []
      : items.filter((item) => own(item) && item.turn === latest.turn);
  let i = items.length;
  while (i > 0 && items[i - 1]?.kind !== "prompt") i--;
  return items.slice(i).filter(own);
}

// phaseOf is the turn's phase. A call whose input has not come is being
// written; one with its input runs; a step that settled waits for its
// calls; once every call has ended the next request is out.
export function phaseOf(items: Item[], before?: string): Phase {
  const turn = turnOf(items, before);
  const running = turn.findLast((item) => item.kind === "tool" && item.status === "running");
  if (running) return running.text === "" ? "tool-input" : "tool-use";
  const last = turn.at(-1);
  if (!last) return "requesting";
  if (last.kind === "tool") return "requesting";
  if (last.status === "streaming") return last.kind === "thinking" ? "thinking" : "responding";
  return "tool-use";
}

// lengthOf is what streamed back in the turn, counted as the terminal
// program counts it: the characters of its replies, its thinking and its
// calls' inputs.
export function lengthOf(items: Item[], before?: string): number {
  let n = 0;
  for (const item of turnOf(items, before)) n += item.text.length;
  return n;
}

// The shown count moves towards what streamed back every 50 ms, by more the
// further behind it is, and leaps a gap past 2000 characters.
const step = 50;
const leap = 2000;
const towards = (gap: number) =>
  gap < 70 ? 3 : gap < 200 ? Math.max(8, Math.ceil(gap * 0.15)) : 50;

// Tracker keeps what the line needs between ticks: the shown length and the
// turn latest before this one. It is reset when a turn begins, with the items
// then, and with neither when the face connects: a turn it joined shows what
// streamed at once.
export class Tracker {
  private shown = 0;
  private last: number | undefined;
  private before: string | undefined;

  reset(now?: number, items: Item[] = []) {
    this.shown = 0;
    this.last = now;
    this.before = items.findLast(own)?.turn;
  }

  at(items: Item[], now: number): Progress {
    const phase = phaseOf(items, this.before);
    const running = turnOf(items, this.before).findLast(
      (item) => item.kind === "tool" && item.status === "running",
    );
    const length = lengthOf(items, this.before);
    if (this.last === undefined || Math.abs(length - this.shown) > leap) {
      this.shown = length;
      this.last = now;
    }
    const steps = Math.floor((now - this.last) / step);
    this.last += steps * step;
    for (let i = 0; i < steps && this.shown !== length; i++) {
      const gap = length - this.shown;
      const by = towards(Math.abs(gap));
      this.shown = gap > 0 ? Math.min(this.shown + by, length) : Math.max(this.shown - by, length);
    }
    return {
      phase,
      tokens: Math.round(this.shown / 4),
      tool: running ? nameOf(running) : undefined,
    };
  }
}

// count is a number of tokens as the terminal program writes it: 622, 1.2k.
const compact = new Intl.NumberFormat("en-US", {
  notation: "compact",
  minimumFractionDigits: 1,
  maximumFractionDigits: 1,
});
export const count = (n: number) => (n < 1000 ? String(n) : compact.format(n).toLowerCase());
