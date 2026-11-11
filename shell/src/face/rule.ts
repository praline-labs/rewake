// The input's top rule (docs/face.md, "The look"): at its left end the
// session's permission mode, as the terminal program names it under its
// input, or pi's runtime work mode. A thinking level is a fallback before
// the mode is observed. All come from the mirror; at its right the run's
// room work, as Claude Code's hint line ends with it (internal/core/work),
// `── brief 61 1/3 ─`, and nothing without work. A narrow pane drops the
// level's hint first, then the mode or the level, then cuts the work with …;
// whatever its role, a session's rule is drawn the same.

import type { PermissionMode, ThinkingLevel } from "../../../face/src/mirror.gen.ts";

// Left is what may stand at the rule's left, as the face heard it from the
// session's mirror. Runtime work mode is separate from permission mode;
// neither is inferred from role or from a template's start.
export type Left = { mode?: PermissionMode; level?: ThinkingLevel; workMode?: string };

// RulePart is a run of the rule's text, in the rule's colour unless it
// names the mode or the thinking level, drawn in that one's colour.
export type RulePart = {
  text: string;
  mode?: PermissionMode;
  level?: ThinkingLevel;
  workMode?: string;
};

// modeNames are the modes as the terminal program shows them; the default
// one says nothing.
export const modeNames: Partial<Record<PermissionMode, string>> = {
  auto: "⏵⏵ auto mode",
  plan: "⏸ plan mode",
  edits: "⏵⏵ accept edits",
  bypass: "⏵⏵ bypass permissions",
  unasked: "⏵⏵ don't ask",
};

// levelName is a thinking level in pi's words for it beside its model,
// `high`, or `thinking off` (its footer, read on 1.4.0).
export const levelName = (level: string) => `◆ ${level === "off" ? "thinking off" : level}`;

// cycleHint says the key, as the terminal program says it after its mode.
const cycleHint = " (shift+tab to cycle)";

const graphemes = new Intl.Segmenter();
const line = (n: number) => "─".repeat(Math.max(n, 0));
const width = (text: string) => Bun.stringWidth(text);

// cut is the text within room cells, cut with … where it is longer.
function cut(text: string, room: number): string {
  if (width(text) <= room) return text;
  let kept = "";
  for (const { segment } of graphemes.segment(text)) {
    if (width(kept + segment) > room - 1) break;
    kept += segment;
  }
  return `${kept.trimEnd()}…`;
}

// tail is a text at the rule's right end, ` text ─`.
const tail = (text?: string) => (text ? ` ${text} ─` : "");

// lefts are what may stand at the rule's left, the widest first: the mode,
// or the thinking level with its hint, then without.
function lefts({ mode, level, workMode }: Left = {}): RulePart[][] {
  if (workMode) {
    const label = { text: `◆ ${workMode}`, workMode };
    return [[label, { text: cycleHint }], [label]];
  }
  const named = mode ? modeNames[mode] : undefined;
  if (named) return [[{ text: named, mode }]];
  if (!level) return [];
  const label = { text: levelName(level), level };
  return [[label, { text: " (alt+t to cycle)" }], [label]];
}

// ruleParts is the rule cells wide: `── left ` at its left, ` work ─` at its
// right, at least a cell of the rule between them and two before the right
// alone. Without the room for both the left narrows, then goes; then the
// work is cut.
export function ruleParts(cells: number, work?: string, said?: Left): RulePart[] {
  const right = tail(work);
  for (const left of lefts(said)) {
    const used = left.reduce((n, part) => n + width(part.text), 0);
    const fill = cells - width("── ") - used - 1 - width(right);
    if (fill >= 1) return [{ text: "── " }, ...left, { text: ` ${line(fill)}${right}` }];
  }
  if (cells - width(right) >= 2) return [{ text: line(cells - width(right)) + right }];
  // Two cells of the rule before the work, a space, the work, a space and a
  // cell of the rule after it.
  const room = cells - 5;
  if (!work || room < 2) return [{ text: line(cells) }];
  const kept = tail(cut(work, room));
  return [{ text: line(cells - width(kept)) + kept }];
}

// ruleOf is the rule's text alone.
export const ruleOf = (cells: number, work?: string, said?: Left) =>
  ruleParts(cells, work, said)
    .map((p) => p.text)
    .join("");

// withMore is the rule with ` ↑ 3 more ` in the middle of its longest run
// of the rule, as pi's editor says what its text hides above it; the rule
// as it was where that run is too short for it.
export function withMore(parts: RulePart[], hidden: number): RulePart[] {
  if (hidden <= 0) return parts;
  const label = ` ↑ ${hidden} more `;
  let best: { at: number; start: number; length: number } | undefined;
  parts.forEach((part, at) => {
    if (part.mode || part.level || part.workMode) return;
    for (const run of part.text.matchAll(/─+/g)) {
      const length = run[0].length;
      if (!best || length > best.length) best = { at, start: run.index, length };
    }
  });
  if (!best || best.length < width(label) + 2) return parts;
  const { at, start, length } = best;
  const left = start + Math.floor((length - width(label)) / 2);
  const text = parts[at]?.text ?? "";
  const next = text.slice(0, left) + label + text.slice(left + width(label));
  return parts.map((part, i) => (i === at ? { ...part, text: next } : part));
}
