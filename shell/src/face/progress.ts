// What the working line tells of a turn's progress (docs/face.md, "The
// look"), as Claude Code's own spinner does: the phase its arrow shows, an
// estimate of what streamed back and how the thinking goes. All of it is
// derived from the model on the face's clock; the session sends none of it.

import type { Item } from "./mirror.gen.ts";

// Phase is where the turn's request stands: out with nothing back yet, or a
// reply, a thinking block, a call's input or a call coming back.
export type Phase = "requesting" | "responding" | "thinking" | "tool-input" | "tool-use";

// Thinking is the word after the tokens: thinking while a thinking block
// streams, then how long it took for a moment.
export type Thinking = { kind: "thinking" | "thought"; text: string };

export type Progress = { phase: Phase; tokens: number; thinking?: Thinking };

// turnOf is the items of the turn going on: those after its prompt.
function turnOf(items: Item[]): Item[] {
  let i = items.length;
  while (i > 0 && items[i - 1]?.kind !== "prompt") i--;
  return items.slice(i).filter((item) => item.kind !== "notice");
}

// phaseOf is the turn's phase. A call whose input has not come is being
// written; one with its input runs; a step that settled waits for its
// calls; once every call has ended the next request is out.
export function phaseOf(items: Item[]): Phase {
  const turn = turnOf(items);
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
export function lengthOf(items: Item[]): number {
  let n = 0;
  for (const item of turnOf(items)) if (item.kind !== "prompt") n += item.text.length;
  return n;
}

// The shown count moves towards what streamed back every 50 ms, by more the
// further behind it is, and leaps a gap past 2000 characters.
const step = 50;
const leap = 2000;
const towards = (gap: number) =>
  gap < 70 ? 3 : gap < 200 ? Math.max(8, Math.ceil(gap * 0.15)) : 50;

// The thinking word: by how long the block has gone on, then, once it ends
// and has shown for two seconds at least, how long it took, for two.
const held = 2000;
function thinkingWord(ms: number): string {
  if (ms >= 45_000) return "deep in thought";
  if (ms >= 30_000) return "thinking some more";
  if (ms >= 20_000) return "thinking more";
  if (ms >= 10_000) return "still thinking";
  return "thinking";
}

// Tracker keeps what the line needs between ticks: the shown length and the
// thinking's moments. It is reset when a turn begins, and with no moment
// when the face connects: a turn it joined shows what streamed at once.
export class Tracker {
  private shown = 0;
  private last: number | undefined;
  private since: number | undefined;
  private ended: { at: number; took: number } | undefined;

  reset(now?: number) {
    this.shown = 0;
    this.last = now;
    this.since = this.ended = undefined;
  }

  at(items: Item[], now: number): Progress {
    const phase = phaseOf(items);
    const length = lengthOf(items);
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
      thinking: this.thinking(items, phase, now),
    };
  }

  private thinking(items: Item[], phase: Phase, now: number): Thinking | undefined {
    if (phase === "thinking") {
      this.since ??= now;
      this.ended = undefined;
      return { kind: "thinking", text: thinkingWord(now - this.since) };
    }
    if (this.since !== undefined) {
      this.ended = { at: now, took: now - this.since };
      this.since = undefined;
    }
    if (!this.ended) return undefined;
    const from = this.ended.at + Math.max(0, held - this.ended.took);
    if (now < from) {
      // Still the thinking word, unless a call runs.
      const calling = turnOf(items).some((i) => i.kind === "tool" && i.status === "running");
      return calling ? undefined : { kind: "thinking", text: "thinking" };
    }
    if (now < from + held)
      return {
        kind: "thought",
        text: `thought for ${Math.max(1, Math.round(this.ended.took / 1000))}s`,
      };
    this.ended = undefined;
    return undefined;
  }
}

// count is a number of tokens as the terminal program writes it: 622, 1.2k.
const compact = new Intl.NumberFormat("en-US", {
  notation: "compact",
  minimumFractionDigits: 1,
  maximumFractionDigits: 1,
});
export const count = (n: number) => (n < 1000 ? String(n) : compact.format(n).toLowerCase());
