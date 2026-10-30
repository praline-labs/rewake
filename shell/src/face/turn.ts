// The row under the conversation (docs/face.md, "The look"): while a turn
// works, a turning glyph and a verb picked for the turn, in Claude Code's
// orange, the time faint; after it, how long it took and when it ended.

import type { FaceLine, Span } from "./messages.ts";

// Clock is what the face knows of the turn's time: when the working turn
// began, or how long the last one took, each only when the face saw it.
export type Clock = { now: number; since?: number; worked?: { took: number; at: number } };

export function duration(ms: number): string {
  const s = Math.max(Math.floor(ms / 1000), 0);
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
}

// timeOf is a moment's local time as a terminal program says it, 7:01 AM.
export function timeOf(ms: number): string {
  const d = new Date(ms);
  const h = d.getHours();
  return `${h % 12 || 12}:${String(d.getMinutes()).padStart(2, "0")} ${h < 12 ? "AM" : "PM"}`;
}

// rewake's own verbs, a turn's pick of them kept for the turn by the moment
// it began or ended.
const doing = [
  "Working",
  "Thinking",
  "Pondering",
  "Mulling",
  "Brewing",
  "Tinkering",
  "Crafting",
  "Musing",
];
const did = ["Worked", "Brewed", "Mulled", "Pondered", "Tinkered", "Crafted", "Cooked", "Churned"];
const pick = (verbs: string[], at: number) =>
  verbs[Math.floor(at / 1000) % verbs.length] ?? verbs[0];

// The glyph turns a frame a second, as often as the face redraws a working
// turn.
const frames = ["·", "✢", "*", "✶", "✻", "✽"];

// turnLine is the row under the conversation that says the turn goes on, or
// how long the last one took.
export function turnLine(turn: string, clock: Clock): FaceLine | undefined {
  const glyph = {
    text: `${frames[Math.floor(clock.now / 1000) % frames.length]} `,
    tone: "accent" as const,
  };
  if (turn === "working") {
    const verb = clock.since === undefined ? "Working" : pick(doing, clock.since);
    const spans: Span[] = [glyph, { text: `${verb}…`, tone: "accent" }];
    if (clock.since !== undefined)
      spans.push({ text: ` (${duration(clock.now - clock.since)})`, tone: "faint" });
    return { spans };
  }
  if (turn === "compacting") return { spans: [glyph, { text: "Compacting…", tone: "accent" }] };
  if (turn === "idle" && clock.worked !== undefined) {
    const { took, at } = clock.worked;
    const said = `${pick(did, at)} for ${duration(took)} · done ${timeOf(at)}`;
    return { spans: [{ text: "  " }, { text: said, tone: "faint" }] };
  }
  return undefined;
}
