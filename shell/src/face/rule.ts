// The input's top rule (docs/face.md, "The look"): the session's permission
// mode at its left end, as the terminal program names it under its input,
// and at the right the run's room work, as Claude Code's hint line ends with
// it (internal/core/work), then the session's name, `── brief 61 1/3 ·
// review-pi ─`. A narrow pane drops the mode first, then the work, then cuts
// the name with …; whatever its role, a session's rule is drawn the same.

import type { PermissionMode } from "../../../face/src/mirror.gen.ts";

// RulePart is a run of the rule's text, in the rule's colour unless it
// names the mode, drawn in that mode's colour.
export type RulePart = { text: string; mode?: PermissionMode };

// modeNames are the modes as the terminal program shows them; the default
// one says nothing.
export const modeNames: Partial<Record<PermissionMode, string>> = {
  auto: "⏵⏵ auto mode",
  plan: "⏸ plan mode",
  edits: "⏵⏵ accept edits",
  bypass: "⏵⏵ bypass permissions",
  unasked: "⏵⏵ don't ask",
};

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

// ruleParts is the rule cells wide: `── mode ` at its left, ` work · name ─`
// at its right, at least a cell of the rule between them and two before the
// right alone. Without the room for both the mode goes; then the work; then
// the name, or the work where no name is given, is cut.
export function ruleParts(
  cells: number,
  work?: string,
  mode?: PermissionMode,
  name?: string,
): RulePart[] {
  const label = mode ? modeNames[mode] : undefined;
  const right = tail([work, name].filter(Boolean).join(" · "));
  if (label) {
    const fill = cells - width(`── ${label} `) - width(right);
    if (fill >= 1)
      return [{ text: "── " }, { text: label, mode }, { text: ` ${line(fill)}${right}` }];
  }
  if (cells - width(right) >= 2) return [{ text: line(cells - width(right)) + right }];
  if (name && work && cells - width(tail(name)) >= 2)
    return [{ text: line(cells - width(tail(name))) + tail(name) }];
  // Two cells of the rule before the text, a space, the text, a space and a
  // cell of the rule after it.
  const last = name ?? work;
  const room = cells - 5;
  if (!last || room < 2) return [{ text: line(cells) }];
  const kept = tail(cut(last, room));
  return [{ text: line(cells - width(kept)) + kept }];
}

// ruleOf is the rule's text alone.
export const ruleOf = (cells: number, work?: string, mode?: PermissionMode, name?: string) =>
  ruleParts(cells, work, mode, name)
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
    if (part.mode) return;
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
