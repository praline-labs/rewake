// The row under the conversation (docs/face.md, "The look"): while a turn
// works, a turning glyph and a verb picked for the turn, in Claude Code's
// orange, then faint its time, its phase's arrow with the tokens streamed
// back and how its thinking goes, as the width allows; after it, how long it
// took and when it ended.

import type { FaceLine, Span } from "./messages.ts";
import { count, type Progress } from "./progress.ts";
import { widthOf } from "./wrap.ts";

// Clock is what the face knows of the turn's time: when the working turn
// began, or how long the last one took, each only when the face saw it, and
// the turn's progress at the moment.
export type Clock = {
  now: number;
  since?: number;
  worked?: { took: number; at: number };
  progress?: Progress;
};

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

// The glyph turns a frame every 120 ms, there and back, and the verb's colour
// pulses towards a lighter shade and back every two seconds, as the terminal
// program draws them; both, and the time, are read from the clock's one now,
// so they move in the same paint.
const glyphs = ["·", "✢", "*", "✶", "✻", "✽"];
const frames = [...glyphs, ...[...glyphs].reverse()];
export const glyphEvery = 120;
const pulse = 2000;
// The glow moves in steps, so the face posts a frame when it changes, not at
// every tick.
const glowSteps = 8;

export const glyphAt = (now: number) => frames[Math.floor(now / glyphEvery) % frames.length];

export function glowAt(now: number): number {
  const wave = 0.5 - 0.5 * Math.cos((2 * Math.PI * (now % pulse)) / pulse);
  return Math.round(wave * glowSteps) / glowSteps;
}

// blinkEvery is how long a running call's dot shows and then hides.
export const blinkEvery = 600;
export const blinkOff = (now: number) => Math.floor(now / blinkEvery) % 2 === 1;

// The bracket shows from the first token or thinking word, else after 16 s.
const quiet = 16_000;
const sep = " · ";

// progressOf is the bracket after the verb, each part in only where the
// width leaves it room past the verb, as the terminal program fits it: the
// thinking word first, then the time, then the tokens; shown in the order
// time, tokens, thinking.
function progressOf(verb: string, clock: Clock, width: number): Span[] {
  const p = clock.progress;
  const elapsed = clock.since === undefined ? undefined : clock.now - clock.since;
  const tokens = p?.tokens ?? 0;
  const shown = p?.thinking !== undefined || tokens > 0 || (elapsed ?? 0) > quiet;
  // The glyph and its space before the verb, and five cells kept free.
  const room = width - (widthOf(verb) + 2) - 5;
  let thinking = p?.thinking && room > widthOf(p.thinking.text) ? p.thinking.text : undefined;
  if (!thinking && p?.thinking?.kind === "thinking" && room > widthOf("thinking"))
    thinking = "thinking";
  const used = thinking ? widthOf(thinking) + sep.length : 0;
  const timer = elapsed === undefined ? undefined : duration(elapsed);
  const withTimer = shown && timer !== undefined && room > used + widthOf(timer);
  const said = `${count(tokens)} tokens`;
  const withTokens =
    shown &&
    tokens > 0 &&
    room > used + (withTimer && timer ? widthOf(timer) + sep.length : 0) + 2 + widthOf(said);
  const parts: Span[][] = [];
  if (withTimer && timer) parts.push([{ text: timer, tone: "faint" }]);
  if (withTokens) {
    const arrow = p?.phase === "requesting" ? "↑" : "↓";
    parts.push([{ text: `${arrow} ${said}`, tone: "faint" }]);
  }
  if (thinking) parts.push([{ text: thinking, tone: "faint" }]);
  if (parts.length === 0) return [];
  return [
    { text: " (", tone: "faint" },
    ...parts.flatMap((part, i) =>
      i > 0 ? [{ text: sep, tone: "faint" as const }, ...part] : part,
    ),
    { text: ")", tone: "faint" },
  ];
}

// turnLine is the row under the conversation that says the turn goes on, or
// how long the last one took.
export function turnLine(turn: string, clock: Clock, width = 80): FaceLine | undefined {
  const glyph = { text: `${glyphAt(clock.now)} `, tone: "accent" as const };
  const glow = glowAt(clock.now);
  if (turn === "working") {
    const verb = `${clock.since === undefined ? "Working" : pick(doing, clock.since)}…`;
    const spans: Span[] = [glyph, { text: verb, tone: "accent", glow }];
    spans.push(...progressOf(verb, clock, width));
    return { spans };
  }
  if (turn === "compacting")
    return { spans: [glyph, { text: "Compacting…", tone: "accent", glow }] };
  if (turn === "idle" && clock.worked !== undefined) {
    const { took, at } = clock.worked;
    const said = `${pick(did, at)} for ${duration(took)} · done ${timeOf(at)}`;
    return { spans: [{ text: "  " }, { text: said, tone: "faint" }] };
  }
  return undefined;
}
