// The row under the conversation (docs/face.md, "The look"): while a turn
// works, a turning dot in the terminal's bright green and what the turn does
// by its phase, then faint its time and its phase's arrow with the tokens
// streamed back, as the width allows; after it, how long it took and when it
// ended.

import type { FaceLine, Span } from "./messages.ts";
import { count, type Progress } from "./progress.ts";
import { widthOf } from "./wrap.ts";

// Clock is what the face knows of the turn's time: when the working turn
// began, or how long the last one took, as the mirror timed them, and
// the turn's progress at the moment.
export type Clock = {
  now: number;
  since?: number;
  worked?: { took: number; at: number; stopped?: boolean };
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

// The glyph turns a frame every 120 ms on the clock's one now, so it moves in
// the same paint as the time. The one-dot orbit is the one in use; the
// other sets are kept for the owner to switch to.
export const arc = ["◜", "◝", "◞", "◟"];
export const orbit = ["⠁", "⠂", "⠄", "⡀", "⢀", "⠠", "⠐", "⠈"];
export const orbit2 = ["⠃", "⠆", "⡄", "⣀", "⢠", "⠰", "⠘", "⠉"];
export const orbit3 = ["⠋", "⠙", "⠸", "⢰", "⣠", "⣄", "⡆", "⠇"];
export const beat = ["·", "•", "●", "•"];
const frames = orbit;
export const glyphEvery = 120;

export const glyphAt = (now: number) => frames[Math.floor(now / glyphEvery) % frames.length];

// workingText is what the turn does now, by its phase.
function workingText(p: Progress | undefined): string {
  switch (p?.phase) {
    case "thinking":
      return "Thinking…";
    case "responding":
      return "Writing…";
    case "tool-input":
    case "tool-use":
      return p.tool ? `Running ${p.tool}…` : "Working…";
    case "requesting":
      return "Waiting…";
    default:
      return "Working…";
  }
}

// blinkEvery is how long a running call's dot shows and then hides.
export const blinkEvery = 600;
export const blinkOff = (now: number) => Math.floor(now / blinkEvery) % 2 === 1;

// The bracket shows from the first token or thinking word, else after 16 s.
const quiet = 16_000;
const sep = " · ";

// progressOf is the bracket after the text, each part in only where the
// width leaves it room past the text, as the terminal program fits it: the
// time, then the tokens.
function progressOf(verb: string, clock: Clock, width: number): Span[] {
  const p = clock.progress;
  const elapsed = clock.since === undefined ? undefined : clock.now - clock.since;
  const tokens = p?.tokens ?? 0;
  const shown = tokens > 0 || (elapsed ?? 0) > quiet;
  // The glyph and its space before the text, and five cells kept free.
  const room = width - (widthOf(verb) + 2) - 5;
  const timer = elapsed === undefined ? undefined : duration(elapsed);
  const withTimer = shown && timer !== undefined && room > widthOf(timer);
  const said = `${count(tokens)} tokens`;
  const withTokens =
    shown &&
    tokens > 0 &&
    room > (withTimer && timer ? widthOf(timer) + sep.length : 0) + 2 + widthOf(said);
  const parts: Span[][] = [];
  if (withTimer && timer) parts.push([{ text: timer, tone: "faint" }]);
  if (withTokens) {
    const arrow = p?.phase === "requesting" ? "↑" : "↓";
    parts.push([{ text: `${arrow} ${said}`, tone: "faint" }]);
  }
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
  if (turn === "working") {
    const verb = workingText(clock.progress);
    const spans: Span[] = [glyph, { text: verb, tone: "accent" }];
    spans.push(...progressOf(verb, clock, width));
    return { spans };
  }
  if (turn === "compacting") return { spans: [glyph, { text: "Compacting…", tone: "accent" }] };
  if (turn === "idle" && clock.worked !== undefined) {
    const { took, at, stopped } = clock.worked;
    const said = `${stopped ? "Stopped after" : "Done in"} ${duration(took)} · ${timeOf(at)}`;
    return { spans: [{ text: "  " }, { text: said, tone: "faint" }] };
  }
  return undefined;
}
